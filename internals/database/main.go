// Package database wraps a database/sql connection pool for SADE. It builds
// the DSN and pool from config.DatabaseConfig, verifies reachability with a
// bounded ping, and exposes a shared *crud.CRUD plus a schema-builder-backed
// Migrate helper. Production runs on PostgreSQL via the pgx stdlib driver;
// tests run on SQLite. The dialect is chosen here from the driver name so no
// other package has to.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"sade/config"

	"github.com/ciprianiordache/crud-depot"
	"github.com/ciprianiordache/schema-builder"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// pingTimeout bounds the reachability check in Connect so a wrong host fails
// in seconds instead of hanging on the OS TCP timeout.
const pingTimeout = 5 * time.Second

// openDB is a seam for tests to stub out sql.Open.
var openDB = sql.Open

// New builds a Database for cfg. It does not connect - call Connect.
func New(cfg config.DatabaseConfig, log *slog.Logger) *Database {
	return &Database{cfg: cfg, log: log}
}

// Connect opens the pool, applies the pool limits, and pings the database
// within pingTimeout. On success it also builds the shared *crud.CRUD.
func (db *Database) Connect(ctx context.Context) error {
	dsn, err := db.buildDSN()
	if err != nil {
		return err
	}

	conn, err := openDB(db.cfg.Driver, dsn)
	if err != nil {
		return fmt.Errorf("database: open %s: %w", db.cfg.Driver, err)
	}

	if db.isSQLite() {
		// One connection keeps :memory: and the foreign_keys pragma alive.
		conn.SetMaxOpenConns(1)
		conn.SetMaxIdleConns(1)
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			_ = conn.Close()
			return fmt.Errorf("database: enable sqlite foreign keys: %w", err)
		}
	} else {
		conn.SetMaxOpenConns(db.cfg.MaxOpenConns)
		conn.SetMaxIdleConns(db.cfg.MaxIdleConns)
	}
	conn.SetConnMaxLifetime(db.cfg.ConnMaxLifetime.Std())
	conn.SetConnMaxIdleTime(db.cfg.ConnMaxIdleTime.Std())

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return fmt.Errorf("database: ping %s at %s:%d: %w", db.cfg.Driver, db.cfg.Host, db.cfg.Port, err)
	}

	db.conn = conn
	db.crud = crud.New(db, db.crudDialect())
	db.log.Info("database connected",
		"driver", db.cfg.Driver, "host", db.cfg.Host, "port", db.cfg.Port, "name", db.cfg.Name)
	return nil
}

// buildDSN renders the driver-specific connection string. Postgres is built
// through net/url so passwords with spaces or symbols are escaped correctly.
func (db *Database) buildDSN() (string, error) {
	switch db.cfg.Driver {
	case "postgres", "pgx":
		u := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(db.cfg.User, db.cfg.Password),
			Host:   net.JoinHostPort(db.cfg.Host, strconv.Itoa(db.cfg.Port)),
			Path:   db.cfg.Name,
		}
		q := u.Query()
		if db.cfg.SSLMode != "" {
			q.Set("sslmode", db.cfg.SSLMode)
		}
		if db.cfg.Timezone != "" {
			q.Set("timezone", db.cfg.Timezone)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil

	case "sqlite", "sqlite3":
		if db.cfg.Name == ":memory:" {
			return "file::memory:?cache=shared", nil
		}
		return db.cfg.Name + ".db", nil

	default:
		return "", fmt.Errorf("database: unsupported driver %q (want pgx, postgres, or sqlite)", db.cfg.Driver)
	}
}

// Migrate ensures every model's table, indexes and foreign keys exist. It is
// idempotent (CREATE TABLE IF NOT EXISTS); pass models parent-last is not
// required for Postgres (FKs are added by ALTER after all tables exist) but
// keep dependency order for SQLite, where FKs are inlined.
func (db *Database) Migrate(models ...any) error {
	if db.conn == nil {
		return ErrNotConnected
	}
	if err := schema.New(db, db.schemaDialect()).CreateSchema(models...); err != nil {
		return fmt.Errorf("database: migrate: %w", err)
	}
	db.log.Info("schema ensured", "models", len(models))
	return nil
}

// CRUD returns the shared crud-depot instance, configured with the right
// dialect. Repositories embed this instead of calling crud.New themselves.
func (db *Database) CRUD() *crud.CRUD { return db.crud }

// SQL exposes the underlying pool for the rare caller that needs it directly
// (migrations tooling, LISTEN/NOTIFY, tests).
func (db *Database) SQL() *sql.DB { return db.conn }

// Exec runs a statement that returns no rows.
func (db *Database) Exec(query string, args ...any) (sql.Result, error) {
	if db.conn == nil {
		return nil, ErrNotConnected
	}
	return db.conn.Exec(query, args...)
}

// Query runs a statement that returns rows.
func (db *Database) Query(query string, args ...any) (*sql.Rows, error) {
	if db.conn == nil {
		return nil, ErrNotConnected
	}
	return db.conn.Query(query, args...)
}

// QueryRow runs a statement expected to return at most one row. It mirrors
// *sql.DB.QueryRow, so a nil connection surfaces as an error on Scan.
func (db *Database) QueryRow(query string, args ...any) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Begin starts a transaction. It lets *Database satisfy crud-depot's
// TxBeginner, so db.CRUD().RunInTx(db, ...) works.
func (db *Database) Begin() (*sql.Tx, error) {
	if db.conn == nil {
		return nil, ErrNotConnected
	}
	return db.conn.Begin()
}

// Close closes the pool.
func (db *Database) Close() error {
	if db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

// Driver is the configured driver name.
func (db *Database) Driver() string { return db.cfg.Driver }

func (db *Database) isSQLite() bool {
	return db.cfg.Driver == "sqlite" || db.cfg.Driver == "sqlite3"
}

func (db *Database) crudDialect() crud.Dialect {
	if db.isSQLite() {
		return crud.SQLite{}
	}
	return crud.Postgres{}
}

func (db *Database) schemaDialect() schema.Dialect {
	if db.isSQLite() {
		return schema.SQLite{}
	}
	return schema.Postgres{}
}
