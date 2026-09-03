// Package testutil holds helpers shared across the project's tests: a
// throwaway database and a silent logger. It is only ever imported from
// _test.go files.
package testutil

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"sade/config"
	"sade/internals/database"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for tests
)

// Logger returns a *slog.Logger that discards everything.
func Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// DB returns a connected in-memory SQLite database migrated for models, and
// registers t.Cleanup to close it. Each call is an isolated database.
func DB(t *testing.T, models ...any) *database.Database {
	t.Helper()
	db := database.New(config.DatabaseConfig{Driver: "sqlite", Name: ":memory:"}, Logger())
	if err := db.Connect(context.Background()); err != nil {
		t.Fatalf("testutil.DB: connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if len(models) > 0 {
		if err := db.Migrate(models...); err != nil {
			t.Fatalf("testutil.DB: migrate: %v", err)
		}
	}
	return db
}
