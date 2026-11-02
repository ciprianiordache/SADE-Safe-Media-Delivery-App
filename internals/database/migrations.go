package database

import (
	"fmt"
	"time"
)

// Migration is one forward-only schema change applied after Migrate has
// ensured every model's table exists. schema-builder only creates missing
// tables, so anything it cannot express - a column added to a table that
// already has rows - goes here.
//
// Apply must be idempotent: on a fresh database the table was just created
// from the current struct, so the column it adds may already be there
// (AddColumn checks for that).
type Migration struct {
	ID    string // unique and stable, e.g. "2026-11-03-sessions-device"
	Apply func(db *Database) error
}

// ColumnKind is the Go-side type of a column added by a migration. It maps to
// the same SQL type schema-builder would pick for that Go type, so a column
// added later is indistinguishable from one created with the table.
type ColumnKind string

const (
	KindString ColumnKind = "string"
	KindInt    ColumnKind = "int"
	KindTime   ColumnKind = "time"
)

// There is deliberately no bool kind: SQLite stores a bool as INTEGER and
// crud-depot cannot convert the int64 it reads back into a Go bool. Model a
// flag as an int (0/1) instead.

// RunMigrations applies, in order, every migration not yet recorded in
// schema_migrations. Each one runs once; its ID is recorded after it
// succeeds, so a failed migration is retried on the next start.
func (db *Database) RunMigrations(migrations ...Migration) error {
	if db.conn == nil {
		return ErrNotConnected
	}
	if _, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		id TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("database: create schema_migrations: %w", err)
	}

	seen := map[string]bool{}
	for _, m := range migrations {
		if m.ID == "" || seen[m.ID] {
			return fmt.Errorf("database: migration id %q is empty or duplicated", m.ID)
		}
		seen[m.ID] = true

		var n int
		if err := db.conn.QueryRow(db.Rebind(`SELECT COUNT(*) FROM schema_migrations WHERE id = ?`), m.ID).Scan(&n); err != nil {
			return fmt.Errorf("database: check migration %s: %w", m.ID, err)
		}
		if n > 0 {
			continue
		}
		if err := m.Apply(db); err != nil {
			return fmt.Errorf("database: migration %s: %w", m.ID, err)
		}
		if _, err := db.conn.Exec(db.Rebind(`INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)`),
			m.ID, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("database: record migration %s: %w", m.ID, err)
		}
		db.log.Info("migration applied", "id", m.ID)
	}
	return nil
}

// AddColumn adds column to table unless it already exists. The column is
// NOT NULL with the kind's zero value as default, because crud-depot scans
// into plain Go zero values and cannot read a NULL back into a string or a
// time.Time - existing rows get "", 0 or the zero time.
func (db *Database) AddColumn(table, column string, kind ColumnKind) error {
	exists, err := db.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	def, err := db.columnDef(kind)
	if err != nil {
		return err
	}
	if _, err := db.conn.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def)); err != nil {
		return fmt.Errorf("database: add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (db *Database) columnDef(kind ColumnKind) (string, error) {
	sqlite := db.isSQLite()
	switch kind {
	case KindString:
		return "TEXT NOT NULL DEFAULT ''", nil
	case KindInt:
		return "INTEGER NOT NULL DEFAULT 0", nil
	case KindTime:
		if sqlite {
			return "DATETIME NOT NULL DEFAULT '0001-01-01 00:00:00+00:00'", nil
		}
		return "TIMESTAMP NOT NULL DEFAULT '0001-01-01 00:00:00'", nil
	default:
		return "", fmt.Errorf("database: unknown column kind %q", kind)
	}
}

func (db *Database) columnExists(table, column string) (bool, error) {
	var n int
	var err error
	if db.isSQLite() {
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	} else {
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, table, column).Scan(&n)
	}
	if err != nil {
		return false, fmt.Errorf("database: inspect %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}
