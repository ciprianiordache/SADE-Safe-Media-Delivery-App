package app

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"time"

	"sade/internals/app/auth"
	"sade/internals/app/user"
	"sade/internals/httpx"
	"sade/internals/ratelimit"
)

// middleware is the standard "wrap a handler" shape.
type middleware func(http.Handler) http.Handler

// chain applies middlewares so the first listed runs outermost.
func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// --- request-scoped user ---------------------------------------------------

type ctxKey int

const userKey ctxKey = iota

func withUser(ctx context.Context, u user.Response) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFrom returns the authenticated user attached by Auth, if any.
func UserFrom(ctx context.Context) (user.Response, bool) {
	u, ok := ctx.Value(userKey).(user.Response)
	return u, ok
}

// --- middlewares ---------------------------------------------------------

// Auth resolves the session cookie and, when valid, attaches the user to the
// request context. It never rejects - RequireUser / RequireRole do that.
func Auth(svc *auth.Service, cookieName string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err == nil {
				if u, aErr := svc.Authenticate(r.Context(), c.Value); aErr == nil {
					r = r.WithContext(withUser(r.Context(), u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser rejects a request that has no authenticated user with 401.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); !ok {
			httpx.Error(w, http.StatusUnauthorized, "not signed in")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole rejects a request whose user is not one of roles with 403. It
// assumes RequireUser ran first.
func RequireRole(roles ...string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFrom(r.Context())
			if !ok || !slices.Contains(roles, u.Role) {
				httpx.Error(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLog logs one line per request with status and duration.
func RequestLog(log *slog.Logger, trust proxyTrust) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Info("http request",
				"method", r.Method, "path", r.URL.Path,
				"status", sw.status, "bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", trust.clientIP(r),
			)
		})
	}
}

// Recover turns a panic in a handler into a 500 and a logged stack trace.
func Recover(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					log.Error("panic in handler",
						"panic", p, "path", r.URL.Path, "stack", string(debug.Stack()))
					httpx.Error(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS answers preflight requests and, for an allowed Origin, adds the
// cross-origin headers the SvelteKit dev server needs (credentials on, so no
// wildcard origin).
func CORS(allowed []string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(allowed, origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Vary", "Origin")
				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit rejects a request with 429 once its client has hit limiter's cap
// within its window - wraps a single route (POST /api/auth/request), not
// the global chain, since it's the one endpoint that both emails someone
// and reveals account existence via timing/side effects otherwise.
//
// trust resolves who "its client" is: behind a TLS terminator every request
// arrives from loopback, so without it the per-IP cap would be one global
// bucket shared by the whole internet.
func RateLimit(limiter *ratelimit.Limiter, trust proxyTrust) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(trust.clientIP(r)) {
				httpx.Error(w, http.StatusTooManyRequests, "too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets the response headers that are the same on every
// route. HSTS is conditional: it is only meaningful - and only safe - once
// the client actually reached us over HTTPS, so it is keyed on the
// forwarded scheme rather than sent unconditionally, which would otherwise
// pin a plain-HTTP dev host to a scheme it cannot serve.
func SecurityHeaders(trust proxyTrust) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if trust.scheme(r) == "https" {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter records the status code and byte count for RequestLog.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}
