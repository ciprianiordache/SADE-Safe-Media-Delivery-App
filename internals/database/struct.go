package database

import (
	"database/sql"
	"log/slog"

	"sade/config"

	crud "github.com/ciprianiordache/crud-depot"
)

// Database owns the *sql.DB connection pool and hands out the pieces the rest
// of the app builds on: a shared *crud.CRUD for repositories, a Migrate helper
// backed by schema-builder, and the plain Exec/Query/QueryRow/Begin methods
// those libraries (and any hand-written SQL) need. The concrete SQL dialect
// (Postgres in production, SQLite in tests) is decided here, once, from the
// driver name - callers never pick a dialect themselves.
type Database struct {
	cfg  config.DatabaseConfig
	log  *slog.Logger
	conn *sql.DB
	crud *crud.CRUD
}
