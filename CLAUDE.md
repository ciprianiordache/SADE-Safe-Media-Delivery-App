# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

**M1–M5 done. The whole app works end to end, backend and frontend:** an operator signs in via
magic link, uploads a file from the SvelteKit dashboard → the job is stored → the worker
watermarks it async → the preview is stored → the recipient is emailed signed `/p/<token>` (view)
+ `/d/<token>` (download) links → the public `share` handlers stream the watermarked preview with
Range support, no login → the recipient's `/preview/[token]` page embeds/downloads it. Only
hardening (M6) remains (plus the `payment` backlog). Done and tested:

- `config/` — full loader (defaults < YAML < env), `config.Duration`, secret generation, `Validate`.
  `Auth.ShareTokenTTL` (default 30d) bounds the signed `/p` and `/d` links.
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
  writes the `Job` row (`status=pending`) + the original `asset.Asset` row, and rolls both back
  (blob + row) on any later failure. `Get`/`List` are owner-scoped (a non-owner's job reads as 404).
  Handler: `POST /api/jobs` (multipart `file` + `recipientEmail`/`watermarkKind`/`watermarkText`/
  `watermarkOpts`, `MaxBytesReader`-guarded), `GET /api/jobs`, `GET /api/jobs/{id}`.
  `Repo` also carries the worker-facing state machine (raw SQL, dialect-aware via `db.Driver()`):
  `ClaimPending` (atomic `UPDATE … RETURNING`, `FOR UPDATE SKIP LOCKED` on Postgres, bumps
  `attempts`), `MarkDone` / `MarkFailed` / `MarkForRetry(nextAttemptAt)` / `ResetStuck(cutoff)`.
  `Job` gained a `next_attempt_at` column (zero = ready now) gating (re)claims for retry backoff.
- **`internals/app/asset/`** — repo only (`Create` / `GetByID` / `ListByJob` / `GetByJobAndKind` /
  `Delete`), like `magic_token`/`session`. Exported `ToResponse`/`ToResponses` so `job` renders
  asset rows in its detail response. No handler — assets reach clients via `share`.
- **`internals/app/share/`** — the public, login-free preview links. `Handler.Preview` (`GET
  /p/{token}`, `Content-Disposition: inline`) and `Handler.Download` (`GET /d/{token}`,
  `attachment`) both: `signer.Verify(purpose, token)` (`"preview"` for `/p`, `"download"` for `/d`)
  → subject is the preview asset id → `Assets.GetByID` → must be `KindPreview` → stream from
  `Blob`. Local files go through `http.ServeContent` (Range / conditional GET, `Accept-Ranges`,
  `nosniff`); a non-seekable backend falls back to a whole-object copy. Error map: `token.ErrExpired`
  → 410, everything else (bad/again wrong-purpose token, unknown asset, original asset, missing
  blob) → an indistinguishable 404; repo error → 500. Declares its own `Assets` + `Blob` ports
  (`asset.Repo` and `storage.Storage` satisfy them directly — no adapters).
- **`internals/app/magic_token/` + `internals/app/session/`** — repos (`Consume` is atomic
  delete-on-use; `GetByHash` / `Delete`).
- **`internals/app/router.go` + `middleware.go`** — central mux, `Recover`/`RequestLog`/`CORS`/`Auth`
  chain, `RequireUser` / `RequireRole`, `UserFrom(ctx)`, `GET /api/me`. The `operator` wrapper
  guards the `/api/jobs*` routes with `RequireUser` and passes the operator id down via
  `job.WithUserID(ctx, id)` (keeps `job` from importing the auth layer). `GET /p/{token}` and
  `GET /d/{token}` are public (no cookie) — the signed token in the path is the whole authz.
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
- `internals/storage/` (local disk, wired via `job` + `worker` + `share`), `internals/ffmpeg/`
  (`os/exec` engine + `-progress` parsing, wired via `worker`), `internals/mailer/`
  (`smtp`/`log`/`noop`), `internals/token/` (HMAC share tokens, wired via `share` for verify and
  `worker.EmailNotifier` for signing — `"preview"` and `"download"` purposes).
- `cmd/watermark/` — CLI that runs the engine on one file with a live progress bar.
- `docker-compose.yml` — Postgres on host port 5433.
- `internals/app/integration_test.go` (full-graph round-trip) + `router_test.go` (real
  request→callback→cookie→`/api/me`→logout, the full job upload→list→detail flow, and a
  seed-preview→sign→`/p`+`/d` fetch over `httptest`).

Not started: `repo.go` / `service.go` / `handler.go` for `payment` (backlog), and `frontend/`
(default SvelteKit skeleton). `mailer` is wired via `auth` + `worker`; `storage` via `job` +
`worker` + `share`; `ffmpeg` via `worker`; `token` via `share` + `worker`.

### Resume here (if the session reset)

**Last shipped:** M5, the SvelteKit frontend (`frontend/src/`), plus the Go-side static handler
that serves it — then a full visual redesign of that same frontend against a Claude Design canvas
the user directed (link in their local session history, not reproduced here). Backend + frontend
now cover the whole flow M1–M5, restyled.

- Frontend routes built: `/` landing, `/login` (magic-link request + `?error=invalid_link`
  banner), `/app` (guarded layout + upload form + a searchable/filterable/sortable job list with
  list/grid views and comfortable/compact density), `/app/jobs/[id]` (detail with a status
  stepper + assets, polls while pending/processing), `/preview/[token]` (public, no auth, mobile-
  first — cascades `<video>` → `<audio>` → `<img>` → plain link against `GET /p/{token}`,
  downloads via `GET /d/{token}`). See the "Design system" paragraph above for the palette/
  component/scope details of the redesign. `src/lib/`: `api.ts` (typed fetch client,
  `credentials: 'include'`, `ApiError`), `types.ts` (hand-kept mirror of the Go `Response` DTOs +
  `isAllowedFile`), `format.ts` (`relativeTime`), `stores.svelte.ts` (`auth`, a class w/
  `$state`), `theme.svelte.ts` + `i18n.svelte.ts` (ditto — Svelte 5 runes only work in
  `.svelte`/`.svelte.ts` files, so anything stateful is named `*.svelte.ts`, not the plain `.ts`
  the plan sketched), `components/` (`LangThemeToggle`, `MarkBar`, `StatusBadge`, `StatusDot`).
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
- Not wired: the worker's `EmailNotifier` still mails the raw `PublicURL/p/<token>` link, not
  `PublicURL/preview/<token>` — intentionally, per the plan ("frontend preview page may just
  embed/redirect to those"). Revisit only if a wrapped player page in the email is wanted later.

**Next — M6 (hardening):** rate-limit on `POST /api/auth/request`, deeper upload validation
(ffprobe the upload at `job.Service.Create` time, not just extension), retry/backoff review, a
full-flow integration test, and a README. Also worth a pass: wire `APP_FRONTEND_DIR`/build into
whatever deploys this (the frontend needs `npm run build` before the Go binary can serve it — no
build step exists yet in `docker-compose.yml` or elsewhere).

- **Spec:** `Writerside/topics/` (`Default-topic.md` = product goal, `sever.md` = block components).
- **Agreed build plan:** `docs/SADE-plan.pdf` — read it before starting any feature. It defines
  the target file tree, the milestones (M0–M6), and every decision below.
- **Repo:** `github.com/ciprianiordache/SADE-Safe-Media-Delivery-App` (`origin`, branch `main`).
  Line endings are normalised to LF via `.gitattributes`. **Workflow: after every commit, also
  `git push origin main`** (the user wants GitHub kept in sync with each commit).

SADE (Safe Media Delivery): an operator uploads a media file (video/audio/image), the backend
applies a watermark asynchronously, then emails the recipient a signed link to the watermarked
preview. Backlog: payments unlock the original file.

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
| `payment.Payment` → `payments` | *(backlog, not wired)* `provider`, `provider_ref`, `status`, `amount_cents`, `currency` | `job_id` → `jobs` cascade |

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
  `"preview"` capability for the preview asset id and mails `PublicURL/p/<token>`. DB row is the
  source of truth. Local storage only (needs `LocalPath`); S3 staging is a TODO.
- `internals/mailer/` — `mailer.New(cfg, log)` → `Mailer.Send(ctx, Message)`. Transport from
  `Mailer.Transport`: `smtp` (STARTTLS on 587, implicit TLS on 465, PLAIN auth when a user is
  set), `log` (renders to the logger — the local magic-link path; body at Info, full RFC 5322 at
  Debug), `noop` (discard). `build()` assembles the message once (quoted-printable,
  `multipart/alternative` when `Message.HTML` is set, RFC 2047 subject); templating is the
  caller's job.
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
