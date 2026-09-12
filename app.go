package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"sade/config"
	"sade/internals/app"
	"sade/internals/app/asset"
	"sade/internals/app/auth"
	"sade/internals/app/job"
	"sade/internals/app/magic_token"
	"sade/internals/app/payment"
	"sade/internals/app/session"
	"sade/internals/app/share"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/ffmpeg"
	"sade/internals/logger"
	"sade/internals/mailer"
	"sade/internals/server"
	"sade/internals/storage"
	"sade/internals/token"
	"sade/internals/worker"

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

	jobSvc := job.NewService(jobRepo, assetRepo, store, cfg.Upload, log)
	jobH := job.NewHandler(jobSvc, store, cfg.Upload, log)

	shareH := share.NewHandler(signer, assetRepo, store, log)

	// The Stripe-backed original-file unlock. A missing key only disables
	// this domain (Service.Checkout/Status report it, rather than the app
	// failing to start) - mirrors ffmpeg's optionality below.
	var stripeClient *stripe.Client
	if cfg.Payment.StripeSecretKey != "" {
		stripeClient = stripe.NewClient(cfg.Payment.StripeSecretKey)
	} else {
		log.Warn("payment unlock disabled", "reason", "no STRIPE_SECRET_KEY configured")
	}
	paymentSvc := payment.NewService(
		payment.NewRepo(db), assetRepo, signer, stripeClient, cfg.Payment,
		cfg.App.PublicURL, cfg.App.FrontendURL, cfg.Auth.ShareTokenTTL.Std(), log,
	)
	paymentH := payment.NewHandler(paymentSvc, log)

	router := app.NewRouter(app.Deps{
		Cfg: cfg, Log: log, Auth: authH, AuthSvc: authSvc,
		User: userH, Job: jobH, Share: shareH, Payment: paymentH,
	})

	a := &App{cfg: cfg, log: log, logGC: logGC, db: db}
	a.server = server.New(cfg.Server, log, router)

	// The watermark worker. If ffmpeg/ffprobe are missing the app still runs
	// (uploads queue as pending); the pool just does not start.
	if engine, eErr := ffmpeg.New(cfg.FFmpeg, log); eErr != nil {
		log.Warn("watermark worker disabled", "reason", eErr)
	} else {
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
			worker.NewEmailNotifier(mail, signer, cfg.App.PublicURL, cfg.Auth.ShareTokenTTL.Std()),
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
