package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/auth"
	"sade/internals/app/job"
	"sade/internals/app/magic_token"
	"sade/internals/app/session"
	"sade/internals/app/share"
	"sade/internals/app/user"
	"sade/internals/storage"
	"sade/internals/testutil"
	"sade/internals/token"
)

// buildTestAppWithFrontend is buildTestApp plus a fake adapter-static build
// directory (index.html + one real asset) wired in via Cfg.App.FrontendDir,
// so NewRouter registers the SPA handler.
func buildTestAppWithFrontend(t *testing.T) *httptest.Server {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{}, job.Job{}, asset.Asset{})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>shell</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "_app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_app", "chunk.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Server.CORSAllowedOrigins = []string{"http://localhost:5173"}
	cfg.Auth = config.AuthConfig{HMACSecret: testShareSecret, SessionCookieName: "sade_session"}
	cfg.Upload = config.Defaults().Upload
	cfg.App.FrontendDir = dir

	store, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	signer, err := token.New(cfg.Auth.HMACSecret)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}

	userSvc := user.NewService(user.NewRepo(db), testutil.Logger())
	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), &capMailer{},
		cfg.Auth, "http://APIBASE", "http://app.test", testutil.Logger(),
	)
	jobSvc := job.NewService(job.NewRepo(db), asset.NewRepo(db), store, cfg.Upload, testutil.Logger())

	router := NewRouter(Deps{
		Cfg:     cfg,
		Log:     testutil.Logger(),
		Auth:    auth.NewHandler(authSvc, cfg.Auth, testutil.Logger()),
		AuthSvc: authSvc,
		User:    user.NewHandler(userSvc, testutil.Logger()),
		Job:     job.NewHandler(jobSvc, store, cfg.Upload, testutil.Logger()),
		Share:   share.NewHandler(signer, asset.NewRepo(db), store, testutil.Logger()),
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestStaticServesFrontendBuild(t *testing.T) {
	srv := buildTestAppWithFrontend(t)
	client := &http.Client{}

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"real asset", "/_app/chunk.js", http.StatusOK, "console.log(1)"},
		{"root", "/", http.StatusOK, "<html>shell</html>"},
		{"spa route falls back to shell", "/app/jobs/some-id", http.StatusOK, "<html>shell</html>"},
		{"unmatched /api is a real 404, not the shell", "/api/nope", http.StatusNotFound, ""},
		{"unmatched /p is a real 404, not the shell", "/p/nope", http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("%s = %d, want %d", tc.path, resp.StatusCode, tc.wantStatus)
			}
			if tc.wantBody != "" {
				buf := make([]byte, len(tc.wantBody))
				_, _ = resp.Body.Read(buf)
				if string(buf) != tc.wantBody {
					t.Errorf("%s body = %q, want %q", tc.path, buf, tc.wantBody)
				}
			}
		})
	}
}

func TestStaticDisabledWhenFrontendNotBuilt(t *testing.T) {
	// buildTestApp (router_test.go) leaves Cfg.App.FrontendDir empty, so the
	// static handler must not be registered - a stray GET / just 404s from
	// the bare mux instead of panicking on a missing directory.
	srv, client, _, _ := buildTestApp(t)
	if got := get(t, client, srv.URL+"/"); got != http.StatusNotFound {
		t.Errorf("GET / with no frontend build = %d, want 404", got)
	}
}
