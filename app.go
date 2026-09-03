package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"sade/config"
	"sade/internals/app"
	"sade/internals/app/auth"
	"sade/internals/app/magic_token"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/logger"
	"sade/internals/mailer"
	"sade/internals/server"
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

	// Domains: repo -> service -> handler.
	userSvc := user.NewService(user.NewRepo(db), log)
	userH := user.NewHandler(userSvc, log)

	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), mail,
		cfg.Auth, cfg.App.PublicURL, cfg.App.FrontendURL, log,
	)
	authH := auth.NewHandler(authSvc, cfg.Auth, log)

	router := app.NewRouter(app.Deps{
		Cfg: cfg, Log: log, Auth: authH, AuthSvc: authSvc, User: userH,
	})

	a := &App{cfg: cfg, log: log, logGC: logGC, db: db}
	a.server = server.New(cfg.Server, log, router)
	return a, nil
}

// Run serves until ctx is cancelled (SIGINT/SIGTERM) or the server stops on
// its own, then shuts the server down gracefully. Close still has to be
// called afterwards to release the database and logger.
func (a *App) Run(ctx context.Context) error {
	return server.Run(ctx, a.server)
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
