package job

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/storage"
	"sade/internals/testutil"
)

// --- fakes -----------------------------------------------------------------

type memStore struct {
	mu     sync.Mutex
	blobs  map[string][]byte
	putErr error
}

func (m *memStore) Put(_ context.Context, key string, r io.Reader) (storage.FileInfo, error) {
	if m.putErr != nil {
		return storage.FileInfo{}, m.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return storage.FileInfo{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blobs == nil {
		m.blobs = map[string][]byte{}
	}
	m.blobs[key] = b
	return storage.FileInfo{Key: key, Size: int64(len(b))}, nil
}

func (m *memStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(b))), nil
}

func (m *memStore) Stat(_ context.Context, key string) (storage.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[key]
	if !ok {
		return storage.FileInfo{}, storage.ErrNotFound
	}
	return storage.FileInfo{Key: key, Size: int64(len(b))}, nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blobs, key)
	return nil
}

func (m *memStore) LocalPath(string) (string, bool) { return "", false }

func (m *memStore) has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.blobs[key]
	return ok
}

// failCreateAssets wraps a real asset.Repo and forces Create to fail, to
// exercise the service's rollback path.
type failCreateAssets struct{ asset.Repo }

func (failCreateAssets) Create(*asset.Asset) (string, error) {
	return "", errors.New("boom")
}

// --- helpers -------------------------------------------------------------

func newSvc(t *testing.T, store storage.Storage, assets asset.Repo, cfg config.UploadConfig) (Service, *database.Database, string) {
	t.Helper()
	db := testutil.DB(t, user.User{}, Job{}, asset.Asset{})
	uid, err := db.CRUD().Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if assets == nil {
		assets = asset.NewRepo(db)
	}
	return NewService(NewRepo(db), assets, store, cfg, testutil.Logger()), db, uid
}

func upload(name, body string) NewUpload {
	return NewUpload{
		RecipientEmail: "client@example.com",
		Filename:       name,
		DeclaredSize:   int64(len(body)),
		File:           strings.NewReader(body),
	}
}

// --- tests -------------------------------------------------------------

func TestCreateStoresJobAndOriginalAsset(t *testing.T) {
	store := &memStore{}
	svc, db, uid := newSvc(t, store, nil, config.Defaults().Upload)

	resp, err := svc.Create(context.Background(), uid, upload("clip.MP4", "fake-video-bytes"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.Status != StatusPending || resp.MediaType != MediaVideo || resp.WatermarkKind != WatermarkBoth {
		t.Errorf("job response = %+v", resp)
	}
	if len(resp.Assets) != 1 || resp.Assets[0].Kind != asset.KindOriginal {
		t.Fatalf("expected one original asset, got %+v", resp.Assets)
	}
	a := resp.Assets[0]
	if a.SizeBytes != int64(len("fake-video-bytes")) || a.Checksum == "" {
		t.Errorf("asset metadata wrong: %+v", a)
	}

	wantKey := "originals/" + resp.ID + "/original.mp4"
	if !store.has(wantKey) {
		t.Errorf("blob not stored under %q; keys = %v", wantKey, store.blobs)
	}

	// The job row is really persisted and pending.
	var j Job
	if err := db.CRUD().ReadOne(Job{}, "id", resp.ID, &j); err != nil {
		t.Fatalf("read back job: %v", err)
	}
	if j.UserID != uid || j.Status != StatusPending {
		t.Errorf("persisted job = %+v", j)
	}
}

func TestCreateRejectsUnsupportedExtension(t *testing.T) {
	store := &memStore{}
	svc, db, uid := newSvc(t, store, nil, config.Defaults().Upload)

	_, err := svc.Create(context.Background(), uid, upload("notes.txt", "hello"))
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Create(.txt) = %v, want ErrUnsupportedMedia", err)
	}
	var jobs []Job
	_ = db.CRUD().Read(Job{}, "user_id", uid, &jobs)
	if len(jobs) != 0 {
		t.Errorf("a job row was created for a rejected upload: %+v", jobs)
	}
	if len(store.blobs) != 0 {
		t.Errorf("a blob was stored for a rejected upload: %v", store.blobs)
	}
}

func TestCreateRejectsBadRecipient(t *testing.T) {
	svc, _, uid := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	in := upload("clip.mp4", "x")
	in.RecipientEmail = "not-an-email"
	if _, err := svc.Create(context.Background(), uid, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(bad email) = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsUnknownWatermarkKind(t *testing.T) {
	svc, _, uid := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	in := upload("clip.mp4", "x")
	in.WatermarkKind = "sideways"
	if _, err := svc.Create(context.Background(), uid, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(bad kind) = %v, want ErrInvalidInput", err)
	}
}

func TestCreateEnforcesSizeCapWhileStreaming(t *testing.T) {
	store := &memStore{}
	cfg := config.Defaults().Upload
	cfg.MaxSizeMiB = 1 // 1 MiB
	svc, db, uid := newSvc(t, store, nil, cfg)

	// A body over the cap, with DeclaredSize lying that it is small so only
	// the streaming guard can catch it.
	big := strings.Repeat("a", (1<<20)+4096)
	in := NewUpload{
		RecipientEmail: "client@example.com",
		Filename:       "clip.mp4",
		DeclaredSize:   10,
		File:           strings.NewReader(big),
	}
	if _, err := svc.Create(context.Background(), uid, in); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Create(oversize) = %v, want ErrFileTooLarge", err)
	}
	// Rollback: neither the job row nor the blob survive.
	var jobs []Job
	_ = db.CRUD().Read(Job{}, "user_id", uid, &jobs)
	if len(jobs) != 0 {
		t.Errorf("job row survived an oversize upload: %+v", jobs)
	}
	if len(store.blobs) != 0 {
		t.Errorf("blob survived an oversize upload: %v", store.blobs)
	}
}

func TestCreateRejectsEmptyFile(t *testing.T) {
	store := &memStore{}
	svc, _, uid := newSvc(t, store, nil, config.Defaults().Upload)
	if _, err := svc.Create(context.Background(), uid, upload("clip.mp4", "")); !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("Create(empty) = %v, want ErrEmptyFile", err)
	}
	if len(store.blobs) != 0 {
		t.Errorf("blob stored for an empty upload: %v", store.blobs)
	}
}

func TestCreateRollsBackWhenAssetWriteFails(t *testing.T) {
	store := &memStore{}
	db := testutil.DB(t, user.User{}, Job{}, asset.Asset{})
	uid, err := db.CRUD().Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := NewService(NewRepo(db), failCreateAssets{asset.NewRepo(db)}, store, config.Defaults().Upload, testutil.Logger())

	if _, err := svc.Create(context.Background(), uid, upload("clip.mp4", "bytes")); err == nil {
		t.Fatal("Create succeeded despite a failing asset write")
	}
	var jobs []Job
	_ = db.CRUD().Read(Job{}, "user_id", uid, &jobs)
	if len(jobs) != 0 {
		t.Errorf("job row survived a failed asset write: %+v", jobs)
	}
	if len(store.blobs) != 0 {
		t.Errorf("blob survived a failed asset write: %v", store.blobs)
	}
}

func TestGetIsScopedToOwnerAndCarriesAssets(t *testing.T) {
	svc, db, uid := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	other, err := db.CRUD().Create(&user.User{Email: "other@example.com"})
	if err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	created, err := svc.Create(context.Background(), uid, upload("clip.mp4", "bytes"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(uid, created.ID)
	if err != nil {
		t.Fatalf("Get(owner): %v", err)
	}
	if len(got.Assets) != 1 || got.Assets[0].Kind != asset.KindOriginal {
		t.Errorf("Get(owner) assets = %+v", got.Assets)
	}

	if _, err := svc.Get(other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(non-owner) = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(uid, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestListReturnsOwnJobsNewestFirst(t *testing.T) {
	svc, db, uid := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	other, _ := db.CRUD().Create(&user.User{Email: "other@example.com"})

	first, _ := svc.Create(context.Background(), uid, upload("a.mp4", "aa"))
	time.Sleep(5 * time.Millisecond)
	second, _ := svc.Create(context.Background(), uid, upload("b.mp3", "bb"))
	if _, err := svc.Create(context.Background(), other, upload("c.png", "cc")); err != nil {
		t.Fatalf("seed other job: %v", err)
	}

	list, err := svc.List(uid, 0, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List returned %d jobs, want 2 (own only)", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("List not newest-first: %s then %s", list[0].ID, list[1].ID)
	}
	for _, j := range list {
		if j.Assets != nil {
			t.Errorf("List response should not carry assets: %+v", j)
		}
	}
}
