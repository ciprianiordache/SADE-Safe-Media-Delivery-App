package database

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"sade/config"

	crud "github.com/ciprianiordache/crud-depot"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for tests
)

type widget struct {
	ID    string `db:"id,primary_key,uuid"`
	Name  string `db:"name,notnull"`
	Count int    `db:"count,default:1"`
}

func (widget) TableName() string { return "widgets" }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() config.DatabaseConfig {
	return config.DatabaseConfig{
		Driver:          "sqlite",
		Name:            ":memory:",
		ConnMaxLifetime: config.Duration(time.Hour),
		ConnMaxIdleTime: config.Duration(30 * time.Minute),
	}
}

// connected returns a Database whose Connect has already succeeded.
func connected(t *testing.T) *Database {
	t.Helper()
	db := New(testConfig(), testLogger())
	if err := db.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestConnectAndDriver(t *testing.T) {
	db := connected(t)
	if db.Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", db.Driver())
	}
	if db.SQL() == nil {
		t.Error("SQL() is nil after Connect")
	}
	if db.CRUD() == nil {
		t.Error("CRUD() is nil after Connect")
	}
}

func TestConnectUnsupportedDriver(t *testing.T) {
	db := New(config.DatabaseConfig{Driver: "oracle", Name: "x"}, testLogger())
	if err := db.Connect(context.Background()); err == nil {
		t.Fatal("expected Connect to reject an unsupported driver")
	}
}

func TestQueryMethodsBeforeConnect(t *testing.T) {
	db := New(testConfig(), testLogger())
	if _, err := db.Exec("SELECT 1"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Exec err = %v, want ErrNotConnected", err)
	}
	if _, err := db.Query("SELECT 1"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Query err = %v, want ErrNotConnected", err)
	}
	if _, err := db.Begin(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Begin err = %v, want ErrNotConnected", err)
	}
	if err := db.Migrate(widget{}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Migrate err = %v, want ErrNotConnected", err)
	}
}

func TestMigrateAndCRUD(t *testing.T) {
	db := connected(t)
	if err := db.Migrate(widget{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Idempotent.
	if err := db.Migrate(widget{}); err != nil {
		t.Fatalf("Migrate (second run): %v", err)
	}

	c := db.CRUD()
	id, err := c.Create(&widget{Name: "gizmo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned an empty id for a uuid primary key")
	}

	var got widget
	if err := c.ReadOne(widget{}, "id", id, &got); err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.Name != "gizmo" || got.Count != 1 {
		t.Errorf("got %+v, want name=gizmo count=1 (default)", got)
	}

	if err := c.ReadOne(widget{}, "id", "missing", &got); !errors.Is(err, crud.ErrNotFound) {
		t.Errorf("ReadOne(missing) err = %v, want crud.ErrNotFound", err)
	}
}

func TestRunInTxRollback(t *testing.T) {
	db := connected(t)
	if err := db.Migrate(widget{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	sentinel := errors.New("boom")
	err := db.CRUD().RunInTx(db, func(tx *crud.CRUD) error {
		if _, err := tx.Create(&widget{Name: "temp"}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunInTx err = %v, want sentinel", err)
	}

	var rows []widget
	err = db.CRUD().Read(widget{}, "name", "temp", &rows)
	if err != nil && !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("Read: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rollback failed: %d rows survived", len(rows))
	}
}

func TestPostgresDSN(t *testing.T) {
	db := New(config.DatabaseConfig{
		Driver: "pgx", Host: "db.internal", Port: 5432,
		User: "sade", Password: "p@ss word/!", Name: "sade", SSLMode: "require", Timezone: "UTC",
	}, testLogger())

	dsn, err := db.buildDSN()
	if err != nil {
		t.Fatalf("buildDSN: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("generated DSN does not parse: %q: %v", dsn, err)
	}
	if u.Scheme != "postgres" || u.Host != "db.internal:5432" || u.Path != "/sade" {
		t.Errorf("unexpected DSN parts: scheme=%q host=%q path=%q", u.Scheme, u.Host, u.Path)
	}
	if pw, _ := u.User.Password(); pw != "p@ss word/!" {
		t.Errorf("password not round-tripped: %q", pw)
	}
	if u.Query().Get("sslmode") != "require" {
		t.Errorf("sslmode = %q, want require", u.Query().Get("sslmode"))
	}
}
