package share_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/share"
	"sade/internals/storage"
	"sade/internals/testutil"
	"sade/internals/token"
)

const testSecret = "share-test-secret-0000000000000000"

type fakeAssets struct {
	byID map[string]*asset.Asset
	err  error
}

func (f fakeAssets) GetByID(id string) (*asset.Asset, error) {
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.byID[id]
	if !ok {
		return nil, asset.ErrNotFound
	}
	return a, nil
}

func localStore(t *testing.T) storage.Storage {
	t.Helper()
	s, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return s
}

// harness wires the share handler over a real local store and a fake asset
// reader, returning a running server, a signer over the same secret, and the
// bytes stored for the seeded preview asset (id "prev1").
func harness(t *testing.T, seed *asset.Asset) (*httptest.Server, *token.Signer, []byte) {
	t.Helper()
	store := localStore(t)
	content := []byte("WATERMARKED-PREVIEW-BYTES-0123456789-abcdefghij")
	if seed != nil && seed.Kind == asset.KindPreview {
		if _, err := store.Put(context.Background(), seed.StorageKey, bytes.NewReader(content)); err != nil {
			t.Fatalf("seed blob: %v", err)
		}
	}
	signer, err := token.New(testSecret)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	assets := fakeAssets{byID: map[string]*asset.Asset{}}
	if seed != nil {
		assets.byID[seed.ID] = seed
	}
	h := share.NewHandler(signer, assets, store, testutil.Logger())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}", h.Preview)
	mux.HandleFunc("GET /d/{token}", h.Download)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, signer, content
}

func previewAsset() *asset.Asset {
	return &asset.Asset{
		ID: "prev1", JobID: "job1", Kind: asset.KindPreview,
		StorageKey: "previews/job1/clip-preview.mp4",
		Filename:   "clip-preview.mp4", MIME: "video/mp4", SizeBytes: 46,
	}
}

func get(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func TestPreviewServesTheBytesInline(t *testing.T) {
	srv, signer, content := harness(t, previewAsset())
	tok, err := signer.Sign("preview", "prev1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, srv.URL+"/p/"+tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Errorf("body = %q, want %q", body, content)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `inline; filename="clip-preview.mp4"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("ServeContent should advertise Accept-Ranges: bytes")
	}
}

func TestDownloadServesAsAttachment(t *testing.T) {
	srv, signer, content := harness(t, previewAsset())
	tok, _ := signer.Sign("download", "prev1", time.Hour)

	resp := get(t, srv.URL+"/d/"+tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Errorf("body mismatch")
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="clip-preview.mp4"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestPreviewHonoursRangeRequests(t *testing.T) {
	srv, signer, content := harness(t, previewAsset())
	tok, _ := signer.Sign("preview", "prev1", time.Hour)

	resp := get(t, srv.URL+"/p/"+tok, map[string]string{"Range": "bytes=0-9"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content[0:10]) {
		t.Errorf("range body = %q, want %q", body, content[0:10])
	}
	if cr := resp.Header.Get("Content-Range"); cr == "" {
		t.Errorf("missing Content-Range header")
	}
}

func TestExpiredTokenIs410(t *testing.T) {
	srv, signer, _ := harness(t, previewAsset())
	tok, _ := signer.Sign("preview", "prev1", -time.Second)

	resp := get(t, srv.URL+"/p/"+tok, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want 410", resp.StatusCode)
	}
}

func TestBadAndWrongPurposeTokensAre404(t *testing.T) {
	srv, signer, _ := harness(t, previewAsset())

	// garbage
	if resp := get(t, srv.URL+"/p/not-a-real-token", nil); resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("garbage token = %d, want 404", resp.StatusCode)
	}
	// a valid "preview" token must not open /d ...
	pv, _ := signer.Sign("preview", "prev1", time.Hour)
	if resp := get(t, srv.URL+"/d/"+pv, nil); resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("preview token on /d = %d, want 404", resp.StatusCode)
	}
	// ... and a "download" token must not open /p
	dl, _ := signer.Sign("download", "prev1", time.Hour)
	if resp := get(t, srv.URL+"/p/"+dl, nil); resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("download token on /p = %d, want 404", resp.StatusCode)
	}
}

func TestUnknownAssetIs404(t *testing.T) {
	srv, signer, _ := harness(t, previewAsset())
	tok, _ := signer.Sign("preview", "does-not-exist", time.Hour)

	resp := get(t, srv.URL+"/p/"+tok, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOriginalAssetIsNeverServed(t *testing.T) {
	orig := &asset.Asset{
		ID: "orig1", JobID: "job1", Kind: asset.KindOriginal,
		StorageKey: "originals/job1/original.mp4", Filename: "clip.mp4", MIME: "video/mp4",
	}
	srv, signer, _ := harness(t, orig)
	tok, _ := signer.Sign("preview", "orig1", time.Hour)

	resp := get(t, srv.URL+"/p/"+tok, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("original served through /p: status = %d, want 404", resp.StatusCode)
	}
}

func TestMissingBlobIs404(t *testing.T) {
	// Asset row exists and is a preview, but nothing was stored at its key.
	a := previewAsset()
	a.StorageKey = "previews/job1/absent.mp4"
	store := localStore(t)
	signer, _ := token.New(testSecret)
	h := share.NewHandler(signer, fakeAssets{byID: map[string]*asset.Asset{"prev1": a}}, store, testutil.Logger())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}", h.Preview)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tok, _ := signer.Sign("preview", "prev1", time.Hour)
	resp := get(t, srv.URL+"/p/"+tok, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRepoErrorSurfacesAs500(t *testing.T) {
	store := localStore(t)
	signer, _ := token.New(testSecret)
	h := share.NewHandler(signer, fakeAssets{err: errors.New("db is down")}, store, testutil.Logger())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}", h.Preview)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tok, _ := signer.Sign("preview", "prev1", time.Hour)
	resp := get(t, srv.URL+"/p/"+tok, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}
