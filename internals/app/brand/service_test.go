package brand

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"sade/config"
	"sade/internals/app/user"
	"sade/internals/storage"
	"sade/internals/testutil"
	"sade/internals/wmopts"
)

func newSvc(t *testing.T) (*Service, string, storage.Storage) {
	t.Helper()
	db := testutil.DB(t, user.User{}, Brand{})
	uid, err := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepo(db), store, testutil.Logger()), uid, store
}

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

	got, _ = svc.RemoveLogo(uid)
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
