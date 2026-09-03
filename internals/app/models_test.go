package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"sade/config"
	"sade/internals/database"

	schema "github.com/ciprianiordache/schema-builder"
	_ "modernc.org/sqlite"
)

// TestModelsValidate runs schema-builder's own validation over every model:
// each must have a uuid/auto primary key and well-formed references.
func TestModelsValidate(t *testing.T) {
	if err := schema.ValidateModels(Models()...); err != nil {
		t.Fatalf("ValidateModels: %v", err)
	}
}

// TestModelsMigrate ensures the whole schema builds against a real engine
// (SQLite here), which exercises the generated DDL - column types, foreign
// keys and indexes - not just the tag parser.
func TestModelsMigrate(t *testing.T) {
	db := database.New(config.DatabaseConfig{Driver: "sqlite", Name: ":memory:"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := db.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(Models()...); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Idempotent.
	if err := db.Migrate(Models()...); err != nil {
		t.Fatalf("Migrate (second run): %v", err)
	}

	// Every model's table must now exist and be queryable.
	for _, m := range Models() {
		table := m.(interface{ TableName() string }).TableName()
		if _, err := db.Exec("SELECT * FROM " + table + " LIMIT 0"); err != nil {
			t.Errorf("table %q not usable after Migrate: %v", table, err)
		}
	}
}
