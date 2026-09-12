package app

import (
	"log/slog"
	"net/http"

	"sade/config"
	"sade/internals/app/auth"
	"sade/internals/app/job"
	"sade/internals/app/payment"
	"sade/internals/app/share"
	"sade/internals/app/user"
	"sade/internals/httpx"
)

// Deps is everything NewRouter needs: the config, the shared logger, the
// per-domain handlers, and the auth service the Auth middleware calls.
type Deps struct {
	Cfg     *config.Config
	Log     *slog.Logger
	Auth    *auth.Handler
	AuthSvc *auth.Service
	User    *user.Handler
	Job     *job.Handler
	Share   *share.Handler
	Payment *payment.Handler
}

// NewRouter builds the application's HTTP handler: routes plus the global
// middleware chain (Recover -> RequestLog -> CORS -> Auth -> routes).
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthz)

	// Public auth endpoints.
	mux.HandleFunc("POST /api/auth/request", d.Auth.RequestLink)
	mux.HandleFunc("GET /api/auth/callback", d.Auth.Callback)
	mux.HandleFunc("POST /api/auth/logout", d.Auth.Logout)

	// Public share links (no session): the watermarked preview, inline or as
	// a download. Authorised by the signed token in the path, not a cookie.
	mux.HandleFunc("GET /p/{token}", d.Share.Preview)
	mux.HandleFunc("GET /d/{token}", d.Share.Download)
	// The clean original, once paid for - see internals/app/payment.
	mux.HandleFunc("GET /o/{token}", d.Share.Original)

	// Public payment endpoints: the recipient on /preview/[token] never has
	// a session, so these are authorised by the same preview token, not a
	// cookie (the webhook is authorised by Stripe's own request signature).
	mux.HandleFunc("POST /api/payments/checkout", d.Payment.Checkout)
	mux.HandleFunc("GET /api/payments/status", d.Payment.Status)
	mux.HandleFunc("POST /api/payments/webhook", d.Payment.Webhook)

	// Signed-in.
	mux.Handle("GET /api/me", RequireUser(http.HandlerFunc(me)))

	// Operator-scoped: require a session and hand the job handlers the
	// operator id, so the job package never imports the auth layer.
	operator := func(h http.HandlerFunc) http.Handler {
		return RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := UserFrom(r.Context())
			h(w, r.WithContext(job.WithUserID(r.Context(), u.ID)))
		}))
	}
	mux.Handle("POST /api/jobs", operator(d.Job.Create))
	mux.Handle("GET /api/jobs", operator(d.Job.List))
	mux.Handle("GET /api/jobs/{id}", operator(d.Job.Get))
	mux.Handle("GET /api/jobs/{id}/assets/{assetId}/content", operator(d.Job.Content))

	// Admin only.
	admin := func(h http.HandlerFunc) http.Handler {
		return RequireUser(RequireRole(user.RoleAdmin)(h))
	}
	mux.Handle("GET /api/users", admin(d.User.List))
	mux.Handle("GET /api/users/{id}", admin(d.User.Get))
	mux.Handle("PATCH /api/users/{id}", admin(d.User.SetRole))

	// The built SvelteKit app (frontend/build), SPA-fallback served for
	// everything else. Skipped when it hasn't been built yet.
	if hasFrontendBuild(d.Cfg.App.FrontendDir) {
		mux.Handle("/", spaFileServer(d.Cfg.App.FrontendDir))
	}

	return chain(mux,
		Recover(d.Log),
		RequestLog(d.Log),
		CORS(d.Cfg.Server.CORSAllowedOrigins),
		Auth(d.AuthSvc, d.Cfg.Auth.SessionCookieName),
	)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// me returns the user attached by the Auth middleware. RequireUser guards the
// route, so the lookup here always succeeds.
func me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}
