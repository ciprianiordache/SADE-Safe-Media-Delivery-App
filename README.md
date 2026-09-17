# SADE — Safe Media Delivery

SADE lets an operator upload a video/audio/image file, watermarks it
asynchronously, and emails the recipient a signed link to the watermarked
preview — no account needed on the recipient's end. The recipient can pay
(Stripe) to unlock the clean original.

Flow: operator signs in via magic link → uploads a file from the dashboard →
the job is queued → a worker watermarks it in the background → the recipient
gets emailed a `/p/<token>` (view) and `/d/<token>` (download) link to the
preview → they can pay to unlock the original via a third signed link,
`/o/<token>`.

## Architecture at a glance

- **Backend**: Go, no ORM. `internals/database` wraps two libraries over the
  same `db`-tagged structs — [schema-builder](https://github.com/ciprianiordache/schema-builder)
  migrates the schema at boot, [crud-depot](https://github.com/ciprianiordache/crud-depot) does
  runtime CRUD. Postgres in production, SQLite in tests.
- **Frontend**: SvelteKit 2 / Svelte 5, built as a static SPA (`adapter-static`)
  and served by the Go binary itself — one process, one deploy artifact.
- **Worker**: an in-process pool polls for pending jobs and drives them through
  `ffmpeg`/`ffprobe` (shelled out directly, no wrapper library).
- **Payments**: Stripe Elements (a `PaymentIntent` + the client-side Payment
  Element embedded in SADE's own page), not a redirect to Stripe's hosted
  Checkout.

See `CLAUDE.md` for the full per-domain breakdown and the project's decision
history; `docs/SADE-plan.pdf` for the original build plan.

## Prerequisites

- Go 1.22+
- Node 18+ (for the frontend build)
- Docker (for the local Postgres, via `docker-compose.yml`) — or point
  `DATABASE_*` at any Postgres instance you already have
- `ffmpeg` + `ffprobe` on `PATH` (or absolute paths via config) — optional at
  startup: without them the app still serves, uploads just stay `pending`
  forever since nothing watermarks them

## Quick start (development)

```bash
docker compose up -d          # Postgres on host port 5433
go run .
```

On first run, `go run .` generates `config.yaml` and `.env` next to the
binary (defaults layered under `config.yaml`, secrets under `.env`) if they
don't already exist. In an interactive terminal it prompts for required
secrets; non-interactively it writes a `CHANGE_ME` placeholder and refuses to
start until you fill it in. At minimum, set in `.env`:

```
DATABASE_PASSWORD=postgres   # matches docker-compose.yml's POSTGRES_PASSWORD
```

`AUTH_HMAC_SECRET` is self-generated (`generate:"rand32"`) — no action
needed. `MAILER_TRANSPORT` defaults to `log`, so a fresh install works
end to end locally with magic links and preview-ready emails rendered to the
log instead of actually sent; switch it to `smtp` (with `SMTP_*`) to send
real mail.

The Go server listens on `:8080` by default (`SERVER_PORT`). Without a
frontend build, only the JSON API and the public `/p`, `/d`, `/o` share
routes are served.

### Frontend

```bash
cd frontend
npm install
npm run build      # outputs to frontend/build, gitignored
```

`internals/app/static.go` serves `frontend/build` (path configurable via
`APP_FRONTEND_DIR`) with SPA fallback, so `go run .` from the repo root then
serves the whole app — dashboard included — from `http://localhost:8080`.
For frontend-only iteration, `npm run dev` runs a dev server against the Go
API directly (CORS is configured for `http://localhost:5173`).

### Payments (optional)

Payment is entirely opt-in: with no `STRIPE_SECRET_KEY` set, the unlock
section is simply hidden and the rest of the app is unaffected. To exercise
it with Stripe's test mode:

```
STRIPE_SECRET_KEY=sk_test_...
STRIPE_PUBLISHABLE_KEY=pk_test_...
```

Webhook-driven confirmation (needed for a payment to complete when nobody's
page is open to trigger the fallback reconciliation check) additionally
needs `STRIPE_WEBHOOK_SECRET`. Locally:

```bash
stripe listen --forward-to localhost:8080/api/payments/webhook
```

prints the `whsec_...` value to set. A single local tester doesn't strictly
need this — `payment.Service.Status` reconciles a still-pending payment
against Stripe directly on the recipient's own page load.

### HTTPS

The browser marks `http://` as "not secure", and some of the app genuinely
needs a secure origin to behave: the session cookie is only worth marking
`Secure` over HTTPS, and Stripe refuses to collect card details on a plain
page outside `localhost`. Testing on a LAN address (a phone pointed at
`http://192.168.1.x:8080`) hits both.

**Recommended locally: a Cloudflare quick tunnel.** It terminates TLS at
Cloudflare's edge with a real, publicly trusted certificate and forwards to
the local server, so every device works with nothing installed on it —
unlike a self-signed certificate, whose CA has to be trusted per device.

```powershell
winget install --id Cloudflare.cloudflared   # once
./scripts/tunnel.ps1                         # leave running
go run .                                     # in a second terminal
```

`scripts/tunnel.ps1` waits for the `https://<random>.trycloudflare.com`
hostname Cloudflare assigns and writes it into `.env` as `APP_PUBLIC_URL`,
`APP_FRONTEND_URL` and `SERVER_CORS_ALLOWED_ORIGINS`, plus
`AUTH_SESSION_COOKIE_SECURE=true`. Those must be set **before** the app
starts: `APP_PUBLIC_URL` is what goes into every emailed magic-link and
share link, so a stale value emails links pointing at the old address. Pass
`-NoEnvUpdate` to print the URL without touching `.env`.

A quick tunnel gets a new hostname on every run. For a stable one — which a
Stripe webhook endpoint needs, since it is registered once in the Stripe
dashboard — use a named tunnel on a Cloudflare-managed domain
(`cloudflared tunnel create sade` / `route dns` / `run`).

**Terminating TLS in the Go server instead** (a real certificate, or a
self-signed one for a LAN IP) needs no tunnel:

```
SERVER_TLS_ENABLED=true
SERVER_TLS_CERT_FILE=/path/fullchain.pem
SERVER_TLS_KEY_FILE=/path/privkey.pem
```

#### Behind a proxy

Whenever something else terminates TLS, requests reach the Go server from
loopback over plain HTTP, with the real client in `X-Forwarded-For` and the
real scheme in `X-Forwarded-Proto`. `SERVER_TRUSTED_PROXIES` (default
`127.0.0.1/32,::1/128`) lists the peers whose forwarded headers are
believed; anything else is read from the connection itself, so a client
reaching the server directly can never forge an address past the
`POST /api/auth/request` rate limiter. Set it to your terminator's address
if it is not on this machine, or to nothing at all to disable the trust.

Note that a terminator does not make the local hop HTTPS. `Secure` cookies
and the HSTS header are keyed on the *forwarded* scheme, which is why
`AUTH_SESSION_COOKIE_SECURE=true` is correct behind a tunnel even though
the server itself is answering plain HTTP.

## Commands

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .
```

```bash
go run ./cmd/watermark -in FILE [-out FILE] [-kind logo|text|both] [-text "..."]
```
runs the watermark engine on one file with a live progress bar — a manual
smoke test of `internals/ffmpeg` independent of the rest of the app; needs
`ffmpeg`/`ffprobe` on `PATH`.

Frontend (from `frontend/`): `npm run dev`, `npm run build`, `npm run
preview`, `npm run check` (type checking; no separate lint step).

## Testing

`go test ./...` runs the full suite, including:

- `internals/app/integration_test.go` — the full model graph (every table,
  every foreign key, `ON DELETE CASCADE`) against a real (SQLite) DB.
- `internals/app/router_test.go` and `internals/app/worker_flow_test.go` —
  real HTTP round trips over `httptest`: login → callback → cookie →
  `/api/me` → logout, upload → list → detail, a Stripe-mocked checkout →
  unlock, and a full worker run (a fake `ffmpeg.Engine`, no binary needed)
  that claims a real pending job, watermarks it, and delivers a working
  `/p`/`/d` link end to end.
- Package-local unit tests per domain (`repo`/`service`/`handler`) and
  supporting package (`ratelimit`, `token`, `storage`, `mailer`, `ffmpeg`'s
  argument-building, `worker`'s retry/backoff state machine with fakes).

A few tests that need real `ffmpeg`/`ffprobe` binaries (e.g.
`job.TestCreateVerifiesMediaAgainstExtension`, `ffmpeg.TestWatermarkIntegration`)
skip automatically when those aren't on `PATH`, rather than failing.

## Status

M1–M6 in progress — the whole app works end to end (upload → watermark →
signed preview link → optional paid unlock of the original). See
`CLAUDE.md`'s "Resume here" section for exactly what's shipped and what's
left on the M6 hardening pass (currently: a retry/backoff review, done; rate
limiting on the magic-link request route, done; ffprobe-based upload
validation, done; remaining: wiring the frontend build into deployment, and
production Stripe webhook registration).
