# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

**M1–M5 done, plus the `payment` unlock. The whole app works end to end, backend and frontend:**
an operator signs in via magic link, uploads a file from the SvelteKit dashboard → the job is
stored → the worker watermarks it async → the preview is stored → the recipient is emailed
signed `/p/<token>` (view) + `/d/<token>` (download) links → the public `share` handlers stream
the watermarked preview with Range support, no login → the recipient's `/preview/[token]` page
embeds/downloads it, and can pay (Stripe, via an embedded Payment Element - SADE's own design,
not a Stripe-hosted page) to unlock the clean original via a third signed purpose, `/o/<token>`.
The operator's own dashboard can also play both the original and
the preview of their own upload, session-authenticated, from the job detail page. Only hardening
(M6) remains.

- `config/` — full loader (defaults < YAML < env), `config.Duration`, secret generation, `Validate`.
  `Auth.ShareTokenTTL` (default 30d) bounds the signed `/p` and `/d` links. `Auth.RequestRateLimit`/
  `RequestRateWindow` (default 5 per 15m) bound `POST /api/auth/request` per client IP - `0`
  disables the limiter.
- `internals/logger/` — `slog` + lumberjack; `internals/database/` — pool + `Connect` + `Migrate` +
  shared `CRUD()` + `Driver()`; `internals/server/` — synchronous-bind HTTP lifecycle.
- Root `main.go` + `app.go` — `NewApp` → `Run` → `Close`; builds logger, connects+migrates the DB,
  builds the mailer + `storage` + the `token` signer, wires the `user` + `auth` + `job` + `share`
  domains, builds the `ffmpeg` engine + the `worker` pool, serves `app.NewRouter(...)`. `App.Run`
  starts the worker, runs the server, then stops the worker within `Worker.ShutdownGrace`. If
  `ffmpeg.New` fails (binaries missing) the worker is skipped and the app still serves — uploads
  just stay `pending` (the rest of the app, `/p` and `/d` included, is unaffected).
- `internals/app/<domain>/model.go` for all six domains + `internals/app/models.go` (`Models()`).
- **`internals/app/user/`** — full stack (repo/service/handler + tests).
- **`internals/app/auth/`** — magic-link flow (`RequestLink` / `Complete` / `Authenticate` /
  `Logout`) + handler (`POST /api/auth/request`, `GET /api/auth/callback`, `POST /api/auth/logout`).
- **`internals/app/job/`** — full stack. `Service.Create` validates the recipient/kind, detects
  media type from the filename extension against `config.Upload.Allowed*`, streams the upload into
  `storage` (`originals/<jobID>/original<ext>`) with a sha256 + size-cap `io.TeeReader`/`LimitReader`,
  then (given an `*ffmpeg.Engine` - optional, wired the same way as the worker's, nil when
  ffmpeg/ffprobe are missing) ffprobes the stored blob via `storage.LocalPath` and rejects a
  mismatch between the extension's media type and ffprobe's own classification as
  `ErrCorruptMedia` (a renamed/corrupt file that cleared the extension check but isn't actually
  decodable, or decodes to a different stream kind), writes the `Job` row (`status=pending`) + the
  original `asset.Asset` row, and rolls both back (blob + row) on any later failure - including a
  failed `verifyMedia`. `Get`/`List` are owner-scoped (a non-owner's job reads as 404).
  Handler: `POST /api/jobs` (multipart `file` + `recipientEmail`/`watermarkKind`/`watermarkText`/
  `watermarkOpts`, `MaxBytesReader`-guarded), `GET /api/jobs`, `GET /api/jobs/{id}`,
  `GET /api/jobs/{id}/assets/{assetId}/content` (`Service.Asset` owner-scopes it the same way as
  `Get`, then the handler streams via `storage` — `http.ServeContent` when seekable, same Range/
  nosniff treatment as `share`; `?dl=1` for `attachment` instead of `inline`). Unlike `/p`/`/d`,
  this is session-authenticated and serves either asset kind, so the dashboard's job-detail page
  can play the operator's own original *and* preview inline, side by side — no signed token
  involved, since the operator already owns the job.
  `Repo` also carries the worker-facing state machine (raw SQL, dialect-aware via `db.Driver()`):
  `ClaimPending` (atomic `UPDATE … RETURNING`, `FOR UPDATE SKIP LOCKED` on Postgres, bumps
  `attempts`), `MarkDone` / `MarkFailed` / `MarkForRetry(nextAttemptAt)` / `ResetStuck(cutoff)`.
  `Job` gained a `next_attempt_at` column (zero = ready now) gating (re)claims for retry backoff.
- **`internals/app/asset/`** — repo only (`Create` / `GetByID` / `ListByJob` / `GetByJobAndKind` /
  `Delete`), like `magic_token`/`session`. Exported `ToResponse`/`ToResponses` so `job` renders
  asset rows in its detail response. No handler — assets reach clients via `share`.
- **`internals/app/share/`** — the public, login-free share links. `Handler.Preview` (`GET
  /p/{token}`, `Content-Disposition: inline`), `Handler.Download` (`GET /d/{token}`,
  `attachment`), and `Handler.Original` (`GET /o/{token}`, `attachment`) all funnel through one
  `serve(w, r, purpose, wantKind, disposition)`: `signer.Verify(purpose, token)` (`"preview"` for
  `/p`, `"download"` for `/d`, `"original"` for `/o`) → subject is an asset id → `Assets.GetByID`
  → must be `wantKind` (`KindPreview` for `/p`+`/d`, `KindOriginal` for `/o` — a token minted for
  one purpose can never open a route expecting a different kind) → stream from `Blob`. Local files
  go through `http.ServeContent` (Range / conditional GET, `Accept-Ranges`, `nosniff`); a
  non-seekable backend falls back to a whole-object copy. Error map: `token.ErrExpired` → 410,
  everything else (bad/wrong-purpose token, unknown asset, wrong kind, missing blob) → an
  indistinguishable 404; repo error → 500. Declares its own `Assets` + `Blob` ports (`asset.Repo`
  and `storage.Storage` satisfy them directly — no adapters). The `"original"` purpose is never
  minted by `share` itself — only `internals/app/payment.Service.Status`, once a job's `Payment`
  is `StatusPaid`.
- **`internals/app/payment/`** — full stack, backed by Stripe **Elements** (`github.com/stripe/
  stripe-go/v82`), not Stripe's hosted Checkout page: card entry happens client-side against a
  `PaymentIntent`'s client secret, via the Payment Element mounted directly into
  `/preview/[token]` (`PaymentForm.svelte`), so the surrounding page — and, via the Element's
  `appearance` API, the form itself — stays SADE's own design rather than a Stripe-branded
  redirect. `internals/app/payment` itself never sees card data; it only ever handles a
  PaymentIntent id and its status. Public routes, authorised by the same signed `"preview"` token
  the recipient's `/preview/[token]` page already holds (never a session): `POST
  /api/payments/checkout` (route name kept for stability; body `{token}` → resolves the preview
  asset → its job → its `KindOriginal` sibling via `asset.Repo.GetByJobAndKind`, records a
  `Payment{Status: pending}`, creates a Stripe PaymentIntent for `config.PaymentConfig.PriceCents`/
  `Currency` with `AutomaticPaymentMethods` enabled, returns `{clientSecret}`), `GET
  /api/payments/status?token=` (reports `{enabled, paid, originalUrl?, amountCents, currency,
  publishableKey?}` — the `/preview` page calls this on load, to decide whether to fetch a
  PaymentIntent and mount the form at all, and again after `PaymentForm` confirms), and `POST
  /api/payments/webhook` (Stripe-signature-verified `payment_intent.succeeded` handling).
  **Two ways to reach `StatusPaid`, both idempotent, both landing on the same row**: the webhook
  (needs `StripeWebhookSecret` + a publicly reachable URL — not true of a laptop in dev), or
  `Service.Status` itself reconciling a still-`pending` payment by calling
  `V1PaymentIntents.Retrieve` on its own `ProviderRef` — so the recipient's *own page load*
  completes the unlock even with no webhook wired (or even without waiting for `PaymentForm`'s
  `confirmPayment` to resolve, for a redirect-based method), which is the common case locally.
  Once paid, `Status` mints an `"original"`-purpose token via `internals/token` and hands back
  `/o/<token>` (see the `share` bullet above). **Config is opt-in, not startup-fatal**: an empty
  `StripeSecretKey` only disables this domain (`CreateIntent` returns `ErrDisabled` → 503;
  `Status` reports `enabled: false` so the frontend hides the whole unlock section) — mirrors
  ffmpeg's optionality, not Auth's hard-fail secrets. An empty `StripeWebhookSecret` independently
  disables just the webhook route (`ErrDisabled` → 501) rather than processing an event it cannot
  verify. `StripePublishableKey` is not a secret (Stripe's own naming) — `Status` hands it to the
  frontend outright, which is what `PaymentForm.svelte` needs to call `loadStripe(...)`.
- **`internals/app/magic_token/` + `internals/app/session/`** — repos (`Consume` is atomic
  delete-on-use; `GetByHash` / `Delete`).
- **`internals/app/router.go` + `middleware.go`** — central mux, `Recover`/`RequestLog`/
  `SecurityHeaders`/`CORS`/`Auth` chain, `RequireUser` / `RequireRole`, `UserFrom(ctx)`, `GET /api/me`. The `operator` wrapper
  guards the `/api/jobs*` routes with `RequireUser` and passes the operator id down via
  `job.WithUserID(ctx, id)` (keeps `job` from importing the auth layer). `GET /p/{token}` and
  `GET /d/{token}` are public (no cookie) — the signed token in the path is the whole authz.
  `POST /api/auth/request` alone is wrapped in `RateLimit(d.AuthRateLimiter)` (per-route, not the
  global chain - it's the one endpoint that emails on every well-formed address, so it's the one
  an attacker could use to spam a mailbox or hammer the DB): the client is resolved by
  `proxy.go`'s `proxyTrust.clientIP` (the forwarded client when the peer is one of
  `Server.TrustedProxies`, the raw `r.RemoteAddr` host otherwise) and a cap hit returns 429. The
  global chain also carries `SecurityHeaders` (nosniff / `X-Frame-Options` / `Referrer-Policy`,
  plus HSTS only when the forwarded scheme is https).
  `internals/app/static.go` (`spaFileServer` + `hasFrontendBuild`) registers `mux.Handle("/", …)`
  last, serving `Cfg.App.FrontendDir` (default `./frontend/build`) with adapter-static's
  `index.html` fallback for client-side routes; it 404s (never the shell) for an unmatched path
  under `/api/`, `/p/`, `/d/`, `/healthz`, and is skipped entirely when the dir doesn't exist yet.
  Tested in `internals/app/static_test.go`.
- **`internals/worker/`** — in-process pool, wired in `app.go`. `New(cfg, JobStore, AssetStore,
  Blob, Engine, Notifier, log)` over small interfaces the pool declares itself; `app.go` binds them
  with `jobStoreAdapter`/`assetStoreAdapter` (in `worker_wiring.go`) + `storage` + `*ffmpeg.Engine`
  + `worker.EmailNotifier`. `Start` runs `ResetStuck` once then a dispatcher (polls
  `ClaimPending` every `PollInterval`, batches of `ClaimBatchSize`) feeding `Concurrency`
  processor goroutines over a channel. `process` = load original → `LocalPath` → `engine.Probe`
  → `engine.Watermark` into `previews/<jobID>/<base>-preview.<ext>` (`.mp4` video / `.m4a` audio /
  source ext image) → `Stat` + sha256 → `asset.AddPreview` → `Notifier.PreviewReady` (a failed
  email is logged, not fatal). Outcome: `MarkDone`, or `MarkForRetry` with `RetryBackoff<<(attempt-1)`
  while `attempts <= MaxRetries`, else `MarkFailed`. Per-job `context` is detached from shutdown
  (bounded by `JobTimeout`); `Stop` waits out `ShutdownGrace`. Only the local storage backend is
  supported (needs `LocalPath`); S3 staging is a TODO.
- `internals/httpx/` — shared HTTP helpers. `internals/testutil/` — `DB(t, models...)` + `Logger()`.
- `internals/ratelimit/` — a small in-process, per-key sliding-window `Limiter` (`New(limit,
  window)` / `Allow(key) bool`, no Redis - fine for SADE's single process). `limit <= 0` or a nil
  `*Limiter` disables it (`Allow` always `true`), so wiring code builds one straight from config
  with no branch. Only consumer so far: `app.go`'s `authRateLimiter`, guarding
  `POST /api/auth/request` via `RateLimit` in `internals/app/middleware.go`.
- `internals/storage/` (local disk, wired via `job` + `worker` + `share`), `internals/ffmpeg/`
  (`os/exec` engine + `-progress` parsing, wired via `worker`), `internals/mailer/`
  (`smtp`/`log`/`noop`), `internals/token/` (HMAC share tokens, wired via `share` for verify;
  `worker.EmailNotifier` signs `"preview"`/`"download"`, `payment.Service` signs `"original"` —
  three domain-separated purposes over the one `Auth.HMACSecret`).
- `cmd/watermark/` — CLI that runs the engine on one file with a live progress bar.
- `docker-compose.yml` — Postgres on host port 5433.
- `internals/app/integration_test.go` (full-graph round-trip) + `router_test.go` (real
  request→callback→cookie→`/api/me`→logout, the full job upload→list→detail flow, a
  seed-preview→sign→`/p`+`/d` fetch, the session-authenticated job-asset-content route, and a full
  Stripe-mocked checkout→reconcile→`/o/{token}` unlock, all over `httptest`).

Nothing left unstarted at the domain level — every table in the schema is wired end to end.
`mailer` is wired via `auth` + `worker`; `storage` via `job` + `worker` + `share`; `ffmpeg` via
`worker`; `token` via `share` + `worker` + `payment`; `stripe-go` via `payment` only.

### Resume here (if the session reset)

**Last shipped:** HTTPS. The user's report was the browser's "conexiune nesecurizată" on
`http://192.168.1.179:8080` (the LAN address the app had been demoed from). The TLS listener
itself already existed and was never the gap - `config.ServerConfig.TLS` +
`internals/server.Start`'s `tls.NewListener` (`internals/server/main.go:62`) have been there since
M0. What was missing was a *certificate* anyone would trust and, more importantly, correct
behaviour once something else terminates TLS in front of the app. Chosen approach, asked first and
picked by the user out of {Cloudflare tunnel, self-signed LAN cert, Let's Encrypt on a real
domain}: **a Cloudflare tunnel**. A self-signed cert for a LAN IP fixes "insecure" only
technically - every device still warns until its CA store is edited by hand, which on a phone is
the worst possible dev loop; a tunnel gets a real, publicly trusted cert on every device with
nothing installed on them, and as a bonus gives Stripe's webhook a reachable URL (see the
`payment` bullet: local dev has been leaning on `Status`'s own reconciliation for exactly this
reason).

The code work is the proxy-awareness that a terminator forces, not the TLS:

- **`internals/app/proxy.go`** (new) - `proxyTrust`, parsed once in `NewRouter` from
  `Server.TrustedProxies`. `clientIP(r)` returns the forwarded client when the *immediate peer* is
  a trusted proxy and the raw peer otherwise; the `X-Forwarded-For` chain is walked right to left
  and stops at the first hop that isn't itself a trusted proxy. `scheme(r)` does the same for
  `X-Forwarded-Proto` (and `r.TLS != nil` when SADE terminates TLS itself). Trust is never
  extended past the peer, so headers are never a way to forge an identity.
- **The bug this fixes**: `middleware.clientIP` read `r.RemoteAddr` and explicitly said "SADE
  isn't deployed behind a reverse proxy yet". Behind any terminator every request arrives from
  loopback, so the per-IP cap on `POST /api/auth/request` would have collapsed into a single
  global bucket - 5 requests per 15 minutes for the entire internet. `RateLimit` and `RequestLog`
  now both take a `proxyTrust`.
- **`config.ServerConfig.TrustedProxies`** (`SERVER_TRUSTED_PROXIES`, default
  `127.0.0.1/32,::1/128`). Defaulting to loopback rather than empty is deliberate: the only thing
  that can exploit it is a process on this machine, which already has the config and the database,
  and it makes every tunnel/nginx/Caddy deployment correct with no extra step. `config.Validate`
  rejects an entry that is neither an IP nor a CIDR.
- **`SecurityHeaders` middleware** (in the global chain, between `RequestLog` and `CORS`):
  `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, and HSTS
  **only when `trust.scheme(r) == "https"`** - sending it unconditionally would pin a plain-HTTP
  dev host to a scheme it cannot serve. Deliberately no CSP yet: Stripe Elements needs
  `js.stripe.com` in `script-src`/`frame-src` and getting that wrong silently breaks card entry,
  so it wants its own pass.
- **`scripts/tunnel.ps1`** (new) starts `cloudflared tunnel --url`, waits for the assigned
  `*.trycloudflare.com` hostname, and writes it into `.env` as `APP_PUBLIC_URL` /
  `APP_FRONTEND_URL` / `SERVER_CORS_ALLOWED_ORIGINS` plus `AUTH_SESSION_COOKIE_SECURE=true`,
  updating those keys in place and leaving every other line (the secrets) untouched. Ordering
  matters and the script's doc comment says so: `APP_PUBLIC_URL` is baked into every emailed
  magic-link and share link, so it has to be right *before* the app starts, not after.
- Nothing changed in the frontend: `api.ts`'s `API_BASE` is already `''` in the production build
  (same origin), which is exactly what the tunnel serves.

New tests in `internals/app/proxy_test.go`: spoofed `X-Forwarded-For` from an untrusted peer is
ignored, the same header from loopback is honoured, the chain is walked from the right past
trusted hops, `X-Forwarded-Proto` is only believed from a trusted peer, HSTS appears only on a
forwarded-https request, and the rate limiter keys per forwarded client rather than collapsing.
Verified live, not just in tests: `cloudflared` installed via winget, `scripts/tunnel.ps1` ran and
produced a working `https://...trycloudflare.com` that returned 200 from `/healthz` through a real
Cloudflare certificate, and the new binary confirmed on a second port that HSTS is absent on a
plain request, present on an `X-Forwarded-Proto: https` one, and that a forwarded client IP
(`198.51.100.77`) reaches the request log in place of `::1`.

**Still open here:** a CSP (see above), and `config.yaml` at the repo root still carries the old
`public_url: http://192.168.1.179:8080` / `session_cookie_secure: false` - `.env` overrides both
(defaults < YAML < env) and the tunnel script writes `.env`, so this is stale rather than wrong,
but it is the value any run without the script falls back to.

**Before that:** fixed the emailed logo not loading, reported right after the HTML-email work
below shipped. Cause: the `<img>` pointed at `PublicURL/logo.png` (`PublicURL` is currently this
dev machine's LAN address, `192.168.1.179:8080` - see the LAN-access entry further down), and
Gmail (like most webmail) never fetches a remote image directly - it routes every one through
Google's own image-proxying servers, which run on the public internet and have no route to a
private/LAN address. No amount of "display images" trust would have fixed that; the URL was
categorically unreachable. Fixed by embedding the logo in the message itself instead of linking
to it: `internals/mailer.Message` gained an `Inline []Inline` field (`CID`, `ContentType`,
`Data`), and `build()` now wraps the existing `multipart/alternative` (text/html) inside an outer
`multipart/related` when `Inline` is set, with each image as a sibling part carrying a
`Content-ID` and base64 body (RFC 2045 76-column wrapping via new `writeBase64Lines`) - the
standard way transactional mail ships a logo, and immune to this failure mode in dev *or*
production. `internals/emailtmpl` now embeds the PNG directly (`//go:embed logo.png` -> exported
`LogoPNG []byte`, `LogoCID = "sade-logo"`) and the template references it as `cid:sade-logo`;
`Data.PublicURL` was dropped entirely (nothing else used it - every button/fallback URL was
already absolute, passed in by the caller). Both `auth.Service.RequestLink` and
`worker.EmailNotifier.PreviewReady` now attach `mailer.Inline{CID: emailtmpl.LogoCID, ...,
Data: emailtmpl.LogoPNG}` alongside the HTML body. `frontend/static/logo.png` (added for the
abandoned URL approach, in the same session) is removed again - nothing else referenced it, the
app's own UI uses the separate Vite-bundled copy at `frontend/src/lib/assets/logo.png`.
`cmd/emailpreview` still renders to plain `.html` files for browser viewing, where a `cid:` URI
can't resolve, so it now post-processes the rendered HTML to swap `cid:sade-logo` for a `data:`
URI of the same embedded bytes - real mail never takes this path, it's just so the preview tool
(and the published Artifact preview below, republished after this fix) stays viewable outside a
mail client. New tests: `internals/mailer`'s `TestBuildRelatedWhenInline` (asserts the
`multipart/related`/`Content-ID`/base64 shape and round-trips the encoded bytes back to the
original), `internals/emailtmpl`'s existing render tests updated for `cid:` instead of a URL.
Verified live: both emails resent through Gmail SMTP with the fix in place.

**Earlier in the same arc:** both transactional emails (magic-link, preview-ready) now send a
branded HTML body alongside the existing plain-text one, instead of plain text only - the user's
explicit ask
("faceti aceste email-uri sa arate bine, nu plain text"). New package **`internals/emailtmpl`**
(`emailtmpl.go` + `emailtmpl_test.go`) is a small shared HTML layer - `Render(Data) (string,
error)` wraps a heading/intro/buttons/note into one branded, table-based, Outlook-safe layout
(`html/template`, so every dynamic value - watermark text, tokens - is auto-escaped), plus
`HumanDuration(time.Duration) string` to turn `15m0s`/`720h0m0s` into "15 minutes"/"30 days" for
the copy. It's a new shared package rather than duplicated per-domain (like `internals/httpx`,
`internals/token`) because `auth` (magic-link) and `worker` (preview-ready) both need the exact
same card/button chrome and shouldn't import each other. Palette/type/radius are copied verbatim
from `frontend/src/app.css`'s light tokens (warm parchment `#fbf8f4`, orange accent `#e8631c`, IBM
Plex Sans, `11px`-ish radius) - deliberately **not** theme-aware like the app itself: mail-client
dark-mode support is inconsistent enough to auto-invert a hand-tuned palette badly, so every email
ships one fixed light design (`<meta name="color-scheme" content="light">`), the same choice
transactional mail from Stripe/GitHub/etc. makes. (The logo first shipped as a `PublicURL/logo.png`
URL reference here - fixed to an embedded `cid:` inline part instead two entries up, once that
turned out unreachable from a LAN address; see that entry.) `internals/app/auth/service.go`'s
`RequestLink` and
`internals/worker/notify.go`'s `PreviewReady` both now build `mailer.Message.HTML` via
`emailtmpl.Render` next to their existing `Text` body (the mailer already supported
`multipart/alternative` - see the `mailer` bullet below - just nothing populated `HTML` before
this). New throwaway dev tool `cmd/emailpreview` (`go run ./cmd/emailpreview -out DIR`) renders
both templates with sample data to local `.html` files without sending real mail, for iterating on
the design; not wired into the app. Verified: real emails sent through the now-live Gmail SMTP
account (see two entries below) and a published Artifact preview
(https://claude.ai/artifact/UC5GavccwNPbfS4Se5DA8P) embedding the actual rendered output via
iframe, both against the exact HTML the server sends - not a separate mockup.

**Earlier in the same arc:** fixed the emailed preview link. `worker.EmailNotifier.PreviewReady`
(`internals/worker/notify.go`) used to mail the recipient the bare `PublicURL/p/<token>` asset
stream as the "View it" link - a gap CLAUDE.md itself had flagged as deliberate ("frontend
preview page may just embed/redirect to those") but the user, testing the app, flagged as wrong:
the recipient should land on the frontend's `/preview/<token>` page (the embedded player +
Stripe-unlock section), not the raw watermarked file. Changed the "View it" link to
`PublicURL/preview/<token>`, kept `PublicURL/d/<token>` for a direct download - no backend
route or token-purpose change needed, since `/preview/[token]` already builds its own `/p/` and
`/d/` calls client-side from the same "preview"-purpose token in the URL. Updated
`TestEmailNotifierSendsSignedViewAndDownloadLinks` (`internals/worker/notify_test.go`) and
`TestWorkerFlowEndToEndThroughRouter` (`internals/app/worker_flow_test.go`, which has no
frontend build in its test harness to actually render the view link, so it now resolves the
view link's token against `/p/{token}` directly to prove it's genuine, rather than fetching the
view link itself) to match. Verified live: uploaded a job, the resulting email's link opened the
SvelteKit preview page on another device on the LAN (see the two entries below for that LAN-access
and real-SMTP setup).

**Earlier in the same arc:** ran the app for the first time this session against real `ffmpeg`/`ffprobe`
binaries (present on this machine's PATH, unlike the dev container M6 was written in - see the
"Earlier" paragraph below), which surfaced a real bug in `internals/ffmpeg.Engine.Probe`'s
video/image classification: ffprobe's `image2` demuxer (what a plain `.jpg`/`.jpeg` goes through)
reports a nonzero `format.duration` - one frame at an assumed 25fps - unlike `png_pipe`/
`webp_pipe`/`tiff_pipe`, which report none. `Probe` classified on `DurationSec == 0` alone, so a
valid JPEG was misread as video, tripped `job.Service.verifyMedia`'s extension-vs-content check,
and every JPEG upload was rejected with `ErrCorruptMedia` (415, "file content does not match its
extension") - reported by the user after trying to upload one. Fixed by also accepting
`format_name` values that identify one of ffprobe's single-image demuxers (`isStillImageFormat`:
exactly `"image2"`, or any `"*_pipe"` name) as an image regardless of duration - safe because
`Probe` is only ever called on an already-stored single file, never an actual image-sequence
pattern. New unit test `TestIsStillImageFormat` (pure, no binary needed) pins the mapping for all
five of `UploadConfig.AllowedImage`'s extensions plus three common video container format names.
`TestCreateVerifiesMediaAgainstExtension` and `TestIsStillImageFormat` now both actually pass
against real binaries (previously only compiled/skip-tested, per the "Earlier" paragraph below).
`TestWatermarkIntegration` still fails on this particular Windows box specifically (`Fontconfig
error: Cannot load default config file`, an access violation from ffmpeg's own font loading with
no `FontPath` given) - an environment gap unrelated to this fix, not investigated further.
Separately, the user also reported a preview-ready email that never arrived despite the dashboard
showing the job `done`: not a bug - `Mailer.Transport` was still `log` (the dev default, per
`config.yaml`), so every prior "preview ready" notification, including the user's own earlier
manual tests, was only ever logged, never sent (a failed/skipped send is deliberately non-fatal -
see the `worker` bullet above - so it never blocks the job reaching `done`). Resolved by pointing
this machine's `.env` at real Gmail SMTP (`MAILER_TRANSPORT=smtp`, an app-password login) and
confirmed both the magic-link and preview-ready emails now actually send.

**Earlier in the same arc:** the first two items of M6's punch list - a per-client-IP rate limit on `POST
/api/auth/request` (`internals/ratelimit.Limiter`, wired as `app.go`'s `authRateLimiter` and
applied only to that one route via `RateLimit` in `middleware.go`; see the `router.go` and
`internals/ratelimit` bullets above) and ffprobe-backed upload validation in `job.Service.Create`
(`verifyMedia`, gated on an optional `*ffmpeg.Engine` the same way the worker's is - see the `job`
bullet above, `ErrCorruptMedia` → 415). `app.go` now builds the `ffmpeg.Engine` once, before
`job.NewService`, and reuses it for the worker instead of constructing it twice. Both are covered
by tests (`internals/ratelimit/limiter_test.go`, `router_test.go`'s
`TestAuthRequestIsRateLimited`, `job/service_test.go`'s `TestCreateVerifiesMediaAgainstExtension`
- the latter skips like `ffmpeg`'s own integration test when `ffmpeg`/`ffprobe` aren't on PATH,
which they weren't in the dev container this was written in, so it hasn't actually run green
against real binaries yet, only compiled and skip-tested). Followed in the same session by the
rest of M6's punch list - the retry/backoff review, a real full-worker-flow test, and a root
README - see the "M6 (hardening) status" entry further down for the details.

**Earlier in the same arc:** the `payment` domain (Stripe Checkout unlock of the original file)
end to end, plus a session-authenticated media player on the operator's own job-detail page. Both
were built in the same session as a direct response to the user playing with the redesigned
dashboard and asking, in order, "how do I know the watermark actually worked" (→ the
job-asset-content route + player) and "let's build payment before M6" (→ the whole `payment`
domain). See the `job`, `share` and `payment` bullets above for the mechanics; this entry is the
narrative/decision trail.

- The job-detail media player closes half of the redesign's "adapted from the mockup" gap noted
  below: the operator can now actually watch the watermarked preview (and the original) inline,
  even though there is still no copyable `/p`/`/d` link or resend button on that page.
- Payment's shape was a deliberate, asked-first set of calls, not a default: **Stripe** (not a
  home-grown gateway) in **test mode** (the user already had test-mode keys), **fixed global
  price** (`PaymentConfig.PriceCents`/`Currency`, not a per-upload price field — simpler, no
  `job` model change). The PaymentIntent-retrieve reconciliation path in `Service.Status` (as
  opposed to relying on the webhook alone) exists specifically because a local dev box has no
  public URL for Stripe's webhook to reach — without it, the unlock would never complete outside
  a deployment with a real domain and a configured webhook endpoint.
- **Switched from Stripe's hosted Checkout to Stripe Elements mid-session**, on the user's
  explicit ask after seeing the first version redirect to a Stripe-branded page: "is it possible
  to use our own design at the card-payment step?" Hosted Checkout (`stripe.CheckoutSession`,
  a redirect) cannot be restyled beyond a logo/color in the Stripe Dashboard; Elements' Payment
  Element embeds the actual (PCI-scoped, still Stripe-controlled) input fields *inside* SADE's own
  page, themeable via its `appearance` API to track `app.css`'s live CSS custom properties
  (including dark mode) - see `PaymentForm.svelte`. This is a real API surface change, not a
  restyle: `payment.Service.Checkout` → `CreateIntent` (`stripe.CheckoutSession` →
  `stripe.PaymentIntent`, `checkoutUrl` → `clientSecret` in the wire response), the webhook
  listens for `payment_intent.succeeded` instead of `checkout.session.completed`, and
  `config.PaymentConfig` gained `StripePublishableKey` (not secret - Stripe's own naming - handed
  to the frontend via `GET /api/payments/status`'s response, not a separate config-exposing
  endpoint). `frontendURL` dropped out of `payment.NewService`'s signature entirely: Checkout's
  success/cancel URLs don't exist for a PaymentIntent, and `confirmPayment`'s `return_url` (still
  needed for a redirect-based method, e.g. 3-D Secure) is built client-side in
  `/preview/[token]/+page.svelte` from `window.location.origin` - the backend has nothing to add.
- Not yet added: an env var actually holding real Stripe test keys in *this* checkout of the repo
  (`.env` is gitignored, per-machine) - the feature is fully wired and tested against a mocked
  Stripe backend (`internals/app/payment/service_test.go`, `internals/app/router_test.go`'s
  `TestPaymentUnlockFlowEndToEnd`), but a live end-to-end run needs `STRIPE_SECRET_KEY` +
  `STRIPE_PUBLISHABLE_KEY` (and, for webhook-driven confirmation specifically,
  `STRIPE_WEBHOOK_SECRET` — e.g. via `stripe listen --forward-to
  localhost:8080/api/payments/webhook` locally) set before `go run .`. Verified live end to end
  this session with the user's own Stripe test-mode keys and the `4242 4242 4242 4242` test card.

**Further back:** M5, the SvelteKit frontend (`frontend/src/`), plus the Go-side static
handler that serves it — then a full visual redesign of that same frontend against a Claude
Design canvas the user directed (link in their local session history, not reproduced here).
Backend + frontend now cover the whole flow M1–M5, restyled.

- Frontend routes built: `/` landing, `/login` (magic-link request + `?error=invalid_link`
  banner), `/app` (guarded layout + upload form + a searchable/filterable/sortable job list with
  list/grid views and comfortable/compact density), `/app/jobs/[id]` (detail with a status
  stepper + a real inline player for both its assets, polls while pending/processing),
  `/preview/[token]` (public, no auth — cascades `<video>` → `<audio>` → `<img>` → plain link
  against `GET /p/{token}`, downloads via `GET /d/{token}`; single column under 860px, a
  two-column `media | unlock` grid above it, with a taller/wider player - see "Design system"
  above; calls `GET /api/payments/status` on load and, when unpaid, `POST /api/payments/checkout`
  for a PaymentIntent to mount `PaymentForm.svelte`'s Payment Element; once paid, shows a download
  link to `GET /o/{token}` instead - the whole section is hidden when `Payment.enabled` is
  `false`). `src/lib/`: `api.ts` (typed fetch client, `credentials: 'include'`, `ApiError`),
  `types.ts` (hand-kept mirror of the Go `Response` DTOs + `isAllowedFile`), `format.ts`
  (`relativeTime`), `stores.svelte.ts` (`auth`, a class w/ `$state`), `theme.svelte.ts` +
  `i18n.svelte.ts` (ditto — Svelte 5 runes only work in `.svelte`/`.svelte.ts` files, so anything
  stateful is named `*.svelte.ts`, not the plain `.ts` the plan sketched), `components/`
  (`LangThemeToggle`, `MarkBar`, `StatusBadge`, `StatusDot`, `PaymentForm` — loads
  `@stripe/stripe-js`'s `loadStripe()`, themes the Payment Element from `app.css`'s live custom
  properties so it tracks light/dark mode, and calls `stripe.confirmPayment({ redirect:
  'if_required' })` so a card that needs no extra authentication resolves in-page instead of
  round-tripping through Stripe).
  `adapter-static` (`fallback: 'index.html'`) + root `+layout.ts` (`ssr = false`) make it a pure
  client-rendered SPA. `npm run check` is clean; `npm run build` produces `frontend/build`
  (gitignored, matches `internals/app/static.go`'s default `Cfg.App.FrontendDir`).
- Backend: added `config.AppConfig.FrontendDir` (`APP_FRONTEND_DIR`, default `./frontend/build`)
  and `internals/app/static.go` — see the `router.go` bullet above. `app.go` needed no change
  (`Deps.Cfg` already carries the whole config). No other backend changes for the redesign — the
  gaps between the design canvas and the shipped UI (list rows keyed on recipient not filename, no
  copyable `/p`/`/d` links or resend button on the job detail page, dashboard search/filter/sort/
  pagination done client-side) are deliberate: adapt-to-the-API-as-is was the chosen default
  rather than expanding the API to match the mockup. Revisit if the client-side-200-jobs ceiling
  or the missing operator-facing links become real problems.
- ~~Not wired: the worker's `EmailNotifier` still mails the raw `PublicURL/p/<token>` link~~ —
  fixed; see the `worker` bullet above and the "Resume here" entry below. The recipient now lands
  on the frontend page (player + Stripe unlock), not the bare media stream.

**M6 (hardening) status:** rate-limit on `POST /api/auth/request`, ffprobe-based upload
validation, the retry/backoff review, a full-flow integration test, and a README are all done
(see "Resume here" above and the entry right below it). The retry/backoff review
(`internals/worker/worker.go` + `internals/app/job/repo.go`'s `ClaimPending`/`MarkForRetry`/
`ResetStuck`) found the existing design already sound and made no changes: atomic claim via
`UPDATE ... RETURNING` (`FOR UPDATE SKIP LOCKED` on Postgres) so concurrent workers/processes
never double-claim a row, exponential backoff (`RetryBackoff<<(attempts-1)`, shift capped at 16 to
guard against overflow) gated by `MaxRetries`, and `ResetStuck` as a crash-recovery net for a row
left `processing` past `StuckJobTimeout`. `internals/app/worker_flow_test.go`'s
`TestWorkerFlowEndToEndThroughRouter` is the new full-flow test: it runs a real `worker.Pool`
(against a `fakeWatermarkEngine`, so no `ffmpeg` binary is required) claiming a job created
through the real HTTP upload route, then fetches the resulting `/p`/`/d` links - the one gap the
existing `httptest` suite had (every other share-link test seeds the preview asset directly
rather than letting a worker produce it). Root `README.md` covers quick start, config, commands,
and testing for a newcomer; `CLAUDE.md` (this file) stays the deeper architecture/decision record.
Also worth a pass: wire `APP_FRONTEND_DIR`/build into
whatever deploys this (the frontend needs `npm run build` before the Go binary can serve it — no
build step exists yet in `docker-compose.yml` or elsewhere). And now that `payment` is live: set
real `STRIPE_SECRET_KEY`/`STRIPE_WEBHOOK_SECRET` in whatever deploys this, and register that
deployment's `/api/payments/webhook` URL in the Stripe dashboard (local dev relies on `Status`'s
own reconciliation instead, per the payment bullet above — fine for one person testing, not a
substitute for the webhook in production, where a payment completed while nobody's page is open
to trigger a status check would otherwise sit `pending` until someone happens to reload).

- **Spec:** `Writerside/topics/` (`Default-topic.md` = product goal, `sever.md` = block components).
- **Agreed build plan:** `docs/SADE-plan.pdf` — read it before starting any feature. It defines
  the target file tree, the milestones (M0–M6), and every decision below.
- **Repo:** `github.com/ciprianiordache/SADE-Safe-Media-Delivery-App` (`origin`, branch `main`).
  Line endings are normalised to LF via `.gitattributes`. **Workflow: after every commit, also
  `git push origin main`** (the user wants GitHub kept in sync with each commit).

SADE (Safe Media Delivery): an operator uploads a media file (video/audio/image), the backend
applies a watermark asynchronously, then emails the recipient a signed link to the watermarked
preview. The recipient can pay (Stripe) to unlock the clean original.

## Commands

Go (from repo root):
- `go build ./...`
- `go test ./...`
- Single package / test: `go test ./internals/database -run '^TestMigrateAndCRUD$'`
- `go vet ./...` and `gofmt -l .`
- `docker compose up -d` — Postgres on host port **5433** (matches config defaults); the app
  fails fast at `db.Connect` if it is not running.
- `go run ./cmd/watermark -in FILE [-out FILE] [-kind logo|text|both] [-text "..."]` — apply the
  watermark to one file with a live progress bar. Manual test of `internals/ffmpeg`; needs
  `ffmpeg`/`ffprobe` on PATH.
- `./scripts/tunnel.ps1` - starts a Cloudflare quick tunnel, writes the resulting
  `https://*.trycloudflare.com` URL into `.env` (`APP_PUBLIC_URL`, `APP_FRONTEND_URL`,
  `SERVER_CORS_ALLOWED_ORIGINS`, `AUTH_SESSION_COOKIE_SECURE`), then leave it running and
  `go run .` in another terminal. Needs `winget install --id Cloudflare.cloudflared` once.
  `-NoEnvUpdate` prints the URL without touching `.env`.
- To exercise the real Stripe unlock (not just the mocked-backend tests): set `STRIPE_SECRET_KEY`
  (a `sk_test_...` key) before `go run .`. For webhook-driven confirmation too (`Service.Status`'s
  own reconciliation covers a single local tester without this): `stripe listen --forward-to
  localhost:8080/api/payments/webhook` prints a `whsec_...` value to set as `STRIPE_WEBHOOK_SECRET`.

Frontend (from `frontend/`):
- `npm run dev` — dev server
- `npm run build` / `npm run preview`
- `npm run check` — `svelte-check` type checking (there is no separate lint step)

## Architecture

### Backend layering (mirrors `github.com/ciprianiordache/nutrition-planner`)

Each domain lives in its own package `internals/app/<feature>/` with a fixed file set:
`model.go`, `errors.go`, `repo.go`, `service.go`, `handler.go` (+ `*_test.go`). `user` and `job`
are built end to end; `asset` is repo-only (semi-internal, like `magic_token`/`session`);
`payment` has only `model.go` so far.
Dependency direction is strictly `handler → service → repo → internals/database`. `handler`
uses `internals/httpx` for JSON I/O; tests use `internals/testutil` for a migrated SQLite DB.
Cross-domain composition follows `auth` (a service holds another domain's `Service`/`Repo`):
`job.Service` composes `asset.Repo` and `internals/storage`.

- `model.go` holds the domain struct with `db:"..."` tags **and** separate
  `CreateRequest` / `UpdateRequest` / `Response` DTOs with `json:` tags plus a `toResponse()`
  mapper. The `db`-tagged struct never reaches the HTTP layer. Purely-internal domains
  (`magic_token`, `session`) skip the DTOs and carry small predicate helpers instead.
- `repo.go` defines a `Repo` interface and a `repo{ db *database.Database }` impl that calls
  `r.db.CRUD()` (never `crud.New` — the dialect is set once in `internals/database`); it
  translates `crud.ErrNotFound` into the package's own `ErrNotFound`, and treats an empty
  `List` page as `([]T{}, nil)`.
- `service.go` — `Service` interface + `service{ repo, log *slog.Logger }` via `NewService`.
  Validation + normalisation live here; mutations are logged, reads are not. Race-safe
  find-or-create pattern: on a unique-constraint failure, re-read (see `user.EnsureByEmail`).
- `handler.go` — `Handler{ svc, log }` via `NewHandler`; methods are `http.HandlerFunc`s using
  `internals/httpx` and `r.PathValue("id")`. They map domain errors to status codes
  (`ErrNotFound`→404, `ErrInvalidInput`→400) and never leak `err` text to the client. Routes are
  registered centrally in `internals/app/router.go` (SADE diverges from nutrition-planner, which
  binds handlers to Wails). `user`'s surface is admin-only (`GET /api/users`, `GET/PATCH
  /api/users/{id}`); the current-user endpoint `/api/me` belongs to `auth` (it's about the session).

Wiring lives in root `app.go` (`NewApp(ctx)` → `App.Run(ctx)` → `App.Close()`): config → logger
(`slog.SetDefault` + `.With("environment", …)`) → `database.Connect` → `db.Migrate(app.Models()...)`
→ `mailer.New` → `storage.New` → per-domain `NewRepo` → `NewService` → `NewHandler` →
`app.NewRouter(app.Deps{…})` → `server.New` → (if `ffmpeg.New` succeeds) `token.New` +
`worker.New`. `App.Run` starts the worker, blocks on `server.Run`, then `worker.Stop`. `main.go`
is a thin `run()` that owns `signal.NotifyContext` and guarantees `App.Close` runs. Worker
dependency adapters live in root `worker_wiring.go` (package `main`). `cmd/` is reserved for
utilities (e.g. `cmd/seed`).

`internals/app/router.go` builds the mux (Go 1.22 method+path patterns) and wraps it in
`chain(mux, Recover, RequestLog, CORS, Auth)` — first listed is outermost. `middleware.go`:
`Auth(authSvc, cookieName)` resolves the session cookie and attaches the `user.Response` to the
request context (never rejects); `RequireUser` → 401, `RequireRole(roles…)` → 403;
`UserFrom(ctx) (user.Response, bool)` reads it back. `CORS` uses `Server.CORSAllowedOrigins`
(credentials on, so no wildcard). The context key lives in `package app`, so `/api/me` is a
tiny handler in `router.go`, not in `auth` (keeps `app` ← `auth` a one-way import).

### Domain models

`internals/app/models.go` exposes `Models() []any` — the single ordered list handed to
`db.Migrate`; add a domain by adding one line there. All IDs are `db:"id,primary_key,uuid"`
(crud-depot generates a UUIDv4 before INSERT; stored as `TEXT`). `created_at`/`updated_at` use
`oncreate`/`onwrite` and **must stay `time.Time`** (crud-depot's timestamp hook only fires on
that exact type). Nullable columns are modelled as **plain zero values, not pointers or
`sql.Null*`** — crud-depot cannot scan a non-NULL value back into a pointer, and schema-builder
maps `sql.NullTime` to `TEXT`. So "unused" means `""` for strings, zero `time.Time` for
timestamps (see the `Expired()` helpers on `magic_token` / `session`). Status/kind enums are
package-level string consts.

| Model (`pkg.Type` → table) | Key fields | Relationships |
|---|---|---|
| `user.User` → `users` | `email` unique, `role` (`operator`/`admin`, default `operator`) | parent of everything |
| `magic_token.MagicToken` → `magic_tokens` | `token_hash` unique (sha256 of the emailed token), `expires_at`. **Single-use = the row is deleted on consumption** (`Repo.Consume`, in a tx with a RowsAffected gate), not a flag | `user_id` → `users` cascade |
| `session.Session` → `sessions` | `token_hash` unique (sha256 of the cookie), `expires_at` | `user_id` → `users` cascade |
| `job.Job` → `jobs` | `status` indexed (`pending`→`processing`→`done`/`failed`), `media_type`, `recipient_email`, `watermark_kind`, `watermark_text`, `watermark_opts` (JSON string), `attempts`, `error`, `next_attempt_at` indexed (zero = ready now; worker retry-backoff gate) | `user_id` → `users` cascade |
| `asset.Asset` → `assets` | `kind` (`original`/`preview`), `storage_key`, `filename`, `mime`, `size_bytes`, `checksum` | `job_id` → `jobs` cascade. **A job's files are asset rows keyed by `job_id`; `Job` holds no asset columns.** |
| `payment.Payment` → `payments` | `provider` (`stripe`), `provider_ref` (PaymentIntent id, indexed), `status` (`pending`/`paid`/`failed`/`refunded`), `amount_cents`, `currency` | `job_id` → `jobs` cascade |

**Every component that logs takes the shared `*slog.Logger` by injection** — `server.New` and
`database.New` already do; feature packages follow the same rule. Do not create a second logger
or log via the bare `log` package.

### Data layer — not an ORM

Two libraries operate on the *same* `db`-tagged structs, both reached through `internals/database`:
- `github.com/ciprianiordache/schema-builder` (`package schema`) — behind `db.Migrate(models…)`.
  Ensures tables/indexes/FKs exist at boot (idempotent). Consumes `primary_key`, `uuid`, `auto`,
  `notnull`, `unique`, `index`, `references:table(col)`, `on_delete:`, `on_update:`.
- `github.com/ciprianiordache/crud-depot` (`package crud`) — behind `db.CRUD()`. Runtime CRUD
  over `database/sql` (`Create/Read/ReadOne/Get/Update/Delete`, `RunInTx`). Consumes `default:`,
  `oncreate`, `onwrite` (which schema-builder ignores). Returns `crud.ErrNotFound`. Note `Read`
  (plural) also returns `crud.ErrNotFound` when nothing matches.

### Supporting packages

- `config/` — `Config` struct with `yaml` / `env` / `default` / `secret` / `generate` field tags.
  `config.Load(yaml, env)` layers **struct defaults < YAML < env**, auto-generates each of
  `config.yaml` / `.env` that is missing (never clobbering the other), and fails startup if a
  `secret` is empty or still `CHANGE_ME`. `secret generate:"rand32"` fields (e.g. `Auth.HMACSecret`)
  are self-generated. `App.FrontendDir` (default `./frontend/build`) is the adapter-static build
  `internals/app/static.go` serves; a missing dir just disables that handler. `config.Duration`
  round-trips as `"15s"` in YAML and env; call `.Std()`.
  `config.Validate` checks enums and cross-field rules. `config.Defaults()` returns a `*Config`
  from the `default` tags only (no file/env/secret) — for standalone tools (`cmd/watermark`) and tests.
- `internals/database/` — `New(cfg, log)` then `Connect(ctx)` (bounded `PingContext`). The SQL
  **dialect is chosen here** from the driver name; callers never pick one. Exposes `db.CRUD()`
  (shared `*crud.CRUD`), `db.Migrate(models…)` (schema-builder, idempotent), and
  `Exec/Query/QueryRow/Begin` (so `*Database` satisfies crud-depot's `Executor`/`TxBeginner`).
  `pgx` driver is registered by a blank import in the package; SQLite is test-only.
- `internals/logger/` — `New(cfg) (*slog.Logger, io.Closer, error)`, json/text, stdout/file/both,
  lumberjack rotation, optional `AddSource`. No custom logging abstraction. **Close the returned
  `io.Closer` on shutdown** (`App.Close` does) — required on Windows to release the log file.
- `internals/server/` — `New(cfg, log, handler)` + `Start`/`Shutdown`/`Err`, or the `Run(ctx, s)`
  convenience. `Start` binds the socket synchronously so a taken port is a returned error, not a
  fatal. Timeouts and `MaxHeaderBytes` come from `config.ServerConfig`. Owns no routes.
- `internals/storage/` — `Storage` interface (`Put`/`Open`/`Stat`/`Delete` + `LocalPath`), built
  by `storage.New(cfg, log)`. Objects are addressed by forward-slash key
  (`originals/<jobID>/<assetID>.mp4`); keys are confined to the root (`..`, absolute, backslash →
  `ErrBadKey`). `Put` is temp-file-then-rename atomic. `LocalPath(key) (path, ok)` gives ffmpeg a
  real path for the local backend; `ok` is false for S3 (not implemented yet — `New` errors on it).
  Metadata (real MIME, checksum) is the asset domain's job, not this package's.
- `internals/ffmpeg/` — watermark `Engine` (`ffmpeg.New(cfg, log)`). Shells `ffmpeg`/`ffprobe`
  **directly via `os/exec`, deliberately NOT `u2takey/ffmpeg-go`**: that library's core file
  hard-imports the full `aws-sdk-go` v1 (unavoidable transitive dep), is lightly maintained
  (v0.5.0 / Go 1.16), and under the hood does the same `exec.CommandContext` we do — a
  hand-built `-filter_complex` string is clearer for overlay+drawtext+opacity+position and
  `Engine.buildArgs` stays a pure function unit-tested with no ffmpeg installed. If the client
  ever requires the literal import, routing `buildArgs` output through `ffmpeg_go.Input().Output()`
  is a contained change. `Engine.Probe(ctx, path)` classifies media; `Engine.Watermark(ctx, Request)`
  dispatches video/image (logo `overlay` + `drawtext`, opacity via `colorchannelmixer`) and audio
  (`amix` of a looped low-gain clip). Every call takes a context so `WORKER_JOB_TIMEOUT` cancels
  it. When `Request.OnProgress != nil`, `-progress pipe:1` output is parsed (`progress.go`) into
  `Progress{Percent, OutTime, Frame, FPS, Speed, …, Done}` — no polling, no temp file; `Percent`
  needs `Request.DurationSec` (or a self-probe). `cmd/watermark` is the CLI demo. Needs the
  `ffmpeg`/`ffprobe` binaries; `New` fails fast if missing.
- `internals/app/auth/` — the magic-link flow (no table of its own; composes `user` + `magic_token`
  + `session` + `mailer`). `Service`: `RequestLink` (ensure account → mint token → email the
  `PublicURL/api/auth/callback?token=` link), `Complete` (spend token → create session → return
  its secret + expiry), `Authenticate` (cookie value → user; deletes an expired session as a side
  effect), `Logout`. Only sha256 hashes of the link token and the session secret are stored. Any
  unusable token → `ErrInvalidToken` (no probing which of unknown/used/expired).
- `internals/worker/` — in-process pool (`worker.New` + `Start`/`Stop`), wired in `app.go`. Declares
  its own dependency interfaces — `JobStore`, `AssetStore`, `Blob`, `Engine`, `Notifier` — bound to
  the real repos/`storage`/`ffmpeg` by adapters in root `worker_wiring.go`, and to fakes in tests.
  A dispatcher polls `JobStore.ClaimPending` (atomic claim, `FOR UPDATE SKIP LOCKED` on Postgres,
  bumps `attempts`) every `PollInterval` and feeds `Concurrency` processors. `process` per job:
  load original → `Blob.LocalPath` → `Engine.Probe` → `Engine.Watermark` into
  `previews/<jobID>/<base>-preview.<ext>` → `Blob.Stat` + sha256 → `AssetStore.AddPreview` →
  `Notifier.PreviewReady`. Outcome drives `MarkDone` / `MarkForRetry` (`RetryBackoff<<(attempt-1)`,
  while `attempts <= MaxRetries`) / `MarkFailed`. `Start` first runs `ResetStuck` (requeue rows
  stuck `processing` past `StuckJobTimeout`). Per-job context is detached from shutdown, bounded
  by `JobTimeout`; `Stop` waits out `ShutdownGrace`. `worker.EmailNotifier` signs a `token`
  `"preview"` capability for the preview asset id and mails `PublicURL/preview/<token>` (the
  frontend page - player + Stripe unlock, itself backed by the same token against `/p/<token>`)
  as the primary link, plus `PublicURL/d/<token>` for a direct download. DB row is the source of
  truth. Local storage only (needs `LocalPath`); S3 staging is a TODO.
- `internals/mailer/` — `mailer.New(cfg, log)` → `Mailer.Send(ctx, Message)`. Transport from
  `Mailer.Transport`: `smtp` (STARTTLS on 587, implicit TLS on 465, PLAIN auth when a user is
  set), `log` (renders to the logger — the local magic-link path; body at Info, full RFC 5322 at
  Debug), `noop` (discard). `build()` assembles the message once (quoted-printable,
  `multipart/alternative` when `Message.HTML` is set, RFC 2047 subject); templating is the
  caller's job. `Message.Inline` (`CID`/`ContentType`/`Data`) nests that `multipart/alternative`
  inside an outer `multipart/related` and adds each as a base64 sibling part with a `Content-ID` -
  an image the HTML references as `cid:<CID>` instead of a URL, so nothing outside the message
  itself has to be reachable for it to render (`internals/emailtmpl`'s logo is the one user so
  far).
- `internals/emailtmpl/` — shared branded HTML layer for transactional email, used by `auth`
  (magic-link) and `worker` (preview-ready). `Render(Data) (string, error)` (heading, intro,
  buttons, fallback link, note) and `HumanDuration(time.Duration) string` ("15 minutes" instead of
  `15m0s`). Palette/type mirror `frontend/src/app.css`'s light tokens verbatim; deliberately not
  theme-aware, unlike the app - mail-client dark-mode support is inconsistent enough to auto-invert
  a hand-tuned palette badly, so the email ships one fixed light design. `LogoPNG` (`//go:embed
  logo.png`) + `LogoCID` are sent as a `mailer.Inline` alongside the HTML by both callers.
- `internals/token/` — `token.New(secret)` → `Signer.Sign(purpose, subject, ttl)` /
  `Verify(purpose, tok)`. Stateless HMAC-SHA256 capability tokens for the public `/p/{token}`
  (purpose `"preview"`) and `/d/{token}` (`"download"`) links — no DB row, no login. Signed
  payload is `purpose.subject.exp` (subject = the preview asset id); a token for one purpose never
  verifies for another. Uses `Auth.HMACSecret` (shared with sessions, domain-separated by the
  purpose prefix). `worker.EmailNotifier` signs both; `internals/app/share` verifies.

### Auth

Operator login is magic-link only (no password). `POST /api/auth/request {email}` →
`user.EnsureByEmail` (find-or-create an operator) → a random 32-byte token, sha256-hashed and
stored with `MagicLinkTTL` → email carries `PublicURL/api/auth/callback?token=<raw>`. The
callback spends the token (atomic delete) and issues a DB-backed session: a random secret in the
`sade_session` cookie (HttpOnly, `Secure` per `Auth.SessionCookieSecure`, SameSite=Lax,
`SessionTTL`), only its sha256 stored. `Auth` middleware attaches the user on every request;
`/api/me` returns it; `/api/users*` additionally needs `RequireRole("admin")`. `POST
/api/auth/logout` deletes the session row and clears the cookie. Recipients of watermarked
previews never get an account — they use signed `internals/token` links served by
`internals/app/share` (`GET /p/{token}` inline, `GET /d/{token}` download).

### Frontend

SvelteKit 2 / Svelte 5. **Runes mode is forced** for all non-`node_modules` files via
`frontend/vite.config.ts`. TypeScript `strict`. Built as a pure client-rendered SPA:
`adapter-static({ fallback: 'index.html' })` + root `src/routes/+layout.ts` (`ssr = false`);
`internals/app/static.go` serves `frontend/build` with the same fallback semantics, so a hard
refresh on e.g. `/app/jobs/<id>` still works.

Routes (`src/routes/`): `/` landing, `/login` (magic-link request, `?error=invalid_link` banner),
`/app` (a guarded layout — `onMount` calls `auth.ensureLoaded()` and redirects to `/login` if
there's no session — plus the upload form and a polling job list), `/app/jobs/[id]` (detail with
a status stepper, polls while pending/processing), `/preview/[token]` (public, no auth, mobile-
first — the token in the URL is the whole authorization, same as the Go `/p`/`/d` handlers it
calls).

**Design system** (redesigned 2026-09-10 from a Claude Design canvas the user seeded a session
prior — see `docs/icon150.png`, the real logo, used everywhere the mark appears): warm palette,
one orange accent, IBM Plex Sans + Mono (loaded via a Google Fonts `<link>` in `app.html`).
Tokens live as CSS custom properties in `app.css` — `--bg`/`--surface`/`--text`/`--border`/
`--accent` (+ `-hover`/`-tint` variants) and per-status colors, redefined under
`:root[data-theme='dark']` and the `prefers-color-scheme` media query. `src/lib/components/`:
`LangThemeToggle.svelte` (the RO|EN + light|dark pill pair, reused on every page next to the mark
or the user chip), `MarkBar.svelte` (logo + wordmark + the toggle, for pages with no operator
session), `StatusBadge.svelte` / `StatusDot.svelte` (job status, shared by the dashboard and
detail page).

Reskinned within what `internals/app/job`'s API actually returns, not the full canvas mockup:
list rows key on `recipientEmail` (the list endpoint has no filename — only a job's `assets`,
fetched on the detail page, carry one), and the job detail page has no copyable `/p`/`/d` links
or a "resend email" action (the API never exposes a preview token to the operator, and there's no
resend endpoint — only the worker's `EmailNotifier` ever sees the token). The dashboard's search /
status filter / sort / density (comfortable, compact) / view (list, grid) / pagination are all
client-side over one `GET /api/jobs?limit=200` fetch — `job.Response` carries no total count, so
this holds up until an operator has 200+ jobs, at which point it needs real server-side paging.

`src/lib/`:
- `api.ts` — typed `fetch` wrapper over `/api/*` (`credentials: 'include'` for the session
  cookie; `ApiError` carries the HTTP status). `API_BASE` is `http://localhost:8080` in dev
  (`import.meta.env.DEV`, matching `config.ServerConfig`'s default port) and `''` in the
  production build (same origin — the Go binary serves both). Exported so `/preview/[token]` can
  build direct `/p/{token}` and `/d/{token}` URLs without a JSON round trip.
- `types.ts` — hand-kept TS mirror of the Go `Response` DTOs (`user`/`job`/`asset`); no codegen,
  keep in sync by hand when a `model.go` `Response` changes. Also `ALLOWED_EXTENSIONS` /
  `isAllowedFile` — a client-side mirror of `config.UploadConfig`'s defaults, so a bad file never
  reaches the upload button; the server's own check is still authoritative.
- `format.ts` — `relativeTime(iso, locale)`, the compact "2m"/"1h"/"3d" timestamps in the job list.
- `stores.svelte.ts`, `theme.svelte.ts`, `i18n.svelte.ts` — reactive singletons (a class holding
  `$state` fields, instantiated once and imported by value). Svelte 5 runes only compile inside
  `.svelte` / `.svelte.js` / `.svelte.ts` files — a plain `.ts` file never goes through the Svelte
  preprocessor, so anything stateful lives in a `*.svelte.ts` file, not the plain `stores.ts` /
  `theme.ts` the original plan sketched. `theme`/`i18n` persist to `localStorage` and are
  initialized once from the root layout's `onMount` (avoids an SSR/hydration mismatch even though
  `ssr` is off). Default locale is `ro`; `theme.value` is `'light' | 'dark' | 'system'` but the
  toggle pill only ever writes `'light'`/`'dark'` explicitly (matches the 2-option design; a
  first render can still show the `system`-resolved state before anyone clicks it).

## Constraints

- `notifix` and `log-dog` (referenced in `Writerside/topics/sever.md`) are the client's private
  in-house products and are **not accessible**. Use `internals/mailer` (SMTP) and
  `internals/logger` (`slog`) instead.
