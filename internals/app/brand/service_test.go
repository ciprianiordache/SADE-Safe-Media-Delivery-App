package brand

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/job"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/storage"
	"sade/internals/testutil"
	"sade/internals/wmopts"
)

func newSvc(t *testing.T) (*Service, string, storage.Storage) {
	t.Helper()
	db := testutil.DB(t, user.User{}, Brand{}, job.Job{})
	uid, err := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatal(err)
	}
	testDB = db
	return NewService(NewRepo(db), store, testutil.Logger()), uid, store
}

// testDB is the database behind the last newSvc (tests here are sequential).
var testDB *database.Database

func pngOf(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 80, B: 20, A: uint8(x % 256)})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestDefaultsThenUpdate(t *testing.T) {
	svc, uid, _ := newSvc(t)
	got, err := svc.Get(uid)
	if err != nil || got.Position != DefaultPosition || got.Onboarded || got.HasLogo {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	ok := UpdateRequest{CompanyName: "  Studio X\n", Position: "tiled", OpacityPct: 50, LogoScalePct: 20, TextTemplate: "{company}"}
	got, err = svc.Update(uid, ok)
	if err != nil || got.CompanyName != "Studio X" || got.Position != "tiled" {
		t.Fatalf("update = %+v, %v", got, err)
	}
	for _, bad := range []UpdateRequest{
		{Position: "middle", OpacityPct: 50, LogoScalePct: 20},
		{Position: "tiled", OpacityPct: 0, LogoScalePct: 20},
		{Position: "tiled", OpacityPct: 50, LogoScalePct: 90},
	} {
		if _, err := svc.Update(uid, bad); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Update(%+v) = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestLogoUploadNormalisesAndSnapshots(t *testing.T) {
	svc, uid, store := newSvc(t)
	ctx := context.Background()

	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader([]byte("<svg>not an image</svg>"))); !errors.Is(err, ErrInvalidLogo) {
		t.Fatalf("garbage logo = %v, want ErrInvalidLogo", err)
	}

	got, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(1600, 400)))
	if err != nil || !got.HasLogo || got.LogoVersion == "" {
		t.Fatalf("SetLogo = %+v, %v", got, err)
	}
	rc, err := svc.Logo(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := image.Decode(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if b := stored.Bounds(); b.Dx() != logoStoreDim || b.Dy() != 200 {
		t.Errorf("stored logo = %v, want scaled to %dx200", b, logoStoreDim)
	}

	snap := wmopts.Decode(mustSnap(t, svc, uid))
	if snap.LogoKey == "" {
		t.Fatal("snapshot lacks the logo")
	}
	if _, err := store.Stat(ctx, snap.LogoKey); err != nil {
		t.Errorf("snapshot points at a missing file: %v", err)
	}

	// A JPEG is accepted too (re-encoded as PNG).
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, img, nil)
	if _, err := svc.SetLogo(ctx, uid, &jb); err != nil {
		t.Errorf("JPEG logo: %v", err)
	}

	got, _ = svc.RemoveLogo(ctx, uid)
	if got.HasLogo || wmopts.Decode(mustSnap(t, svc, uid)).LogoKey != "" {
		t.Error("logo still set after removal")
	}
}

func TestOnboarding(t *testing.T) {
	svc, uid, _ := newSvc(t)
	got, err := svc.CompleteOnboarding(uid)
	if err != nil || !got.Onboarded {
		t.Fatalf("CompleteOnboarding = %+v, %v", got, err)
	}
	if again, _ := svc.Get(uid); !again.Onboarded {
		t.Error("onboarding not persisted")
	}
}

func mustSnap(t *testing.T, svc *Service, uid string) string {
	t.Helper()
	s, err := svc.Snapshot(uid)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLogoPixelCap(t *testing.T) {
	svc, uid, _ := newSvc(t)
	ctx := context.Background()
	// Square over 4 MP, and long-and-thin within the per-side cap but over 4 MP.
	for _, dims := range [][2]int{{2049, 2049}, {6000, 700}} {
		if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(dims[0], dims[1]))); !errors.Is(err, ErrInvalidLogo) {
			t.Errorf("%dx%d logo = %v, want ErrInvalidLogo", dims[0], dims[1], err)
		}
	}
	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(2000, 2000))); err != nil {
		t.Errorf("4 MP logo refused: %v", err)
	}
}

func TestLogoDecodeSlots(t *testing.T) {
	svc, uid, _ := newSvc(t)
	// Both slots busy: a third upload waits, and gives up with its request.
	for i := 0; i < decodeSlots; i++ {
		svc.decode <- struct{}{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(64, 64))); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SetLogo with no free slot = %v, want DeadlineExceeded", err)
	}
	// A freed slot lets the next one through, and it hands the slot back.
	<-svc.decode
	if _, err := svc.SetLogo(context.Background(), uid, bytes.NewReader(pngOf(64, 64))); err != nil {
		t.Fatalf("SetLogo after a slot freed: %v", err)
	}
	if n := len(svc.decode); n != decodeSlots-1 {
		t.Errorf("slots in use after upload = %d, want %d", n, decodeSlots-1)
	}
}

func TestOnlyTheCurrentLogoIsKept(t *testing.T) {
	svc, uid, store := newSvc(t)
	ctx := context.Background()
	key := func() string { return wmopts.Decode(mustSnap(t, svc, uid)).LogoKey }
	exists := func(k string) bool { _, err := store.Stat(ctx, k); return err == nil }

	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(64, 64))); err != nil {
		t.Fatal(err)
	}
	keyA := key()
	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(80, 64))); err != nil {
		t.Fatal(err)
	}
	keyB := key()
	if exists(keyA) || !exists(keyB) {
		t.Fatalf("after replace: old exists=%v, new exists=%v", exists(keyA), exists(keyB))
	}

	// Re-uploading the same picture gives the same key: never delete it.
	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(80, 64))); err != nil {
		t.Fatal(err)
	}
	if key() != keyB || !exists(keyB) {
		t.Fatal("re-uploading the current logo deleted it")
	}

	// A pending job still uses keyB: replacing keeps the file.
	j := &job.Job{UserID: uid, Status: job.StatusPending, MediaType: "image", RecipientEmail: "r@example.com",
		WatermarkOpts: wmopts.Encode(wmopts.Opts{LogoKey: keyB})}
	jobID, err := testDB.CRUD().Create(j)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLogo(ctx, uid, bytes.NewReader(pngOf(96, 64))); err != nil {
		t.Fatal(err)
	}
	keyC := key()
	if !exists(keyB) {
		t.Fatal("deleted a logo a pending job still uses")
	}

	// Once that job is done, removing the logo deletes the current file.
	if _, err := testDB.Exec(testDB.Rebind("UPDATE jobs SET status = 'done' WHERE id = ?"), jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveLogo(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if exists(keyC) {
		t.Error("removed logo's file still stored")
	}
}

func TestLogoUploadDailyLimit(t *testing.T) {
	svc, uid, _ := newSvc(t)
	h := NewHandler(svc, testutil.Logger())
	for i := 1; i <= LogoUploadsPerDay+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/brand/logo", nil)
		req = req.WithContext(WithUserID(req.Context(), uid))
		rec := httptest.NewRecorder()
		h.UploadLogo(rec, req)
		want := http.StatusBadRequest // no file: still counts as an attempt
		if i > LogoUploadsPerDay {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("attempt %d = %d, want %d", i, rec.Code, want)
		}
	}
}
