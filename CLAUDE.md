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

**Latest:** eleventh `/impeccable critique` of the installed app, with the `/design-taste-frontend`
lens (user asked for both; the taste skill's marketing rules do not apply to an app - only its
one-system / motion / keyboard / copy checks and its copy-lint were used) - 27/40, snapshot
`.impeccable/critique/2026-11-12T17-40-56Z__frontend-src-tauri.md` - and all five fixes (user:
system + feedback first, all 5; "Fișier nou" left to me - kept as the orange brand pill, no lift):
- **One control system** (documented in DESIGN.md "One control system"): `--radius-xs` 6px added;
  every off-scale radius (16/13/10/7/5/4px) mapped to 12/8/6; one `.btn-danger` in app.css
  (DeliveryCard's, the job page's `danger-btn armed` and revoke's `danger-btn` merged); two link
  looks only - `.inline-link` / `.link-btn` (accent, underlined) and `.link-btn.quiet` (muted),
  defined once in app.css, the local copies in +page / welcome / login removed; LogoPicker's
  `.text-btn` turns red on hover only when `.danger`; "⋯" (`.more-btn`) transparent like the other
  hairline pills.
- **Feedback**: one shared `:active` rule (brightness 0.93; 1.15 in dark) for rows, side items,
  menu rows, chips, close/more/icon buttons, segmented, pill-group, dropzone, back link, links,
  switches, summary, caption buttons; `.switch:hover`; no hover lift for `.btn` in the app; the
  global focus rule no longer sets border-radius (a focused switch stays a pill - verified 999px).
- **Settings**: in the app `.prefs-main .win` is 8px, hairline, no shadow (Windows 11 groups).
- **Copy** (taste rules): links are oprite/pornite everywhere ("Confirmă: oprește linkurile", toasts,
  history), one sign-out verb ("Deconectează-te", "Deconectează toate dispozitivele" - no longer
  wraps), "În coadă" for pending (filter too), no "!" ("Contul e pregătit", "Copia cu filigran a
  fost trimisă către…"), no "în siguranță" lines (recipient footer "Livrat prin SADE"; offline:
  "Livrările rămân pe server…"), buttons with objects ("Vezi livrarea", "Închide logo-ul",
  "Arată sesiunile"), no em dashes left in the Romanian strings, dead `brand.save/leaveConfirm/
  unsaved` keys removed.
- **List**: column headers are buttons that sort (Client / Fișier A-Z, Stare, Când toggles
  newest/oldest, arrow on the active one; new `Sort` modes `client` / `file`, also in the select);
  age shows "acum 1 h" (`ago`); the file column takes the remaining width (table fills the
  window); each row has an `aria-label` (recipient, file, status, age) and no `aria-haspopup`.
- Verified in the rebuilt app over CDP (header sort, row label, focused switch radius, Brand groups,
  list layout). `npm run check` clean. Installer not rebuilt. Not committed.

**Before that:** tenth `/impeccable critique` of the installed app (25/40 - stricter run: status and
error recovery up, new findings on theme/efficiency/help; snapshot
`.impeccable/critique/2026-11-12T17-17-58Z__frontend-src-tauri.md`) and all five fixes (user: the
two P1 first, all 5; file page vs pane left to me - kept as is for now, the pane lacks Details,
the original/preview switch and Download):
- **Distinct names** (a reviewer closed the whole app by clicking "Închide"): window close is
  "Închide SADE" (`window.close`), pane closes are "Închide panoul" (`common.closePane`, JobPanel +
  upload pane); Security's group buttons carry the device ("Arată sesiunile: Chrome pe Windows",
  `settings.sessionsShowFor` / `sessionsRevokeAllFor`).
- **Maximise says "Restabilește"** when the window is maximised, with Windows' two-squares glyph:
  `lib/window.ts` `watchMaximized` (isMaximized + onResized, both in core:default),
  `WindowControls` `maxLabel`. Verified live: Mărește -> Restabilește -> Mărește.
- **Theme "Ca sistemul"**: AccountMenu and LangThemeToggle offer system / light / dark
  (`aria-pressed` on the stored choice; the old `isDark` derivations are gone), so the app can
  follow Windows again.
- **Window title** follows the open delivery in the app ("<file> · Livrări · SADE").
- **App look**: `.chip` ("Linkuri active") is a dot and a word; Brand's two windows lose their
  shadow (`html.is-app .app-body .win.form/.preview`); the file page is one white surface
  (`.app-file` background); `input[type=range]` min-height 24px; the disabled send button says
  why ("Alege întâi un fișier.", `aria-describedby`).
- Not done: one identical title bar on every page (search/sort make no sense off the list), a
  custom video player, multi-select, a shortcut sheet, notifications.
- `npm run check` clean; app rebuilt and checked over CDP. Installer not rebuilt. Not committed.

**Before that:** ninth `/impeccable critique` of the installed app (26/40, snapshot
`.impeccable/critique/2026-11-12T16-49-08Z__frontend-src-tauri.md`; detector over CDP: only the
cream-palette / PositionPicker false positives, contrast clean) and all six fixes (user: behaviour
first, all 6; login: my recommendation = hide the server, no sade:// deep link yet - the email's
6-digit code already covers sign-in from the app):
- **File drops**: `tauri.conf.json` window `dragDropEnabled: false` - Tauri 2's default swallowed
  OS file drops on Windows before the page's HTML5 `drop` (upload dropzone, LogoPicker). Not
  tested with a real Explorer drag (CDP cannot do an OS drag) - try it once by hand.
- **Server down**: `routes/app/+page.svelte` `loadJobs` now catches: `loadError` + `lastLoaded`;
  with no jobs yet it shows "Nu ne putem conecta la server" + Reîncearcă (never the empty account /
  upload form - `showUpload` requires `!loadError`); with jobs it keeps the list and the bar says
  "Fără conexiune · actualizat la HH:MM · Reîncearcă". The poll also refreshes every ~32s when
  nothing is processing (opened/paid show up, a returning server is noticed). `routes/app/
  +layout.svelte` boots through `boot()` with a catch: a full-window "Nu ne putem conecta la
  server" + Reîncearcă (with window controls in the app) instead of a blank window. Verified live:
  app started with the server down -> error screen -> server up -> Reîncearcă -> list; server
  stopped mid-session -> offline line, list kept.
- **Look**: in the app `.st` is a dot and a word (no border/fill; `html.is-app .st`); menu items
  in `AccountMenu` focus as a filled row (no orange ring); the account menu no longer repeats
  "Livrări" in the app; Brand's settings icon is a picture, not a shield; Security's passkey note
  (the app cannot use passkeys) moved from first to last; Security's window title is
  "Securitate · SADE". Settings' white groups on the tinted ground were kept on purpose - that is
  Windows 11 Settings' own pattern.
- **Keyboard**: the delivery rows are one Tab stop (`focusRowId`: the open row, else the first;
  `tabindex` 0/-1 when `wide`), arrows + Home/End move inside the list.
- **Bug**: an in-app link to `/app?job=<id>` while on /app now opens the pane (`$effect` on
  `page.url`).
- **Login (app)**: the server address sits in a `<details>` "Opțiuni avansate" ("Adresa serverului
  SADE", with a hint), not on the sign-in screen itself.
- Not done: multi-select/batch, header sort, a "System" theme option, native notifications, a
  `sade://` deep link, the file page vs pane duplication (open product question).
- `npm run check` clean; app rebuilt (`npx tauri build --no-bundle`). Installer not rebuilt /
  not copied to `downloads/`. Not committed.

**Before that:** new README screenshots + README rewrite (user: delete the stray root PNGs, new shots,
update the README). `docs/`: `dashboard.png`, `dashboard-dark.png`, `job.png`, `brand.png`,
`email.png`, `preview.png`, `mobile.png`, `unlocked.png` (1600px wide, from 2x captures; mobile
600, email 900). Taken from the demo operator on a temporary 8091 instance: today's test jobs
deleted, four deliveries uploaded from the marketing page's licensed wedding photos +
`docs/test.mp4`, one paid in Stripe TEST mode by confirming its PaymentIntent with `pm_card_visa`,
Stripe's test-mode badge hidden for the shots. Found on the way and fixed: on the recipient page
(>= 900px) a payment form taller than the picture stretched the dark stage, leaving a black band
above the photo - the stage now `align-self: start`; "Linkul rămâne valabil — ..." lost its em
dash. README: new intro/flow (one email, `/preview` page, recipient in every watermark), a
screenshot tour, architecture adds the installed app and email, commands add `cmd/emailpreview`,
`cmd/seed`, `npm run build:app`, Status rewritten. Not committed.

**Before that:** `/impeccable critique` of the WHOLE web frontend (26/40, snapshot
`.impeccable/critique/2026-11-12T08-53-19Z__frontend-src.md`; detector CLI clean, overlay found one
real contrast issue) and all five fixes (user: watermark + recipient first, show the sender's
account email on the recipient page, all 5):
- **P0 - the recipient is now on every copy** (it was not, despite Brand's "Mereu pe fișier"):
  `wmopts.WithRecipient(text, recipient, max)` appends "· <recipient>" to the operator's own
  per-upload text (operator part shortened with "...", never the address); `wmopts.render` appends
  `{recipient}` to a template that lacks it (after dropping empty company/date); the worker turns
  kind "logo" into logo + a recipient-only text line (`process.go`, audio untouched). Frontend
  mirrors it (`format.ts` `withRecipient`, `withRecipientPlaceholder`); the upload form says "Adresa
  clientului se adaugă la final: · x" under the text field and the summary always shows the burned
  text ("Logo-ul tău și {email}" for logo). Tests: `worker_test.go`
  `TestProcessAlwaysBurnsTheRecipient`, `wmopts_test.go` `TestWithRecipient` (old "Confidential"
  expectation updated - it encoded the bug).
- **Recipient page** (`/preview/[token]`): new public `GET /api/share/info?token=`
  (`internals/app/share/info.go`, payment rate limit) -> `{fileName, mediaType, sizeBytes, sender}`
  via `Handler.UseDetails` (app.go reuses the email's `deliveryInfo` lookup), 410 "expired" /
  "disabled", 404 invalid (`info_test.go`). The page checks it first: skeleton, then distinct
  cards for expired / invalid / disabled with what to do next ("cere expeditorului un link nou: un
  răspuns la emailul primit ajunge la el"); h1 = file name, "Trimis de <operator email>", bar
  "Copie cu filigran", an after-payment line, unlock copy no longer says "audibil"/"îți trimite",
  payment errors never show the server's text, the stage note is light on the dark stage.
  `PaymentForm`: Stripe `locale` follows the page, country defaults to RO.
- **Touch targets** measured with elementFromPoint (scratchpad `hit.js`): `.btn-sm::after` -6px
  (was 42px), `OverflowMenu` -6px, the dashboard search label gets a hit strip (input lifted above
  it so the caret still lands), the narrow chip row's `.side-item` is 44px drawn (the sideways
  scroll clipped `::after`), the recipient bar's download icon 44px wide. Sidebar counts and "Tip"
  moved from `--text-faint` (3.9:1) to `--text-muted`.
- **Terms and dates**: "link-ul/link-uri" -> "linkul/linkuri" everywhere in ro, "Status" -> "Stare";
  `relativeTime` now "2 min / 3 h / 5 zile" (list age column 64px), Security phrases use `ago()`
  ("activ acum 3 h"); Brand's date row shows the burned ISO date (`burnedDate()`), other dates
  `fullDate` ("12 nov. 2026, 11:41", as in the emails).
- **Hierarchy**: Brand's two window names are real `h2.bar-title`; the job page's bar says
  "Livrări / Livrare" instead of repeating the file name; JobPanel caps the picture at 34svh when
  stacked so the delivery actions start on the first screen.
- Not done from the critique: Security cards vs windows, page-head size differences, bulk actions,
  album delivery (product question), Stripe Link block (Stripe-side setting).
- Verified: `go test ./...` green except the known Fontconfig crash, `npm run check` clean, live on
  a temporary 8091 instance (tap targets re-measured, recipient page valid + invalid states). The
  demo account got 4 extra test deliveries (example.com recipients). Not committed.

**Before that:** second `/impeccable critique` of the emails (24/36, up from 18; snapshot
`.impeccable/critique/2026-11-12T04-25-46Z__internals-emailtmpl-emailtmpl-go.md`; detector: only
the cream-palette false positive) and all five fixes (user: "All 5 issues", rest "do what you
think is better"):
- **Outlook markup actually ships now (bug):** html/template strips every HTML comment in the
  template text, so the `<!--[if mso]>` blocks (fixed 560px column, mso styles) never reached a
  real email. They now enter as `template.HTML` values (`view.MSOHead/MSOOpen/MSOClose`). Each
  button gets a VML `v:roundrect` for Outlook (`vmlButton`, width from the label length) and the
  ordinary `<a>` inside `<!--[if !mso]><!-->` - Word ignored padding/radius on the link.
- **Replies reach the operator:** `mailer.Message.ReplyTo` (bare address only, parsed with
  `mail.ParseAddress`; display names / CRLF dropped - `TestBuildReplyTo`); the delivery email sets
  it to the sender's verified account email and the footer says a reply goes to the sender.
  `MailerConfig.FromName` default is now "SADE" (was English "Safe Media Delivery"; the local
  `config.yaml` too).
- **Layout:** the window bar (three dots + crumb) is gone from the emails (`Data.Crumb` removed);
  heading is a real `<h1>` with `word-break`/`overflow-wrap:anywhere`; new `Data.Prompt` (ink line
  above the buttons) and `Data.ButtonNote` (small line under them); the code is drawn as two
  groups ("482 913" with a drawn gap, so a copy pastes "482913"; `CompleteWithCode` also strips
  inner spaces now); fallback URL in the text face, not mono.
- **Alert:** "Nu ai fost tu? Închide toate sesiunile: ... inclusiv de pe acesta ..." above the ink
  button, fallback URL added, link validity (7 zile) in the note, the IP row dropped (Settings ›
  Securitate shows it; note says so), passkey advice reworded around mailbox access.
- **Delivery:** no "De la" row (the heading names the sender), intro "Îl vezi direct în browser."
  (+ unlock line), reassurance "Nu ai nevoie de cont, iar SADE nu îți cere niciodată parola." right
  under the button, note = watermark + link validity, the English "SADE (Safe Media Delivery)" gone.
- **Plain text:** all three bodies are built from the same strings as the HTML and end with
  `emailtmpl.TextFooter` (tagline + footer).
- `cmd/emailpreview` updated. Verified: tests (emailtmpl Outlook markup / code groups / footer,
  mailer Reply-To, auth alert order), browser renders at 390 and 680 (no overflow, 46px buttons).
  Not verified in real Outlook/Gmail. Not committed.

**Before that:** first `/impeccable critique` of the emails (18/36, snapshot
`.impeccable/critique/2026-11-11T22-21-57Z__internals-emailtmpl-emailtmpl-go.md`) and all five
fixes (user: "rulează-le pe toate"; sender = operator's email; no Download; the app's window):
- **`internals/emailtmpl` redesigned to the app**: Reddit Sans + JetBrains Mono, heading 25px/500
  -0.025em, pill buttons (`Primary` orange with #111 text, new `Ink` dark pill, else a hairline
  pill), the product window (bar with three grey dots and "SADE / <Crumb>"), new `Rows` (hairline
  label/value rows), logo + "SADE" wordmark instead of the uppercase eyebrow. Fluid: max 560px
  (fixed 560 only inside an MSO conditional) - phones no longer get a 592px page; under 600px
  buttons go full width and rows stack. Small print is #544b42 / footer #6e655b (all >= 4.5:1;
  the old #9c9287/#b4a997 failed), note hairline in its own row (margin on a td was ignored),
  fallback link muted and a real `<a>`, preheader padded. New `Data.Lang` (default "ro", sets
  `<html lang>` and the chrome words), `Crumb`, `Rows`, `Footer`; `HumanDurationIn(lang, d)`
  ("15 minute", "24 de ore", "30 de zile") and `FormatTime(lang, t)` (Bucharest time, Romanian
  months; `time/tzdata` embedded).
- **All three emails in Romanian**: sign-in ("Conectare la SADE", "Conectează-te", code), new
  sign-in alert ("Conectare nouă în contul tău SADE": facts as rows, time in Romania, ink button
  "Închide toate sesiunile", device label via `auth.deviceRO`), recipient delivery.
- **Recipient email** (`worker.EmailNotifier`): subject/heading "<operator email> ți-a trimis un
  fișier" (the account email - verified by sign-in, cannot pose as a bank; company name/logo stay
  off per TODOS.md), rows Fișier / Tip · size / De la, a line on the watermark and (payments on)
  the unlock, ONE button "Deschide fișierul" to `/preview/<token>` - **the `/d/` download link is
  gone from the email** (the page is the one destination). Details come from
  `EmailNotifier.UseDetails(func(previewID) (DeliveryInfo, error))`, wired in app.go (preview
  asset -> job -> user email + original asset); without it the email says "Ai primit un fișier
  prin SADE". Plain-text bodies match (the old "valid for 720h0m0s" bug is gone).
- `cmd/emailpreview` renders the three current emails (the delivery one through the real
  notifier). Tests: emailtmpl (Romanian durations, lang/window/rows, Bucharest time), worker
  (one page link, no /d/, sender + file), auth (Romanian code regex, alert subject),
  `worker_flow_test` (one link). Detector on the rendered emails: only cream-palette (the brand
  paper). Not verified in real mail clients (Gmail/Outlook) - browser render only. Not committed.

**Before that:** the installed app's scrolling (user: at the end of a scroll the page stretched, the
top bar went away, fixed parts moved). Cause: no page in the app scrolls as a whole, only inner
panes do; when a pane reached its end the scroll was handed to the window, and WebView2's
elastic overscroll (touchpad / touch) pulled the whole page, title bar included. Fix (app only,
`html.is-app` in `app.css`): `html` and `body` are `height: 100%`, `overflow: hidden`,
`overscroll-behavior: none`; the panes that scroll (`.app-body`, `.app-auth`, `.prefs-main`,
`.media-col`, `.side-col`, `.job-win`, plus the dashboard's `.list` and `.open-pane`) are
`overscroll-behavior: none` (the old `contain` still let a pane's own content stretch).
`.app-shell` is `height: 100%`; the app's sign-in (`AuthShell` `.app-auth`) scrolls on its own
under the title bar (`justify-content: safe center`). Gotcha found on the way: do NOT put
`overscroll-behavior: none` on every element - a box with `overflow: hidden` and a text field
are scroll boxes to the browser, and blocking them stopped the wheel from reaching the pane
(the Brand form did not scroll with the mouse over it). Verified in the built app with real
wheel input over CDP (scratchpad `wheel.mjs`): Brand (over a field and a label), Security, file
page, dashboard pane - each pane reaches its end, the window's scrollTop and the title bar stay
at 0. The stretch itself only shows with a touchpad/touch screen, so it was not seen live here.

**Before that:** eighth `/impeccable critique` of the installed app (27/40, snapshot
`.impeccable/critique/2026-11-11T21-43-45Z__frontend-src-tauri.md`; the live detector found no
real issue anywhere, only the known cream-palette / PositionPicker false positives) and all five
fixes (user: "Poti incepe treaba"; the list's status tag everywhere; payment only in the delivery):
- Brand's window bar always says how saving works: "Se salvează automat" at rest, "Se
  salvează…", then "✓ Salvat · se aplică fișierelor noi" (`brand.autosave`, `brand.savedShort`).
- One "⋯" on the file page: `DeliveryCard` takes `ondelete` / `deleteDisabled` / `deleteTitle`
  and its menu (now in the head, beside the links chip, so it exists in every link state) holds
  "Oprește linkurile…" and, under a separator, "Șterge fișierul…". `OverflowMenu` items gained
  `sep`. The page's own head "⋯" is gone; the delete confirm still shows in the head.
- The file page's status uses the list's tag (`stateClass` from `lib/delivery.ts`); the dot+word
  `.state-line` is gone. The Details "Plată" row is gone (the delivery shows it).
- List columns sized to their content (`28px minmax(140px,280px) minmax(0,460px) 120px 56px`,
  `justify-content: start`); the compact row (pane open) keeps the time under the status.
- Brand: the preview's "no logo" footnote removed (the logo row says it); the history's repeat
  count is "de 5 ori" without a leading dot and never splits.
- Verified in the built app over CDP; installer copied to `downloads/`. Not committed.

**Before that:** seventh `/impeccable critique` of the installed app (26/40, snapshot
`.impeccable/critique/2026-11-11T17-56-25Z__frontend-src-tauri.md`) and all five fixes (user:
"rulează-le pe toate"; payment as a row in the delivery; the rare actions in a "⋯" menu):
- **Payment on the operator's side** (backend): `share.Delivery` gained `payments`,
  `amountCents`, `currency`, `paidAt` (the latest "paid" event). `OperatorHandler.SetPayment`
  is called from app.go only when `StripeSecretKey` is set (otherwise `payments: false` and the
  row hides). Test: `share/delivery_test.go` `TestDeliveryReportsThePayment`. Frontend:
  `DeliveryCard` shows "Plată · Neplătit / Originalul se deblochează la 49 €" or "✓ Plătit 49 € pe
  <date>" above the actions; the file page's Details gained a "Plată" row (`job.paid`).
  `formatPrice` moved to `lib/format.ts` (PaymentForm keeps its own copy).
- **Closed pane left in the URL** (bug): `syncUrl` compared with `page.url.search`, which a shallow
  `replaceState` does not update, so `?job=` stayed and F5 reopened a closed pane. It now builds
  from and compares with `location`.
- **Accessibility**: `JobPanel` no longer is an `aria-live` region (it re-read the pane on every
  15s poll) - a `.sr-only` live line announces the state only; the list pane is `inert` while the
  narrow overlay covers it; the rows list has an accessible name; `::placeholder` uses
  `--text-muted` (the browser grey was 3.5:1 on dark); avatar initials go light on dark.
- **Action hierarchy**: new `components/OverflowMenu.svelte` ("⋯", arrow keys, Escape). In
  `DeliveryCard` "Copiază linkul" is the one ink button, "Oprește linkurile…" lives in "⋯" and
  arms a red confirm block; the resend limit moved to the Resend tooltip. On the file page
  "Șterge fișierul…" is in "⋯" (confirm buttons appear in its place), the status is a dot + word
  (`.state-line`), headings 22px (h2) / 18px (h3).
- **Settings as a preferences window**: in the app `.win` groups are flat (hairline, no shadow);
  Security is one column (max 760px, heading over its group). Signing out this very session asks
  once ("Ieși din cont"). Brand: the no-logo row is titled "Logo"; sliders use the ink colour like
  the switches; the sample address is `client@exemplu.com` everywhere.
- **Row menu**: adds "Retrimite emailul" and "Oprește linkurile" (asks again inside the menu) /
  "Pornește linkurile"; Enter on the open row goes to its page.
- Verified in the built app over CDP (payment row, "⋯" menus, closed pane clears `?job=`, row
  menu, Security layout); the live detector is clean on the dark dashboard. Not committed.

**Before that:** sixth `/impeccable critique` of the installed app (26/40, snapshot
`.impeccable/critique/2026-11-11T17-36-04Z__frontend-src-tauri.md`; it looked harder at Security,
Brand and the keyboard) and all five fixes (user: "rulează-le pe toate"; file page kept as a
window; Brand saves on its own):
- **The app names itself in Security** (backend): `auth.Handler.client` appends `SADEApp` to the
  user agent of a request carrying `X-SADE-Client: app`; `DescribeUserAgent` turns it into "SADE
  app on Windows" (`appUAMarker`, test in `security_test.go`), the frontend's `deviceLabel` into
  "Aplicația SADE pe Windows". The app's own session from before the marker is labelled the same
  (`sessionLabel` in the security page). Only new sign-ins carry it on the server.
- **File page as a window**: no card border/radius/shadow in the app, the head fixed, the picture
  and the delivery columns scroll on their own (`.app-file`, the layout's `app-body.full` for
  `/app/jobs/*`); `h1` kept as `.sr-only` (new global class in `app.css`), also on the dashboard;
  headings 18px (h2) / 15px (h3) over 14.4px text; the recipient row dropped from Details (it is
  in the lead); the title bar shows the logo only on welcome.
- **History**: runs of the same opened/downloaded event fold into one row ("· de 5 ori", latest
  time) that unfolds on click (`DeliveryCard` `runs`); the status chip left the pane head.
- **Brand saves on its own**: every change saves after a 700ms pause (`$effect` + `save()`), the
  draft is never replaced by the answer; "Se salvează… / ✓ Salvat / Nu s-a salvat · Reîncearcă"
  in the window bar; no Save button, no leave-confirm (leaving saves what waits); "Poziție și
  mărime" is an h2; the no-logo hint is muted, underlined.
- **Desktop conventions**: Ctrl/Cmd+F focuses search (kbd hint shows it; placeholder "Caută");
  right-click / menu key / Shift+F10 on a row opens a menu (Deschide, Copiază linkul
  destinatarului, Pagina fișierului; arrows, Escape, Tab); caption buttons `tabindex=-1`; Escape
  closes the pane at every width; format hint lists webp; column headers and row age moved to
  `--text-muted`; usage shows KB under 1 MB; dark-mode selected row 13%; in the narrow overlay the
  bar names what is open; security intro `text-wrap: pretty`.
- Verified in the built app over CDP: menu, Ctrl+F, pane, file page scroll, history unfold,
  "Aplicația SADE pe Windows", autosave ("Salvat"), live detector leaves only cream-palette.
  Gotcha: Python edit scripts on Windows wrote CRLF / turned `` into a backspace - the
  scratchpad `edlib.py` keeps each file's line endings. Not committed.

**Before that:** fifth `/impeccable critique` of the installed app (29/40) and all five fixes (user:
"rulează-le pe toate"; a status bar was declined for now):
- File page in the app: no repeated title (the name is in the title bar only), the "Cu filigran /
  Original" switch sits in the head beside the status chip and "Șterge", the empty inner bar is gone.
- Thin scrollbars in the app (`html.is-app ::-webkit-scrollbar*` in `app.css`, no arrows).
- Fonts are bundled for the app (offline): `frontend/static-app/fonts/` (Reddit Sans + JetBrains
  Mono woff2, latin + latin-ext, `fonts.css`). `app.html` loads `%sveltekit.env.PUBLIC_FONTS_HREF%`:
  `frontend/.env` = the Google Fonts URL (website), `frontend/.env.app` = `/fonts/fonts.css`. Both
  files are now tracked (`frontend/.gitignore` negations; public values only, no secrets). The
  Tauri CSP no longer allows Google.
- Upload pane: closes with x like a delivery; the form fills the pane (address and watermark side
  by side from 640px, container query `upload`).
- Shortcuts: Ctrl/Cmd+N opens the upload pane, Ctrl/Cmd+, opens Settings (`routes/app/+layout.svelte`
  `onAppKey`; shown in the buttons' tooltips). Security groups sessions of the same device kind
  ("Chrome pe Windows · 6 sesiuni", Arată / Deconectează toate); this device stays its own row.
- Verified in the built app over CDP (fonts load from `/fonts/fonts.css`, both shortcuts, upload,
  security and file page screenshots). New installer copied to `downloads/` (2.2 MB). Not committed.

**Before that:** fourth `/impeccable critique`, of the installed app itself (28/40, snapshot
`.impeccable/critique/2026-11-11T16-59-20Z__frontend-src-tauri.md`; live scan over CDP: only
"cream-palette", the brand's paper - false positive), and all five fixes (user: "rulează-le pe
toate"; the file page becomes a full-window delivery, minimum window width 720px):
- File page in the app: no back link (the title bar says "< Livrări" + the file name via
  `lib/apptitle.svelte.ts appTitle`), compact head, no crumb, fills the window.
- Every app width keeps the app shell: the window's minimum is 720px (`tauri.conf.json`); the
  dashboard uses the shell from 720px, the pane sits beside the list from 1024px and slides over
  it below that (`overlay`).
- `html.is-app` (set in `routes/+layout.svelte`): no `.win-dots` inside the window, no UI text
  selection (fields and `.mono` still selectable), the browser's context menu suppressed outside
  text fields, app pages padded as a window (`.app-body > .page:not(.app-page)`), drop-downs
  drawn by the app (`appearance: base-select`, a small chevron; WebKit keeps its native one).
- F5 keeps the page: `afterNavigate` remembers `/app...` in sessionStorage, `routes/+layout.ts`
  sends a reload that Tauri served from "/" back there.
- Native: `tauri-plugin-window-state` (size/position remembered); macOS gets the default app menu
  (`Menu::default`, cfg macos - compiles only on the Mac CI runner, not verified here).
- Verified in the built Windows app over CDP: file page title, F5 restore, overlay at 800px,
  pane at full width, settings without dots, context menu blocked, user-select none.

**Before that:** the installed app follows the system it runs on (user: the bar looked Mac-only;
the sidebar should stay put; settings and login like a professional desktop app):
- `WindowControls side="left|right"` (each bar places both; only the system's side renders,
  `platform.ts desktopOS()` from the user agent): macOS = the three lights on the left; Windows =
  46px minimise/maximise/close caption buttons, full bar height, flush right (close red), as
  Windows 11; Linux = round GNOME buttons on the right. Gotcha found: the class `.win` is global
  (window surface) - the Windows group is `.caps`. Checked as all three by overriding the user
  agent over CDP (scratchpad `cdp.mjs uashot`).
- Dashboard (wide): the list pane no longer scrolls as a whole - the rows column scrolls, the
  sidebar ("Fișier nou", filters, Setări) stays fixed.
- Settings in the app: `routes/app/settings/+layout.svelte` makes a preferences window - a fixed
  220px sidebar (Brand, Securitate) and the section scrolling beside it; `SettingsTabs` shows only
  the section name + one line there. Security hides passkeys in the app (`settings.passkeysAppNote`:
  managed in the browser - WebAuthn needs the server's domain).
- Login in the app (`AuthShell`): the app icon over a compact centred card, no marketing statement;
  copy says a 6-digit code ("Trimite codul").
- Known: a full reload of a deep route in the app (F5 on /app/settings) lands on /app - Tauri serves
  only real files and sends the rest to the root; in-app links navigate client-side and are fine.

**Before that:** the installed app no longer looks like a web page (user: "aplicatia arata exact ca
o pagina de web": drop the top bar, keep the 3-dot bar, open the right pane only on selection,
move the create button and the operator chip). **Installed app only** (`IS_APP`); the web `/app`
is unchanged.
- The window is frameless (`tauri.conf.json` `decorations: false`, `shadow: true`; capability
  `default.json` adds window start-dragging/close/minimize/toggle-maximize). The 3 dots are the
  real window controls (`components/WindowControls.svelte` + `lib/window.ts`, lazy
  `@tauri-apps/api/window`): grey, coloured on hover like macOS; on the web they stay the
  decorative `.win-dots`. Bars carry `data-tauri-drag-region`.
- Dashboard: the list window's bar is the title bar (controls, crumb, search, sort, the
  operator's `AccountMenu` - extracted from the layout, used by both); the window fills the app
  (`.app-page`), no page head, no nudges on wide. Sidebar: "Fișier nou" first (`.new-btn`, opens
  the upload pane), "Setări" at its foot. The right pane opens only for a selected row or the
  upload form (`paneOpen`; no default selection in the app), closes with x (`JobPanel onclose`)
  or Escape.
- Other app pages get a 44px title bar from the `/app` layout (controls, "< Livrări", page
  name, account); `MarkBar` becomes the same kind of bar on the signed-out pages (login).
- Verified in the built app over CDP: list full width, row -> pane with x, Escape closes,
  maximise 1320 -> 1920 -> 1320, "Fișier nou" -> upload pane, Settings with its bar; the web
  dashboard checked unchanged. New installer copied to `downloads/`.
- Next candidates: settings/job pages still use the web page heads inside the app window.

**Before that:** the app is split into **the website** (marketing page, sign-up, new download page)
and **an installed app** (Tauri 2: Windows, macOS, Linux now; iOS and Android next), sharing one
SvelteKit codebase; the operator web app at `/app` stays (user's choices: Tauri 2, keep the web
app, start with the download page + desktop).
- **Server (sign-in for the app):** the app runs on its own origin (`http://tauri.localhost` on
  Windows, `tauri://localhost` elsewhere), so no cookies. It sends `X-SADE-Client: app` and its
  own binding value `X-SADE-Login` (32 random bytes it keeps; `auth.Handler.browserNonce` reads it
  when there is no `sade_login` cookie - a forwarded email is still useless elsewhere). A finished
  sign-in (code, link, passkey) answers `{token, expiresAt}` to the app (`writeSession`); the
  `Auth` middleware accepts `Authorization: Bearer <session secret>` (`auth.BearerToken`), logout
  too. `config.AppConfig.AppOrigins` (default the three Tauri origins) join CORS (headers now
  allow Authorization/X-SADE-*, methods PUT) and the origin check. Media for `<img>/<video>`:
  `GET /api/jobs/{id}/assets/{assetId}/link` mints a 6h `"asset"`-purpose token for
  (operator, asset); the content route accepts `?t=` instead of a session (`applink.go`
  `mediaAccess`). `config.AppConfig.DownloadsDir` (`./downloads`, gitignored) is served at
  `/downloads/` (no listings, `.json` no-cache). Tests: `internals/app/app_client_test.go`.
- **Frontend app mode:** `vite build --mode app` (`npm run build:app`, `.env.app` sets
  `VITE_SADE_APP=1` + `VITE_SADE_SERVER`) builds `frontend/build-app` from `static-app/` (no
  marketing page; `/` redirects to `/app` in `+layout.ts`). `lib/platform.ts` (IS_APP, server URL,
  token, binding nonce, headers); `api.ts` uses them (credentials omit, token saved from auth
  responses, `assetLink`, `blobUrl`); `lib/media.ts` `use:authedSrc`/`use:authedHref` replace
  every `src=`/download `href=` of an operator's media (signed link in the app, blob for the
  logo, plain URL on the web). Login in the app shows the code field open and a "Server: ...
  Schimbă" control; passkeys are hidden in the app (WebAuthn needs the server's domain as origin).
- **Tauri:** `frontend/src-tauri` (crate `sade`, identifier `ro.sade.app` - must match the
  eventual store listings, cannot change after publishing), window 1320x860, CSP in
  `tauri.conf.json`, icons generated from `frontend/src-tauri-assets/icon.svg` (the logo redrawn
  as vectors; `npx tauri icon`). Built here: `SADE_0.1.0_x64-setup.exe` (1.9 MB) + `.msi`.
  Toolchain installed this session via winget: Rustup (stable MSVC) + VS 2022 Build Tools (C++).
  Verified live in the built app through WebView2's CDP (`--remote-debugging-port=9222` on
  127.0.0.1, scratchpad `cdp.mjs`): code sign-in -> token saved -> dashboard split view -> the
  preview image loads through a signed link.
- **Download page** `frontend/static/descarca/index.html` (+ `assets/download.js`, external
  because the CSP only hashes index.html/200.html inline scripts): detects the OS, reads
  `/downloads/downloads.json` (`{version, files{windows, macos-arm, macos-intel,
  linux-appimage, linux-deb}, sizes}`), enables what exists, "În curând" for the rest and for
  iOS/Android; linked from the marketing nav and footer.
- **CI:** `.github/workflows/desktop.yml` builds Windows, macOS (arm + intel) and Linux on a
  `desktop-v*` tag or by hand, uploading installers as run artifacts (no publishing). Not set up:
  Apple/Windows code signing, the updater key, the store accounts - the owner's to create.
- Next: iOS/Android (`tauri android init` / `tauri ios init`; iOS needs a Mac), the updater,
  a real production server URL for `VITE_SADE_SERVER`.

**Before that:** third `/impeccable critique` (29/40, snapshot
`.impeccable/critique/2026-11-11T08-07-24Z__frontend-src-routes-app.md`) and its four fixes (user:
"rulează-le pe toate", split view on tablets too):
- The dashboard split view starts at **1024px** (was 1280); at 1024 the list's filters turn into a
  chip row and the open delivery stacks the picture over the delivery block.
- The passkey/brand nudges come **after the work on every screen** (`.nudges`, order 2), so a phone
  sees the list first. On a window under 480px the bar drops "Livrări /" and keeps the filter
  name (`.crumb-root`); the search placeholder is "Caută" below 1024px (`dashboard.searchTiny`).
- Phone header: "Fișier nou" stays as a round "+" (`.nf-text` hidden, aria-label kept).
- Job page: the status chip and "Șterge" sit beside the title (`.head-acts`), two steps kept; the
  armed warning (`job.deleteArmed`) and errors show under the lead; the bottom delete section is
  gone.
- Verified live at 1024, 390 (light + dark) and 1440.

**Before that:** second `/impeccable critique` (29/40, no P1; snapshot
`.impeccable/critique/2026-11-11T07-28-27Z__frontend-src-routes-app.md`) and all its fixes (user:
"rulează-le pe toate"; chose compact title + arrow keys):
- Dashboard >= 1280px: compact head (title and lead on one line), the passkey/brand nudges move
  under the window, the window is `max(560px, 100svh - 178px)` so it fits the screen and only its
  panes scroll. Up/Down on a focused row open the previous/next delivery (`onRowsKey`, rows carry
  `data-id`). `JobPanel`: "Pagina fișierului" is a pill next to the status chip; the picture is
  sticky beside the history.
- Brand settings: the company field and the LogoPicker sit right under their rows; the "whole
  picture" choice lives only in the toggle list (`PositionPicker` got `showTiled`); the client's
  address shows a lock + "Mereu" instead of a grey disabled switch; the no-logo row reads "Fără
  logo / Fișierele primesc doar text"; the company row's note is the hint, not the name again.
- Verified live at 1440 light + dark and 390. Arrow keys not seen moving (the demo account has
  one delivery). The headless browser's daemon restarted mid-run and lost its session; the
  scratchpad `relogin.sh` signs it in again through a temporary 8091 log-mailer instance.

**Before that:** `/impeccable critique` of the app against the marketing page's pictures (28/40,
snapshot `.impeccable/critique/2026-11-11T07-05-51Z__frontend-src-routes-app.md`), then all its
fixes (user: "Poti sa te apuci, alege varianta cea mai buna"; the window dots stay, by the
user's choice, so the app looks like the pictures):
- **Dashboard is the mockup's split view** from 1280px: one window, the list (filters + rows) on
  the left, the open delivery on the right (`components/JobPanel.svelte`: file name, recipient,
  chip, the picture beside `DeliveryCard`, link to the full job page). The first row opens by
  default; `?job=<id>` keeps the selection; a plain click selects, a modified click still opens
  `/app/jobs/<id>`. "Fișier nou" (header `?upload=1`) turns the right pane into the upload form
  (`{#snippet uploadForm()}` is rendered there on wide screens, under the list otherwise);
  after an upload the new delivery opens. Narrower than 1280px: list window + form window as
  before, rows go to the job page. The window is a fixed height (clamp 620-940px) with both panes
  scrolling. Sort moved into the window bar (hidden when the window is under 680px); "/" focuses
  the search (`kbd` hint); a "no matches" state offers "Arată toate livrările".
  CSS gotcha found on the way: a container query cannot restyle its own container, so the
  list's grid lives in `.list-grid` inside the `.list-pane` container.
- **Recipient unlock is the hero picture**: title "Originalul fără filigran, <size>" (new
  `originalSize` in `GET /api/payments/status`, from `payment.StatusResult.OriginalSize`), no
  "Plată cu cardul" label, button "Plătește 49 €" (whole prices without decimals, narrow
  symbol), a skeleton while Stripe.js loads, the paid tick drawn as SVG. **Backend:**
  `payment.Service.CreateIntent` now asks Stripe for `payment_method_types: [card]` instead of
  automatic methods (no Bancontact/MB WAY/EPS list; wallets ride on card). An already-open
  PaymentIntent from before is reused as is, so old demo jobs still show the old list until a new
  intent is made. Tests: `payment/service_test.go` (fake backend records the create params).
- **Brand settings lead with the mockup's toggle list** ("Filigranul contului · N din 5
  pornite"): logo row (thumbnail + Change/Add), company name, client address, date, whole picture.
  Company/date toggle `{company}`/`{date}` in the template (`setToken`, canonical order when the
  template is only tokens, appended otherwise); **the client address only turns on** - it ties a
  leaked copy to its recipient; "whole picture" toggles `position: tiled` and remembers the corner.
  The detailed controls follow under "Firma și logo" and "Poziție și mărime"; the raw template
  is behind "Textul exact".
- Removed the kicker above the sign-in statement (`AuthShell`).
- Verified live (headless; signed in through a temporary 8091 instance with
  `MAILER_TRANSPORT=log`, stopped afterwards): dashboard split view at 1440 light + dark, upload
  pane, "/" focus, 1024 and 390 list layouts, brand settings at 1440/390/dark, preview page.
  `npm run check` clean, `go test ./internals/app/...` green. Not seen live: a brand-new
  card-only PaymentIntent (needs a new job; covered by the test), the empty-account split view.
  Not committed.

**Before that:** second pass at matching the marketing page, this time its *mockups*, not just its
tokens (user, via `/design-taste-frontend`: "Vreau acum sa schimbam tot frontend-ul sa arate ca
cel din pagina de prezentare"). Every app page is now built from the marketing page's own UI
pictures; see DESIGN.md "The product window" for the system. In short:
- `app.css`: new tokens `--line`/`--line-2`/`--fill`/`--fill-2`/`--ink-btn`/`--shadow-sm`, radius
  12/8; new shared classes `.page`/`.page-head`/`.back`, `.win`/`.win-bar`/`.win-dots`/`.win-body`,
  `.rows`, `.st` tags, `.chip`, `.avatar`, `.switch`, `.btn-ink`/`.btn-line`/`.btn-sm`/`.btn-block`,
  `.link-u`, `.side-item`/`.side-label`, `.sk`; `.segmented`/`.pill-group` are tonal tracks.
- `/app` layout: marketing header (Livrări / Setări centred, "Fișier nou", account menu holding
  language + theme). `/app` is the "Lista de livrări" window: sidebar filters with counts (Toate,
  În lucru, Deschise, Plătite, Oprite, Eșuate; Tip: Video/Imagini/Audio; sort), rows with
  initials, file name + size and one status tag (`lib/delivery.ts`), search over recipient and
  file name; the upload form is a window beside it with the usage meters in its footer.
  `dashboard.title` is now "Livrări"/"Deliveries".
- **Backend:** `job.Response` gained `fileName` + `sizeBytes` (the original asset's), filled for
  List and Get by a third raw-SQL query in `job.Repo.DeliverySummary` (`assets` table, same
  import-cycle reason as the others). Test: `job/delivery_test.go`.
- Job page: big file-name h1, one window with the player and `DeliveryCard` (pills: Copiază
  linkul / Retrimite / Oprește linkurile, the link field, the history as the mockup's timeline)
  plus hairline details; the stepper shows only while pending/processing.
- Settings: `SettingsTabs` is a page head with segmented tabs (`lead` prop); Brand is two windows
  ("Filigranul contului" + preview); Security is the marketing split (heading left, panel right)
  with the global `.switch`.
- Login/confirm/revoke: new `AuthShell` (statement + two points left, the form card on the warm
  panel right). `MarkBar` is the marketing bar ("Despre SADE" + toggles; `siteLink={false}` on
  the recipient page). Recipient `/preview/[token]`: the hero window (bar with download, media on
  a dark stage, unlock column; "Descarcă originalul" is `.btn-ink`).
- Welcome: visual only (bigger h1, window footer for the actions); flow and copy unchanged.
- Verified live (headless, second instance on 8091 with `MAILER_TRANSPORT=log`): dashboard, job,
  settings, security, login, preview at 1440 and 390, dashboard + job in dark; `npm run check`
  clean; `go test ./internals/app/...` green. Not seen live: confirm/revoke pages after the shell
  change (same component as login), the empty-dashboard state, the Stripe form fully mounted on
  desktop (it was still loading in the shot; it mounted on the phone shot). Not committed.

**Before that:** the whole app now matches the marketing page (user: "vreau sa corespunda tot cu ce
apare in aceasta pagina"). The marketing page **is** `/`: adapter-static's fallback is now
`200.html` (`frontend/vite.config.ts`), `build/index.html` comes from `frontend/static/index.html`,
`internals/app/static.go` serves `/200.html` as the SPA shell when it exists (else `/`, for old
builds/tests), and `buildCSP` hashes the inline scripts of both pages
(`TestStaticMarketingPageAndShell`). `connect-src` gained `https://fonts.googleapis.com` - Stripe.js
fetches the Payment Element's font CSS (`PaymentForm.svelte` `fonts.cssSrc`) from the page. The old
Svelte landing (`src/routes/+page.svelte`) is deleted; `MarkBar`'s "/" link uses
`data-sveltekit-reload`. Look: `app.css` tokens rewritten (see DESIGN.md "Origin"): Reddit Sans +
JetBrains Mono, `--accent` #e8631c with #111 text, pill `.btn`, tonal `.btn-ghost`, sentence-case
labels, headings 500 / -0.025em, `--shadow-card`, 1280px `.container`, `.mono` = text face with
tabular figures (`.code-mono` for real monospace), all `font-weight: 600/700` in components -> 500;
72px header in `/app` and `MarkBar` (login/confirm/revoke/preview). Verified live (headless, Go
server with `MAILER_TRANSPORT=log`): `/`, login, welcome, dashboard (light + dark, 1440 + 390), job
page, settings, recipient preview with Stripe - no CSP errors. Docker Desktop had to be started for
Postgres. Not committed.

**Before that:** a standalone static marketing page built with `/design-taste-frontend` at
`frontend/static/index.html` (+ `frontend/static/assets/` kit, `credits.html`, stand-in Wikimedia
wedding photos). Separate from the SvelteKit `/` route (left untouched). Second session finished it:
the burned-in watermark on the demo photos is now SVG images (`assets/img/wm-*.svg`) instead of
HTML text, the hero demo fits its frame, muted text darkened to 7:1, the FAQ got a mock of the real
(English) preview-ready email, the delivery-panel watermark/logo stretch and timeline wrap fixed.
Gate rounds used up at OPEN; remaining 3 findings waived in `.taste-skill/waivers.json`. (The build
collision noted here was resolved the next turn - see "Latest".)

**Before that:** frontend pass from `/impeccable critique` (25/40; snapshot in `.impeccable/critique/`),
user: "Rulează-le pe toate, în ordine". Off-limits by the user's choice: landing copy/composition,
the API, `/app/welcome`. Frontend only:
- **Recipient page** (`/preview/[token]`): media keeps its own shape (no fixed 16:9 - a phone video
  was a 105px strip), max 78svh; no vertical centring; a context line under the title explains the
  watermark carries the recipient's address (+ "original after payment" when unpaid); once paid,
  the watermarked download becomes a secondary link under "Descarcă originalul"; card note no
  longer assumes the reader knows SADE. `PaymentForm` restyles the Stripe Element on theme change.
- **Colour**: `--accent` is now #b8481a (buttons, white text 5.3:1), new `--accent-ink` (links/text,
  #f0834a in dark) and `--accent-bright` (#e8631c, fills only). Every informative `.faint` /
  `--text-faint` use moved to `--text-muted`; `.section-label` defined once in `app.css`.
- **Targets**: `.btn` min-height 44px; pills, inline links, chips get a 44px hit area via `::after`.
- **Dashboard**: recipient typo guard (`lib/email.ts` `suggestEmail`, blocks the first submit,
  "Adresa e corectă" to keep it); a summary (recipient + exact burned-in text) above the send
  button; list before the upload form at <= 860px with a "Fișier nou" jump; no row/banner side
  stripes; "Deschis" chip neutral (accent no longer a status).
- **Job page**: `<h1>` is the original file name, recipient below; stepper says "Trimis" not
  "Livrat"; delete explains why it is disabled while processing. **DeliveryCard**: load error +
  retry, the disable confirm stays armed until Cancel (was disarmed on blur), resend limit shown.
- **Toasts** with a link last 12 s and pause on hover/focus. **Account menu**: arrow keys / Home /
  End, Escape returns focus; "Setări" also in the top bar (>= 600px). **Login**: the 6-digit code
  sits behind "Ai deschis emailul pe alt dispozitiv?". Theme toggle labels localised.
- Verified: `npm run check` clean, detector 3 -> 0 findings, preview + login live at 375/1440,
  light/dark on a second server instance (port 8091). **/app pages not seen live** (no session
  could be minted for the review) - checked by type-check and reading only. **Restart the Go
  server after `npm run build`** (CSP hashes).

**Before that:** `PRODUCT.md` added at the repo root (`/impeccable init`): users (studios, media
freelancers, agencies, all equal; recipients with no account), purpose (protection + traceability
+ payment as one package), positioning (recipient-traceable branded watermark), constraints,
principles. Answers from the user: all operator types, "both equally", own SaaS Romania-first.
`DESIGN.md` is the visual record; `PRODUCT.md` is the product record - design work reads both.

**Before that:** brand UX pass from /plan-design-review (22 decisions, 3/10 -> 8/10) + a second
/plan-eng-review of those tasks (user: "Implementez DT1-DT9 acum"). Decisions and tasks are in the
same plan file as the brand work (sections "Design review" and "Eng review of the design tasks").

- **Watermark text never leaves the picture, and never loses the recipient** (`internals/ffmpeg/
  textfit.go`, `wmopts.RenderTextFit`). ffmpeg cannot size a font from the text's own width, so
  `Engine.fitText` estimates it (0.62 x font size per character) and fits one line to 90% of the
  frame width (30% per copy when tiled, copies sit a third apart). Text that already fits keeps
  exactly today's size. Too-long template text gives up, in order: the end of the company name
  ("..." ASCII, the deployment font may lack U+2026), the company, the date - **never the
  recipient** (it is what ties a leaked file to its receiver); if the recipient alone is too long
  the font goes below the normal floor (min 6px). The operator's own per-upload text is never cut,
  only shrunk. Callers pass `Request.TextFor(budget)`; the worker's `renderText` supplies it.
  No frame width from ffprobe -> 1280 assumed + Warn. Shared examples: `internals/ffmpeg/testdata/
  textfit.json`; the browser preview mirrors the numbers in `frontend/src/lib/format.ts`
  (`fitWatermarkText`, `renderWatermarkTextFit`, counted in code points) on a 1920px reference
  frame - parity is promised only above ~820px, below that the 18px floor decides on the server.
- **`Probe` is rotation-aware**: a phone video stored 1920x1080 with a 90 degree display matrix
  (or legacy `rotate` tag) is reported 1080x1920, as ffmpeg renders it; cover art (`attached_pic`)
  never wins over the real picture. Logo size and text fit use the rendered width.
- **Brand store** (`stores.svelte.ts`): explicit `status` idle/loading/ready/error, concurrent
  loads share one request, a refresh keeps `ready`, 8 s timeout -> `error`. Welcome and Settings
  show "Reîncearcă" on error instead of an endless "Se încarcă…"; the `/app` dashboard waits for
  the brand before rendering (no dashboard flash before the welcome redirect); the upload form's
  Logo/Text control is neutral while unknown and only says "no logo" once the server confirmed it.
- **Welcome** (`/app/welcome`): step 1 is the question as heading + a live preview; the name is
  optional; name and position are saved once (Continue from step 2, or Skip - Skip keeps what was
  typed); step 2 says "fără logo = doar text"; step 3 copy differs with/without a logo; the CTA goes
  to `/app?upload=1` (upload zone focused, flag removed). Focus moves to each step's heading;
  progress bar has `role=progressbar`.
- **PositionPicker**: a frame shaped like the picture with the five spots where they appear, plus
  a separate "Pe toată imaginea"; real radio inputs in one group (arrow keys, one Tab stop).
- **LogoPicker**: remove asks twice (the file is deleted for good since eng D7), "Logo-ul se
  salvează imediat", `role=alert` errors, `aria-busy`. **Dashboard**: "Adaugă logo" opens the
  LogoPicker inline (chosen file and recipient stay); the text field's placeholder is the exact
  text that would be burned in; `BrandBanner` nudges accounts that skipped setup (no name, no logo).
- **Settings > Brand**: leave-confirm with unsaved edits (not for redirects to /login), sticky
  compact preview + sticky Save bar at <= 860px, the disabled size slider says why.
- Informative notes moved from `--text-faint` (2.9:1) to `--text-muted`. New `DESIGN.md` at the
  repo root documents tokens, components and these rules.
- Verified: `go test ./...` green except the known `TestWatermarkIntegration` Fontconfig crash
  (new: `textfit_test.go` incl. a real rotated MP4 via ffprobe, `wmopts` fit tests); `npm run
  check` clean; live in the headless browser: new account -> welcome (no flash), long name fits,
  arrow keys, no-logo copy, upload CTA focus, inline logo keeps the form, reload keeps "Logo +
  text", Settings at 375px light + dark, and a real upload rendered "Studio Foto si Video pentru
  Evenimente Nunti Botezuri si Corporate Romania · client-x@example.com · 2026-11-07" at 25px,
  inside the frame. **Not verified live**: the brand-load error states and the leave-confirm
  dialog (the headless browser can neither stub requests on Windows nor safely answer a confirm
  dialog) - covered by reading only.

**Before that:** operator brand, reviewed with /plan-ceo-review + /plan-eng-review before shipping
(user: "Brandul operatorului", then "Implementez T0–T9 acum"). Plan + decision ledger + both review
reports: `~/.gstack/projects/ciprianiordache-SADE-Safe-Media-Delivery-App/ceo-plans/2026-11-07-operator-brand.md`.

- **What ships**: new domain `internals/app/brand` (one `brands` row per account: company, logo,
  position `top-left|top-right|bottom-left|bottom-right|center|tiled`, opacity 10-100, logo scale
  5-50 % of the frame width, text template `{company} · {recipient} · {date}`, `onboarded` 0/1).
  Routes, session only: `GET/PUT /api/brand`, `GET/POST/DELETE /api/brand/logo`, `POST
  /api/brand/onboarded`. `job.Service.Create` freezes a snapshot of it into `jobs.watermark_opts`
  (`internals/wmopts`, `{"v":1,...}`) through the optional `job.BrandSource` / `job.BrandAware`
  plug-in, so a later brand change never alters a queued or sent file. The worker turns the
  snapshot into `ffmpeg.Style` (position/opacity/scale, logo path, a 3x3 tiled grid; text size and
  margins scale with the frame width).
- **Rules**: an operator with no logo of their own gets **text only**, whatever kind they pick -
  the server's default logo is never stamped on an operator's files (D17). The upload API no
  longer reads `watermarkOpts`, and `job.Response` no longer returns it (the snapshot holds a
  storage key). The dashboard form disables Logo/Both without a logo, and treats a brand it could
  not load as "no logo" (`kind` is derived, so the operator's choice comes back once it loads).
- **Logo handling**: PNG/JPEG, <= 3 MB, each side 16-6000 px and **<= 4 MP** total (decompression
  guard), at most **2 decodes at once** process-wide (`Service.decode` semaphore, ctx-aware),
  re-encoded to PNG, longest side 800 px, stored as `brands/<user>/<sha256[:8]>.png`. **Only the
  current logo is kept**: replacing or removing deletes the previous file, unless a
  pending/processing job's snapshot still names it (`brand.Repo.LogoInUse`, raw SQL - the worker
  reads the file during the encode, so deleting it mid-encode would fail that attempt). Upload
  attempts are capped at **20 per account per 24h** (`internals/ratelimit`, in memory, 429).
- **Deferred (TODOS.md)**: the company name/logo in the preview-ready email and on the recipient's
  `/preview` page. With open signup anyone could name their "company" after a bank and have SADE
  mail a link to a page with a card form. Gate it on verified/paid senders first. That code was
  removed from main and kept on branch **`wip/brand-email-preview`** (pushed). The email and the
  page are therefore unchanged; the company shows only in the watermark itself.
- **Frontend**: `/app/welcome` (3 steps, shown once to every account incl. existing ones,
  skippable), Settings split into tabs (`/app/settings` = Brand, `/app/settings/security` =
  the former settings page), components `LogoPicker`, `PositionPicker`, `BrandPreview`,
  `SettingsTabs`. `cmd/seed` gives the demo operator a brand ("Studio Demo", `docs/icon150.png`).
- Verified: `go test ./...` green (except the known `TestWatermarkIntegration` Fontconfig crash),
  `npm run check` clean, and live in a headless browser: welcome -> company + logo + position ->
  upload -> worker done with the brand logo + "Studio Live · recipient · date" burned in ->
  recipient email subject still "Your preview is ready" -> `/preview` unbranded, Stripe form loads,
  desktop + 375px. Not verified live: the "brand API down" state of the upload form (the headless
  browser cannot stub requests); covered by reading only.
- Tests: `brand/service_test.go` (pixel cap, decode slots, only-current-logo + in-use check, 20/day),
  `job/brand_test.go` (snapshot frozen, no-logo -> text, no snapshot in the response),
  `worker_test.go` (logo file missing -> text, never the server logo), `router_test.go`
  (`/api/brand*` needs a session), `notify_test.go` (email stays generic), `wmopts_test.go`,
  `ffmpeg/style_test.go`.

**Before that:** delivery signals on the dashboard list (user: "Luam totul in ordine sa incepem cu
indicatorul"). `job.Response` gained `openedAt` (latest "viewed" event, omitted if never) and
`paid`, filled for both `List` and `Get` by `service.addDelivery` from the new
`job.Repo.DeliverySummary(ids)` - two raw-SQL queries per page (`job_events` kind `viewed`, and
`payments` status `paid`; paid comes from payments, not events, so purchases from before the
history existed still show). Raw SQL because `job_event`/`payment` tests import `job`, so `job`
importing them would be a test import cycle. A failed lookup only logs - the list still loads.
Frontend: per-row chips on `/app` - "Plătit" (green), "Deschis acum 2h" (accent, full date on
hover; `format.ago`), "Nedeschis" (faint, done jobs with links on and no payment - the
"maybe resend" signal), "Link-uri oprite" (red) - plus two filters, "Deschise" and "Plătite",
shown only when they have matches. Test: `job/delivery_test.go`. Verified live on desktop and
375px.

**Before that:** SaaS phase 2 - delivery links and history on the job page (user: "Hai sa
continuam", picking up the roadmap's next step). Verified in tests and live (headless browser +
curl as the recipient):

- **History**: new table `job_events` (`internals/app/job_event`, kinds `sent`, `resent`,
  `viewed`, `downloaded`, `paid`, `original_downloaded`, `links_disabled`, `links_enabled`;
  `created_at` set explicitly in UTC). Writers: `recordingNotifier` in `worker_wiring.go` wraps
  the worker's `EmailNotifier` ("sent" once the email went out); `share.Handler` records
  "viewed"/"downloaded"/"original_downloaded" via `RecordOnce` (30 / 10 min windows, and only for
  a request starting at byte 0 - a player's range requests are one view); `payment` records "paid"
  on both paths (reconcile + webhook) via its new optional `UseDelivery(links, events)` (reached
  through the `payment.Deliverer` interface, since `NewService` returns the `Service` interface).
- **Links switch**: `jobs.links_disabled` (int, migration `2026-11-03-job-links-disabled`,
  `job.Response.linksDisabled`). When set, `/p` `/d` `/o` answer 410 "this link has been disabled"
  (`share.errDisabled`), and payment's `resolvePreview` returns `ErrLinksDisabled` (410) for
  status and checkout - `Status` checks it even with Stripe off, so the preview page can say so.
  Switching off is refused (409) once the job is paid, like delete.
- **Operator routes** (`share.OperatorHandler`, owner-scoped through `job.Service.Get` with the
  new exported `job.UserIDFrom(ctx)`): `GET /api/jobs/{id}/delivery` (`{enabled, paid, ready,
  previewUrl, expiresAt, events}` - a freshly signed `/preview/<token>` link, so the operator can
  finally copy what the recipient got), `POST /api/jobs/{id}/resend` (max 3 per job per hour,
  429 beyond), `PUT /api/jobs/{id}/links {enabled}`. app.go builds one `EmailNotifier` shared by
  the worker and resend (so resend works even with ffmpeg missing).
- **Frontend**: `components/DeliveryCard.svelte` in the job page's right column (status pill,
  copy link, resend, two-step disable / re-enable, timeline with icons; polls every 15s and
  reloads when the job finishes); `/preview/[token]` shows a "Link dezactivat" card on 410.
- Not done: delivery signals on the dashboard list (e.g. an "opened"/"paid" marker per row) - the
  list endpoint carries no event data yet.

Tests: `share/delivery_test.go` (switch -> 410, range requests -> one view, owner scoping,
resend limit, no disable after payment), `job_event/repo_test.go`.

**Before that:** SaaS phase 1 - schema migrations + stronger sign-in, keeping the magic link (user:
"Eu totusi vreau sa ramanem la usurinta unui magic link dar vreau sa vedem daca putem sa
inbunatatim acest lucru si sa il facem mai sigur"; compared against Lidl's password + SMS code -
recommended passkeys instead: phishing-resistant, no SIM-swap, no per-SMS cost). Verified in
tests (incl. a software WebAuthn authenticator) and live (headless browser + curl as "another
device"):

- **Migrations** (`internals/database/migrations.go`): `db.RunMigrations(...)` applies each
  `Migration{ID, Apply}` once, tracked in `schema_migrations`, right after `db.Migrate(Models())`
  (app.go + cmd/seed). `db.AddColumn(table, col, KindString|KindInt|KindTime)` is idempotent
  (checks `information_schema` / `pragma_table_info`) and adds `NOT NULL DEFAULT <zero>`, since
  crud-depot cannot scan NULL into a plain string/time. **No bool kind**: crud-depot can't convert
  SQLite's INTEGER back to a Go bool - flags are `int` 0/1 (e.g. `users.require_passkey`). The list
  lives in `internals/app/migrations.go` - append only.
- **Link bound to the browser + 6-digit code**: `POST /api/auth/request` sets an HttpOnly
  `sade_login` cookie (Path `/api/auth`, SameSite Strict, MagicLinkTTL; reused on resend) and the
  token row stores `browser_hash` + `code_hash` (HMAC with `Auth.HMACSecret`, bound to the token)
  + `attempts`. `/api/auth/complete` only signs in from that browser (409 `other_browser`
  otherwise, link kept); `/api/auth/verify` returns `sameBrowser`. The email also carries the
  code: `POST /api/auth/code {email, code}` works from the same browser, 5 wrong codes kill the
  email (`magic_token.Repo.ConsumeByCode`), per-IP limiter `AuthCodeLimiter` (4x the request
  budget). A stolen/forwarded link is useless on its own.
- **Sign-in history + new-device alert**: new table `login_events` (`internals/app/login_event`).
  A sign-in from a user agent the account hasn't used in 90 days (and not the account's first
  sign-in) emails "New sign-in" with a signed `revoke-sessions` token ->
  `FrontendURL/auth/revoke#token=` (new page) -> `POST /api/auth/revoke` ends every session.
  Sent async (`Service.async`; tests make it sync). History purged after 90 days by PurgeExpired.
- **Devices**: `sessions` gained `user_agent`, `ip`, `last_seen_at` (touched at most every 5 min
  by `AuthenticateSession`, which also returns the session id - the router's `Auth` middleware
  puts it in context via `auth.WithCaller`). `GET /api/account/sessions`, `DELETE
  /api/account/sessions/{id}`, `GET /api/account/activity`. `auth.DescribeUserAgent` -> "Chrome on
  Windows" (frontend `deviceLabel` localises "on").
- **Passkeys** (`github.com/go-webauthn/webauthn`, new table `passkeys` storing the whole
  `webauthn.Credential` as JSON): register from Settings (`/api/account/passkeys/begin|finish`,
  list, delete), passkey-only sign-in on /login (`/api/auth/passkey/begin|finish`, discoverable),
  and **"Require a passkey at every sign-in"** (`PUT /api/account/require-passkey`, needs >= 1
  passkey; deleting the last one turns it off so nobody is locked out): the email step then returns
  `{status:"passkey_required", passkey:{ceremonyId, options}}` instead of a cookie. Open
  ceremonies live in memory (5 min, single-use). `app.go`'s `newWebAuthn` sets RPID = host of
  `FrontendURL`; **passkeys are off on a bare IP** (WebAuthn forbids it) and are tied to the
  hostname (a new trycloudflare URL means new passkeys). `GET /api/auth/config` tells the login
  page whether to offer them. Frontend: `lib/webauthn.ts` (base64url <-> ArrayBuffer, no library),
  `lib/signin.ts`, `/app/settings` (passkeys, require toggle, devices, activity; "Setări" in the
  account menu), `PasskeyBanner` on the dashboard (dismissable), code field on /login's sent
  screen, other-browser + passkey-step states on /auth/confirm.
- **Timezone bug found on the way (pre-existing):** timestamp columns are Postgres `TIMESTAMP`
  (no zone) and the driver wrote the local wall clock - on this Romanian machine every stored time
  was +2h, read back as UTC. Expiry checks ran 2h late (a "15 minute" link lived ~2h15m) and the UI
  showed shifted times. `main.go` and `cmd/seed` now set `time.Local = time.UTC`. Rows written
  before the fix stay shifted (dev data only; they age out).
- Not done (deliberately): SMS, TOTP, step-up re-auth, admin recovery for a user who loses every
  passkey with the requirement on (deleting passkeys needs a session; documented in the UI: add
  two).

Tests: `internals/database/migrations_test.go`, magic_token browser/code tests,
`auth/security_test.go`, `auth/passkey_e2e_test.go` (soft authenticator: register, passkey-only
sign-in, wrong key refused, email + required passkey), `internals/app/account_test.go`.

**Before that:** a frontend usability pass (user: "Fa un research si sa aducem inbunatatiri
frontend-ului, nu trebuie sa ingreunam, trebuie facut intuitiv, usor de utilizat"). Frontend only,
no API change. Verified live in the headless browser (desktop + 375px), `npm run check` clean.

- **Upload:** the dropzone said "drag a file" but had no drop handler - real drag & drop now.
  `api.createJob` uses XHR (fetch has no upload progress) with `onProgress` + `AbortSignal`: a
  progress bar with % and a Cancel button. The picked file shows as a card (icon, name, size,
  Change / remove). The text field hides for "Logo" (ffmpeg ignores text there, and it is no
  longer sent). Last watermark kind is remembered (`localStorage` `sade:watermarkKind`).
- **Bug found while testing:** a poll that ran during an upload already fetched the new job, then
  the upload's own `[job, ...jobs]` added it again - a duplicate `{#each}` key aborted the update
  (form not reset, no toast). Now deduped by id.
- **Feedback:** `lib/toast.svelte.ts` + `components/Toasts.svelte` (rendered once in the root
  layout). Toasts on upload, on delete, and when a polled job goes pending/processing -> done or
  failed ("Done! The preview was sent to X", with a View link). Polling skips hidden tabs.
- **Job list:** one list view (dropped the grid - its thumbnails were empty black boxes - and the
  density toggle and rows-per-page). Media-type icon per row (`components/MediaIcon.svelte`),
  media type / watermark kind translated (were raw `video · both`), full date on hover
  (`format.fullDate`), filter chips with zero matches hidden, pagination (15) only when needed,
  skeleton rows while loading. Upload card is sticky on desktop.
- **Job detail:** failed jobs get an explanation banner instead of a silently missing stepper;
  stepper's current step pulses and has a one-line hint; "Details" and "Files" cards merged;
  sizes via `format.formatBytes`; attempts shown only when > 1; the player switches to the preview
  once it appears.
- **Login:** a "Check your email" screen with the address, resend with a 30s countdown, "Use a
  different email", 429 mapped to a localized message; autofocus/autocomplete on the field.
- **Other:** landing gets a 3-step "How it works"; preview page gets an unlock heading,
  `noindex`, localized footer and a clearer RO download label; per-page `<title>`s; `<html lang>`
  follows the locale; account menu closes on outside click / Escape and shows an avatar (email
  hidden under 600px, shown inside the menu); `i18n.t(key, vars)` fills `{name}` placeholders;
  `prefers-reduced-motion` respected; mobile overflow fixed (`minmax(0, 1fr)` grids,
  `min-width: 0` on the search box).

Reminder from this pass: **restart the Go server after `npm run build`** - the CSP hashes of
`index.html`'s inline scripts are read once at startup, so a stale server blocks the new build.
Follow-up (user saw a blank page): the SPA shell was also served with no `Cache-Control`, so
browsers cached it heuristically and could pair an old shell with a new CSP (or the reverse) -
blank page, CSP errors in the console. `spaFileServer` now sends `no-cache` for everything except
`/_app/immutable/*` (`public, max-age=31536000, immutable`). `TestStaticCacheHeaders`.

**Before that:** a general bug hunt (user: "vreau sa te uiti de posibile probleme in aplicatie si sa
incercam sa le rezolvam"). Found and fixed:

- **15s server timeouts cut uploads and downloads.** `ReadTimeout`/`WriteTimeout` (15s) applied
  to every route, so a large upload (cap is 2 GiB) or a slow video stream died mid-transfer.
  New `LongRequest(d)` middleware (`internals/app/middleware.go`) sets per-request read+write
  deadlines via `http.NewResponseController` on `POST /api/jobs`, the asset-content route and
  `/p` `/d` `/o`; `config.ServerConfig.LongRequestTimeout` (2h). `statusWriter` gained
  `Unwrap()` so the controller reaches the real conn through `RequestLog`. Also
  `ReadHeaderTimeout` (10s, new config) on the `http.Server`. Regression test
  `TestLongRequestOutlivesServerWriteTimeout` (200ms server timeout vs a 400ms handler).
- **A worker panic killed the whole server** (unrecovered panic on a goroutine) and left the job
  `processing`. `Pool.safeProcess` recovers it into an ordinary job error (retry/fail as usual).
  `TestHandleRecoversFromPanic`.
- **Stuck jobs were only requeued at startup.** `dispatch` now runs `ResetStuck` every
  `StuckJobTimeout/2` too. `config.Validate` requires `StuckJobTimeout > JobTimeout`, otherwise
  that would requeue a job still being encoded.
- **Reprocessing duplicated the preview.** `asset.Repo.UpsertPreview` (used by
  `worker_wiring.go`'s `AddPreview`) updates the job's existing preview row and keeps its id, so
  already-emailed links (signed over that id) stay valid. `TestUpsertPreviewKeepsOneRowAndItsID`.
- **Expired magic tokens and sessions were never deleted** (only on reuse). `DeleteExpired` on
  both repos (raw SQL via new `database.Rebind`), `auth.Service.PurgeExpired`, run at start and
  hourly by `App.purgeExpired` in `app.go`.
- **No way to delete a job**, so an operator at the storage quota was stuck for good. `DELETE
  /api/jobs/{id}` (`job.Service.Delete`): owner-scoped, 409 `ErrBusy` while processing (atomic
  `DeleteIfNotProcessing`, so the worker can't claim it mid-delete), 409 `ErrPaid` once a payment
  is paid (`Repo.HasPaidPayment`, raw SQL - job can't import payment, payment's tests import job).
  Rows go first, then blobs. Local `storage.Delete` now also removes the object's own (now empty)
  per-job directory. Frontend: two-step "Șterge" / "Confirmă ștergerea" section at the bottom of
  `/app/jobs/[id]`. Verified live end to end.
- **Session cookie could go over HTTP with an https PublicURL.** `app.go` forces
  `SessionCookieSecure=true` when `App.PublicURL` is https.
- **Dashboard stayed on a dead session.** `api.ts` fires `sade:unauthorized` on any 401 except
  `/api/me`; the `/app` layout sends the user to `/login`. Verified live: logout-all from another
  client, then a click in the dashboard goes to /login.
- **Upload errors were English in a Romanian UI.** `uploadErrorMessage` maps 429/413/415/400 to
  i18n strings (`dashboard.error*`).
- `UploadConfig.KeepOriginals` documented as reserved/ignored: the paid unlock serves the
  original, so it is always kept.

Checked and left alone: the rate limiter's map is swept; worker state writes ignore ctx (the
adapters drop it), so an expired job ctx cannot fail `MarkDone`.

**Before that:** security audit (VibeSec skill, report delivered in Romanian at the user's request) and
the fixes for every finding, plus a first step toward a SaaS shape - the user's ask, verbatim:
"vreau sa rezolvam toate aceste probleme, imbunatatim autentificarea si incercam sa facem acest
serviciu mai apropiat de un saas". What shipped, by finding:

- **ffmpeg filtergraph (was high):** operator watermark text was escaped into a single-quoted
  `drawtext text='...'` value, and the escaping could not keep it inside the value. The text no
  longer enters the graph at all: `Engine.Watermark` writes `SanitizeText(req.Text)` to an
  `os.CreateTemp` file next to the output, and `drawtext` reads it via
  `textfile='...':expansion=none` (`internals/ffmpeg/filters.go`; `Request.textFile` is
  unexported, set only by `Watermark`). `escapeDrawtextText` is gone; `escapeFilterPath` handles
  server-side paths only, and `Watermark` refuses a font/text path containing `'`.
  `SanitizeText` flattens control chars and caps at `MaxTextRunes` (200); `job.Service.Create`
  rejects longer text and non-JSON / >2 KiB `watermarkOpts` as `ErrInvalidInput`. Verified with
  the real ffmpeg via `cmd/watermark`: `Test: 50% it's ok` renders literally.
- **Open signup (was high):** kept open on purpose - self-serve is the SaaS default - and bounded
  instead: `config.UploadConfig.JobsPerDay` (25, rolling 24h) and `StorageQuotaMiB` (10 GiB,
  originals + previews; checked against the declared size *and* again against the stored size).
  Admins are exempt via `job.WithQuotaExempt(ctx)`, set by the router's `operator` wrapper.
  `GET /api/usage` (`job.Handler.Usage`) feeds the dashboard's new `UsageMeter.svelte`.
  `job.Repo` gained raw-SQL `CountCreatedSince` / `StorageBytes`. `Auth.SignupMode`
  (`open|allowlist|closed`, + `SignupAllowedEmails/Domains`) restricts new accounts; a refused
  address gets the same 202 (no enumeration). **No per-user `plan` column**: schema-builder
  cannot `ADD COLUMN` to an existing table (checked - no such code), so plans/billing for
  operators need a new table or a migration step first - the natural next SaaS step.
- **Payment reopen/hide (was medium):** `payment.Service` now treats a job as paid if *any*
  payment is paid (`Repo.ListByJob`; `LatestByJob` kept), `CreateIntent` returns
  `ErrAlreadyPaid` (409) for a paid job and reuses the newest pending PaymentIntent while
  Stripe still accepts confirmation, and both `reconcile` and the webhook require
  `coversPayment` (amount received >= price, same currency) before marking paid.
- **Payment endpoints unbounded (was medium):** `PaymentConfig.RateLimit/RateWindow` (60 / 15m)
  per client IP on checkout + status (`Deps.PaymentRateLimiter`).
- **Tokens in logs (was medium):** `RequestLog`/`Recover` log `redactPath(path)` -
  `/p/ /d/ /o/ /preview/` become `<prefix>***` (`internals/app/security.go`). Verified live.
- **No CSP (was medium):** `buildCSP(frontendDir)` - `script-src 'self'` + Stripe + sha256
  hashes of the inline scripts in `frontend/build/index.html`, read once at startup (restart after
  a frontend rebuild). Styles keep `'unsafe-inline'` (Svelte style attributes). Plus COOP,
  Permissions-Policy, and `Cache-Control: no-store` on `/api/*`. Verified live: confirm page and
  dashboard render with zero console errors. `/preview/[token]` verified live too (user approved
  running `go run ./cmd/seed -links`, which is read-only - it returns before the `TRUNCATE`):
  the Stripe Payment Element mounts under the CSP (7 `js.stripe.com` frames, no CSP violations,
  only Stripe's usual test-mode/HTTP warnings), a reload keeps one `payments` row (intent reused),
  `POST /api/payments/checkout` on a paid job answers 409 `already paid`, and the paid job's page
  shows "Original deblocat" with a working `/o/` link.
- **CSRF (was low):** `OriginCheck(originGuard)` in the global chain (after CORS, before Auth):
  a POST/PUT/PATCH/DELETE whose `Origin` isn't PublicURL/FrontendURL/CORS list and whose
  `Sec-Fetch-Site` is `same-site`/`cross-site` (or has a foreign/`null` Origin with no
  Sec-Fetch-Site) gets 403. No Origin and no Sec-Fetch-Site = non-browser client, allowed.
  `/api/payments/webhook` exempt.
- **Login CSRF / link prefetch (was low):** the email now links to the **frontend**
  `FrontendURL/auth/confirm#token=<raw>` (fragment - never sent to a server or logged). New page
  `frontend/src/routes/auth/confirm/+page.svelte` strips the hash, calls `POST /api/auth/verify`
  (`Service.Peek` / `magic_token.Repo.Peek` - does not spend) to show the account, and signs in
  only on click via `POST /api/auth/complete`. `GET /api/auth/callback` still exists for old
  emails but only redirects to the confirm page (token charset-checked) - **a GET never signs
  in anymore**. Also a per-recipient-address limiter (`Service.SetEmailLimiter`, same budget as
  the per-IP one) so rotating IPs can't flood one mailbox.
- **Session revocation (was low):** `session.Repo.DeleteByUser`, `auth.Service.RevokeAll`,
  `POST /api/auth/logout-all` (account menu: "Deconectare de pe toate dispozitivele"), and
  `user.Handler.OnRoleChanged` (wired to `RevokeAll` in `app.go`) ends sessions on a role change.
- **Original link forwarding (was low, by design):** a preview-link holder can still unlock the
  original after payment (payment is per job, not per person). Mitigated, not removed:
  `PaymentConfig.OriginalLinkTTL` (24h) instead of the 30-day share TTL - the payer's page re-mints
  on every visit, so only forwarded `/o/` copies are affected.
- Also: `job.Response.Error` is now a generic "processing failed" (`publicError`) - the stored
  message has ffmpeg stderr with server paths; `/o/` added to `static.go`'s reserved prefixes;
  `.gstack/` (browse reports) gitignored.

New tests: `internals/app/security_test.go`, auth (`TestPeekDoesNotSpendTheToken`,
`TestSignupPolicy`, `TestPerAddressLimit`, `TestRevokeAllEndsEverySession`,
`TestHandlerVerifyThenComplete`, callback-forwards test), job quotas/validation, payment
reuse/paid-stays-paid/underpaid. Router e2e tests now sign in via `completeSignIn` (POST, like the
frontend). `go test ./...` green except the pre-existing `TestWatermarkIntegration` Fontconfig
crash on this Windows box.

**Before that:** portfolio screenshots taken from the `cmd/seed` demo dataset (app run on localhost with
`MAILER_TRANSPORT=log` and `APP_PUBLIC_URL`/`AUTH_SESSION_COOKIE_SECURE` overridden via process
env, which `godotenv` never clobbers, so `.env`'s tunnel values stayed untouched) and embedded in
`README.md`'s new "Screenshots" section: `docs/dashboard.png`, `docs/job.png` (job detail),
`docs/preview.png` (recipient `/preview/[token]` page with the Stripe Payment Element), and
`docs/unlocked.png` (the same page after payment, "Download the original"). Retaken after the seed switched
its video jobs to `docs/test.mp4` and every address to `@example.com`.

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

**Still open here:** ~~a CSP~~ (done - see "Latest"), and `config.yaml` at the repo root still carries the old
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
  a single paginated list, toasts on upload/finish), `/app/jobs/[id]` (detail with a status
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
- `go run ./cmd/seed` - **destructive**: `TRUNCATE`s every table and deletes `originals/`+`previews/`
  from storage, then seeds a demo dataset for screenshots: operator `operator@example.com`, 12 jobs
  (video jobs are stream copies of the checked-in `docs/test.mp4` - run from the repo root; image/audio
  generated with ffmpeg's `mandelbrot`/`gradients`/`aevalsrc` sources; all uploaded through the real
  `job.Service.Create`), 2 `paid` payments. The 8 "done" jobs are left `pending` for the app's own
  worker to watermark on next start; failed/processing/pending demo rows are pinned with
  `next_attempt_at` a year out. **Every** seeded address - operator and all recipients - is on
  `example.com` (RFC 2606 reserved), so no transport can ever mail a real person; log in by requesting
  a link for `operator@example.com` with `MAILER_TRANSPORT=log` and taking it from the log. `-links` prints signed
  `/preview/<token>` URLs for every preview asset. Run the app for a demo with
  `MAILER_TRANSPORT=log` so the worker's preview-ready mail to the dummy recipients is only logged.
  Audio jobs need `assets/watermark/audio.mp3` (gitignored, per-deployment like the logo/font) - it
  was missing on this machine, so every audio upload failed until a short chime was generated there.
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
| `login_event.LoginEvent` → `login_events` | `method` (`link`/`code`/`passkey`), `user_agent`, `ip`, `created_at` indexed | `user_id` → `users` cascade |
| `passkey.Passkey` → `passkeys` | `credential_id` unique (base64url), `credential` (go-webauthn JSON), `name`, `last_used_at` | `user_id` → `users` cascade |
| `job_event.JobEvent` → `job_events` | `kind` (sent/resent/viewed/downloaded/paid/original_downloaded/links_disabled/links_enabled), `detail`, `created_at` indexed | `job_id` → `jobs` cascade |
| `brand.Brand` → `brands` | `user_id` unique, `company_name`, `logo_key` (`brands/<user>/<hash>.png`, "" = none), `position`, `opacity_pct`, `logo_scale_pct`, `text_template`, `onboarded` (0/1) | `user_id` → `users` cascade. Snapshot copied into `jobs.watermark_opts` at upload |

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

Operator login is a magic link (no password), bound to the requesting browser, with a 6-digit
code from the same email as the cross-device path and optional passkeys (alone, or required as a
second step) - see "Latest" above for the details. The paragraph below predates that.

Operator login is magic-link only (no password). `POST /api/auth/request {email}` →
`user.EnsureByEmail` (find-or-create an operator) → a random 32-byte token, sha256-hashed and
stored with `MagicLinkTTL` → email carries `FrontendURL/auth/confirm#token=<raw>` (fragment;
`Auth.SignupMode` and a per-address limiter can silently skip the send). That page calls `POST
/api/auth/verify` (peek, shows the account) and, on the user's click, `POST /api/auth/complete`,
which spends the token (atomic delete) and issues a DB-backed session: a random secret in the
`sade_session` cookie (HttpOnly, `Secure` per `Auth.SessionCookieSecure`, SameSite=Lax,
`SessionTTL`), only its sha256 stored. `Auth` middleware attaches the user on every request;
`/api/me` returns it; `/api/users*` additionally needs `RequireRole("admin")`. `POST
/api/auth/logout` deletes the session row and clears the cookie; `POST /api/auth/logout-all`
deletes every session of the user. `GET /api/auth/callback` only redirects old links to the
confirm page. Recipients of watermarked
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
status filter / sort / pagination are all
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
