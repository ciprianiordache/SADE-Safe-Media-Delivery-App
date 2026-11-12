package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"sade/config"
	"sade/internals/app"
	"sade/internals/app/asset"
	"sade/internals/app/auth"
	"sade/internals/app/brand"
	"sade/internals/app/job"
	"sade/internals/app/job_event"
	"sade/internals/app/login_event"
	"sade/internals/app/magic_token"
	"sade/internals/app/passkey"
	"sade/internals/app/payment"
	"sade/internals/app/session"
	"sade/internals/app/share"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/ffmpeg"
	"sade/internals/logger"
	"sade/internals/mailer"
	"sade/internals/ratelimit"
	"sade/internals/server"
	"sade/internals/storage"
	"sade/internals/token"
	"sade/internals/worker"

	"github.com/go-webauthn/webauthn/webauthn"
	stripe "github.com/stripe/stripe-go/v82"
)

// App is the wired-together application: configuration, the shared logger,
// the database pool, and the HTTP server. NewApp builds the graph in
// dependency order; Run serves until the context is cancelled; Close releases
// everything in reverse order.
//
// The logger built here is THE logger for the process. Every component that
// logs is handed this instance (tagged with the environment) or a child of it
// via With - server, database, mailer and every feature package take it.
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	logGC  io.Closer
	db     *database.Database
	server *server.Server
	worker *worker.Pool // nil when ffmpeg is unavailable
	auth   *auth.Service
}

// NewApp loads config, starts the logger, connects the database and ensures
// the schema, builds the mailer and the domain repo -> service -> handler
// chains, and assembles the HTTP server. On any failure it unwinds whatever
// it already opened before returning.
func NewApp(ctx context.Context) (*App, error) {
	cfg, err := config.Load("config.yaml", ".env")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// App.Debug is a shortcut for "log everything" without editing config.
	if cfg.App.Debug {
		cfg.Logger.Level = "debug"
	}

	log, logGC, err := logger.New(cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}
	log = log.With("environment", cfg.App.Environment)
	slog.SetDefault(log)
	// A session cookie must never travel over plain HTTP once the public
	// address is HTTPS, whatever AUTH_SESSION_COOKIE_SECURE says.
	if strings.HasPrefix(strings.ToLower(cfg.App.PublicURL), "https://") && !cfg.Auth.SessionCookieSecure {
		cfg.Auth.SessionCookieSecure = true
		log.Warn("forcing Secure session cookies: app.public_url is https")
	}
	log.Info("starting",
		"name", cfg.App.Name, "version", cfg.App.Version, "public_url", cfg.App.PublicURL)

	fail := func(err error) (*App, error) {
		_ = logGC.Close()
		return nil, err
	}

	db := database.New(cfg.Database, log)
	if err := db.Connect(ctx); err != nil {
		return fail(fmt.Errorf("connect database: %w", err))
	}
	// Ensure the schema. app.Models() is ordered parent-first for SQLite
	// (inline FKs); Postgres adds FKs via ALTER after every table exists.
	if err := db.Migrate(app.Models()...); err != nil {
		_ = db.Close()
		return fail(fmt.Errorf("migrate schema: %w", err))
	}
	if err := db.RunMigrations(app.Migrations()...); err != nil {
		_ = db.Close()
		return fail(fmt.Errorf("run migrations: %w", err))
	}

	mail, err := mailer.New(cfg.Mailer, log)
	if err != nil {
		_ = db.Close()
		return fail(fmt.Errorf("init mailer: %w", err))
	}

	store, err := storage.New(cfg.Storage, log)
	if err != nil {
		_ = db.Close()
		return fail(fmt.Errorf("init storage: %w", err))
	}

	// Signs and verifies the public /p and /d share links. Shared by the
	// share handler and the worker's preview-ready notifier.
	signer, err := token.New(cfg.Auth.HMACSecret)
	if err != nil {
		_ = db.Close()
		return fail(fmt.Errorf("init share-token signer: %w", err))
	}

	// Domains: repo -> service -> handler.
	jobRepo := job.NewRepo(db)
	assetRepo := asset.NewRepo(db)

	userSvc := user.NewService(user.NewRepo(db), log)
	userH := user.NewHandler(userSvc, log)

	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), mail,
		cfg.Auth, cfg.App.PublicURL, cfg.App.FrontendURL, log,
	)
	authH := auth.NewHandler(authSvc, cfg.Auth, log)
	authRateLimiter := ratelimit.New(cfg.Auth.RequestRateLimit, cfg.Auth.RequestRateWindow.Std())
	// Same budget per recipient address as per client IP: rotating IPs
	// still cannot flood one mailbox with sign-in links.
	authSvc.SetEmailLimiter(ratelimit.New(cfg.Auth.RequestRateLimit, cfg.Auth.RequestRateWindow.Std()))
	userH.OnRoleChanged = authSvc.RevokeAll
	// Sign-in history (new-device alerts, the activity list) and passkeys.
	authSvc.UseLoginEvents(login_event.NewRepo(db))
	authSvc.UseAlertSigner(signer)
	authSvc.UsePasskeys(passkey.NewRepo(db), newWebAuthn(cfg, log))
	// Guesses at the 6-digit sign-in code, per client IP. Each email also
	// dies after a few wrong codes; this bounds guessing across emails.
	authCodeLimiter := ratelimit.New(cfg.Auth.RequestRateLimit*4, cfg.Auth.RequestRateWindow.Std())

	// The watermark engine also backs job.Service's upload validation (an
	// ffprobe pass confirming the stored file's actual stream kind matches
	// the extension it was accepted under) - both uses share one Engine, and
	// both degrade gracefully to nil when ffmpeg/ffprobe are missing.
	engine, engineErr := ffmpeg.New(cfg.FFmpeg, log)
	if engineErr != nil {
		log.Warn("watermark worker disabled", "reason", engineErr)
		engine = nil
	}

	jobSvc := job.NewService(jobRepo, assetRepo, store, engine, cfg.Upload, log)
	jobH := job.NewHandler(jobSvc, store, cfg.Upload, log)

	// Delivery: the job's links switch + history ("sent", "opened",
	// "paid"...), shared by the public share links, payment and the
	// operator's delivery routes. One notifier mails the recipient - the
	// worker when a preview is ready, the operator's "resend" otherwise.
	jobEvents := job_event.NewRepo(db)
	notifier := worker.NewEmailNotifier(mail, signer, cfg.App.PublicURL, cfg.Auth.ShareTokenTTL.Std())
	// The recipient's email names who sent the file (the operator's account
	// email) and the file itself, so it is not an anonymous "someone shared".
	userRepo := user.NewRepo(db)
	payments := cfg.Payment.StripeSecretKey != ""
	deliveryInfo := func(previewID string) (worker.DeliveryInfo, error) {
		prev, err := assetRepo.GetByID(previewID)
		if err != nil {
			return worker.DeliveryInfo{}, err
		}
		j, err := jobRepo.GetByID(prev.JobID)
		if err != nil {
			return worker.DeliveryInfo{}, err
		}
		info := worker.DeliveryInfo{MediaType: j.MediaType, Payments: payments}
		if u, err := userRepo.GetByID(j.UserID); err == nil {
			info.Sender = u.Email
		}
		if orig, err := assetRepo.GetByJobAndKind(j.ID, asset.KindOriginal); err == nil {
			info.FileName, info.SizeBytes = orig.Filename, orig.SizeBytes
		}
		return info, nil
	}
	notifier.UseDetails(deliveryInfo)

	// The operator's brand: frozen onto each upload and burned into the
	// watermark. Not shown in the email or on the recipient's page yet -
	// that waits for verified senders (TODOS.md).
	brandSvc := brand.NewService(brand.NewRepo(db), store, log)
	brandH := brand.NewHandler(brandSvc, log)
	if b, ok := jobSvc.(job.BrandAware); ok {
		b.UseBrands(brandSvc)
	}

	shareH := share.NewHandler(signer, assetRepo, store, log)
	shareH.UseDelivery(jobRepo, jobEvents)
	// The recipient's page names the file and its sender, as the email does.
	shareH.UseDetails(func(previewID string) (share.Info, error) {
		d, err := deliveryInfo(previewID)
		return share.Info{FileName: d.FileName, MediaType: d.MediaType, SizeBytes: d.SizeBytes, Sender: d.Sender}, err
	})
	deliveryH := share.NewOperatorHandler(jobSvc, jobRepo, jobEvents, notifier,
		signer, cfg.App.PublicURL, cfg.Auth.ShareTokenTTL.Std(), log)
	if cfg.Payment.StripeSecretKey != "" {
		deliveryH.SetPayment(cfg.Payment.PriceCents, cfg.Payment.Currency)
	}

	// The Stripe-backed original-file unlock. A missing key only disables
	// this domain (Service.CreateIntent/Status report it, rather than the
	// app failing to start) - mirrors ffmpeg's optionality below.
	var stripeClient *stripe.Client
	if cfg.Payment.StripeSecretKey != "" {
		stripeClient = stripe.NewClient(cfg.Payment.StripeSecretKey)
	} else {
		log.Warn("payment unlock disabled", "reason", "no STRIPE_SECRET_KEY configured")
	}
	paymentSvc := payment.NewService(
		payment.NewRepo(db), assetRepo, signer, stripeClient, cfg.Payment,
		cfg.App.PublicURL, cfg.Payment.OriginalLinkTTL.Std(), log,
	)
	if d, ok := paymentSvc.(payment.Deliverer); ok {
		d.UseDelivery(jobRepo, jobEvents)
	}
	paymentH := payment.NewHandler(paymentSvc, log)

	router := app.NewRouter(app.Deps{
		Cfg: cfg, Log: log, Auth: authH, AuthSvc: authSvc,
		User: userH, Job: jobH, Share: shareH, Delivery: deliveryH, Payment: paymentH, Brand: brandH,
		AuthRateLimiter:    authRateLimiter,
		AuthCodeLimiter:    authCodeLimiter,
		PaymentRateLimiter: ratelimit.New(cfg.Payment.RateLimit, cfg.Payment.RateWindow.Std()),
		Signer:             signer,
	})

	a := &App{cfg: cfg, log: log, logGC: logGC, db: db, auth: authSvc}
	a.server = server.New(cfg.Server, log, router)

	// The watermark worker. If ffmpeg/ffprobe are missing (engine == nil,
	// above) the app still runs (uploads queue as pending); the pool just
	// does not start.
	if engine != nil {
		a.worker = worker.New(
			worker.Config{
				Concurrency:     cfg.Worker.Concurrency,
				PollInterval:    cfg.Worker.PollInterval.Std(),
				ClaimBatchSize:  cfg.Worker.ClaimBatchSize,
				MaxRetries:      cfg.Worker.MaxRetries,
				RetryBackoff:    cfg.Worker.RetryBackoff.Std(),
				JobTimeout:      cfg.Worker.JobTimeout.Std(),
				StuckJobTimeout: cfg.Worker.StuckJobTimeout.Std(),
				ShutdownGrace:   cfg.Worker.ShutdownGrace.Std(),
				TextTemplate:    cfg.FFmpeg.TextTemplate,
			},
			jobStoreAdapter{jobRepo},
			assetStoreAdapter{assetRepo},
			store,
			engine,
			recordingNotifier{inner: notifier, assets: assetRepo, events: jobEvents, log: log},
			log,
		)
	}
	return a, nil
}

// Run starts the watermark worker (if enabled) and serves until ctx is
// cancelled (SIGINT/SIGTERM) or the server stops on its own, then shuts the
// server and the worker down gracefully. Close still has to be called
// afterwards to release the database and logger.
func (a *App) Run(ctx context.Context) error {
	go a.purgeExpired(ctx)
	if a.worker != nil {
		a.worker.Start(ctx)
	}
	srvErr := server.Run(ctx, a.server)
	if a.worker != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), a.cfg.Worker.ShutdownGrace.Std())
		defer cancel()
		if err := a.worker.Stop(stopCtx); err != nil {
			a.log.Error("worker did not stop cleanly", "error", err)
		}
	}
	return srvErr
}

// Close releases resources in reverse order of construction: database, then
// the log file. Safe to call once, after Run returns.
func (a *App) Close() error {
	var errs []error
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close database: %w", err))
		}
	}
	if a.logGC != nil {
		if err := a.logGC.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close logger: %w", err))
		}
	}
	return errors.Join(errs...)
}

// purgeExpired drops expired sign-in tokens and sessions once at startup and
// then hourly, until ctx is cancelled.
func (a *App) purgeExpired(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := a.auth.PurgeExpired(ctx); err != nil {
			a.log.Error("purge expired credentials", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// newWebAuthn builds the passkey verifier for the frontend's origin, or nil
// (passkeys off) when that origin cannot host passkeys - WebAuthn refuses a
// bare IP address as the relying party, e.g. a LAN demo on 192.168.x.x.
// Passkeys are bound to the hostname: ones created on one tunnel URL do not
// work on the next.
func newWebAuthn(cfg *config.Config, log *slog.Logger) *webauthn.WebAuthn {
	u, err := url.Parse(cfg.App.FrontendURL)
	if err != nil || u.Hostname() == "" {
		log.Warn("passkeys disabled", "reason", "APP_FRONTEND_URL has no host")
		return nil
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		log.Warn("passkeys disabled", "reason", "frontend runs on a bare IP address", "host", host)
		return nil
	}
	origins := []string{}
	seen := map[string]bool{}
	for _, o := range append([]string{cfg.App.FrontendURL, cfg.App.PublicURL}, cfg.Server.CORSAllowedOrigins...) {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		ou, err := url.Parse(o)
		if o == "" || seen[o] || err != nil || ou.Hostname() != host {
			continue
		}
		seen[o] = true
		origins = append(origins, o)
	}
	wa, err := webauthn.New(&webauthn.Config{RPID: host, RPDisplayName: "SADE", RPOrigins: origins})
	if err != nil {
		log.Warn("passkeys disabled", "reason", err)
		return nil
	}
	return wa
}
