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
	"sade/internals/ratelimit"
)

// Deps is everything NewRouter needs: the config, the shared logger, the
// per-domain handlers, and the auth service the Auth middleware calls.
type Deps struct {
	Cfg             *config.Config
	Log             *slog.Logger
	Auth            *auth.Handler
	AuthSvc         *auth.Service
	User            *user.Handler
	Job             *job.Handler
	Share           *share.Handler
	Payment         *payment.Handler
	AuthRateLimiter *ratelimit.Limiter // bounds POST /api/auth/request; nil/limit<=0 disables it
	AuthCodeLimiter *ratelimit.Limiter // bounds POST /api/auth/code + passkey begin; nil disables it
	// PaymentRateLimiter bounds the public checkout/status routes per client
	// IP (each call can reach Stripe); nil/limit<=0 disables it.
	PaymentRateLimiter *ratelimit.Limiter
}

// NewRouter builds the application's HTTP handler: routes plus the global
// middleware chain (Recover -> RequestLog -> SecurityHeaders -> CORS ->
// OriginCheck -> Auth -> routes).
func NewRouter(d Deps) http.Handler {
	// Who to believe about the client's IP and scheme when a TLS terminator
	// sits in front of us - see proxy.go.
	trust := newProxyTrust(d.Cfg.Server.TrustedProxies)

	mux := http.NewServeMux()
	d.Auth.SetClientIP(trust.clientIP)

	mux.HandleFunc("GET /healthz", healthz)

	// Public auth endpoints. /request is rate-limited per client IP - it
	// emails a link on every well-formed address, so it's the one route an
	// attacker could otherwise use to spam a mailbox or hammer the DB.
	mux.Handle("POST /api/auth/request", RateLimit(d.AuthRateLimiter, trust)(http.HandlerFunc(d.Auth.RequestLink)))
	// The emailed link opens a frontend confirmation page; these two back
	// it (show the account, then sign in on the user's click). The old GET
	// callback only forwards there - a GET never signs anyone in.
	mux.HandleFunc("GET /api/auth/callback", d.Auth.Callback)
	mux.HandleFunc("POST /api/auth/verify", d.Auth.Verify)
	mux.HandleFunc("POST /api/auth/complete", d.Auth.Complete)
	// The 6-digit code from the same email, typed in the browser that asked
	// for it. Each email dies after a few wrong codes; this per-IP cap
	// bounds guessing across emails too.
	mux.Handle("POST /api/auth/code", RateLimit(d.AuthCodeLimiter, trust)(http.HandlerFunc(d.Auth.CompleteCode)))
	mux.HandleFunc("GET /api/auth/config", d.Auth.Config)
	mux.Handle("POST /api/auth/passkey/begin", RateLimit(d.AuthCodeLimiter, trust)(http.HandlerFunc(d.Auth.PasskeyLoginBegin)))
	mux.HandleFunc("POST /api/auth/passkey/finish", d.Auth.PasskeyLoginFinish)
	// The new-device alert email's "this wasn't me" (signed token).
	mux.HandleFunc("POST /api/auth/revoke", d.Auth.RevokeFromAlert)
	mux.HandleFunc("POST /api/auth/logout", d.Auth.Logout)
	mux.Handle("POST /api/auth/logout-all", RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		if err := d.AuthSvc.RevokeAll(r.Context(), u.ID); err != nil {
			d.Log.Error("logout everywhere", "user", u.ID, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not sign out")
			return
		}
		d.Auth.ClearSession(w)
		w.WriteHeader(http.StatusNoContent)
	})))

	// Public share links (no session): the watermarked preview, inline or as
	// a download. Authorised by the signed token in the path, not a cookie.
	long := LongRequest(d.Cfg.Server.LongRequestTimeout.Std())
	mux.Handle("GET /p/{token}", long(http.HandlerFunc(d.Share.Preview)))
	mux.Handle("GET /d/{token}", long(http.HandlerFunc(d.Share.Download)))
	// The clean original, once paid for - see internals/app/payment.
	mux.Handle("GET /o/{token}", long(http.HandlerFunc(d.Share.Original)))

	// Public payment endpoints: the recipient on /preview/[token] never has
	// a session, so these are authorised by the same preview token, not a
	// cookie (the webhook is authorised by Stripe's own request signature).
	payLimit := RateLimit(d.PaymentRateLimiter, trust)
	mux.Handle("POST /api/payments/checkout", payLimit(http.HandlerFunc(d.Payment.Checkout)))
	mux.Handle("GET /api/payments/status", payLimit(http.HandlerFunc(d.Payment.Status)))
	mux.HandleFunc("POST /api/payments/webhook", d.Payment.Webhook)

	// Signed-in.
	mux.Handle("GET /api/me", RequireUser(http.HandlerFunc(me)))

	// The account's security settings: devices, sign-in history, passkeys.
	mux.Handle("GET /api/account/sessions", RequireUser(http.HandlerFunc(d.Auth.Sessions)))
	mux.Handle("DELETE /api/account/sessions/{id}", RequireUser(http.HandlerFunc(d.Auth.RevokeSession)))
	mux.Handle("GET /api/account/activity", RequireUser(http.HandlerFunc(d.Auth.Activity)))
	mux.Handle("GET /api/account/passkeys", RequireUser(http.HandlerFunc(d.Auth.Passkeys)))
	mux.Handle("POST /api/account/passkeys/begin", RequireUser(http.HandlerFunc(d.Auth.PasskeyRegisterBegin)))
	mux.Handle("POST /api/account/passkeys/finish", RequireUser(http.HandlerFunc(d.Auth.PasskeyRegisterFinish)))
	mux.Handle("DELETE /api/account/passkeys/{id}", RequireUser(http.HandlerFunc(d.Auth.DeletePasskey)))
	mux.Handle("PUT /api/account/require-passkey", RequireUser(http.HandlerFunc(d.Auth.RequirePasskey)))

	// Operator-scoped: require a session and hand the job handlers the
	// operator id, so the job package never imports the auth layer.
	operator := func(h http.HandlerFunc) http.Handler {
		return RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := UserFrom(r.Context())
			ctx := job.WithUserID(r.Context(), u.ID)
			if u.Role == user.RoleAdmin {
				ctx = job.WithQuotaExempt(ctx)
			}
			h(w, r.WithContext(ctx))
		}))
	}
	mux.Handle("POST /api/jobs", long(operator(d.Job.Create)))
	mux.Handle("GET /api/jobs", operator(d.Job.List))
	mux.Handle("GET /api/usage", operator(d.Job.Usage))
	mux.Handle("GET /api/jobs/{id}", operator(d.Job.Get))
	mux.Handle("DELETE /api/jobs/{id}", operator(d.Job.Delete))
	mux.Handle("GET /api/jobs/{id}/assets/{assetId}/content", long(operator(d.Job.Content)))

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

	// Pages SADE itself serves, plus the dev server the CORS list admits.
	origins := append([]string{d.Cfg.App.PublicURL, d.Cfg.App.FrontendURL}, d.Cfg.Server.CORSAllowedOrigins...)
	guard := newOriginGuard(origins, "/api/payments/webhook")

	return chain(mux,
		Recover(d.Log),
		RequestLog(d.Log, trust),
		SecurityHeaders(trust, buildCSP(d.Cfg.App.FrontendDir)),
		CORS(d.Cfg.Server.CORSAllowedOrigins),
		OriginCheck(guard),
		Auth(d.AuthSvc, d.Cfg.Auth.SessionCookieName, trust),
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
