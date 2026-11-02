package app

import "sade/internals/database"

// Migrations is every schema change schema-builder cannot make itself (it
// only creates missing tables). Run by app.go right after Models() are
// ensured, in this order, each exactly once (tracked in schema_migrations).
// Append only - never edit or reorder an entry that has shipped.
func Migrations() []database.Migration {
	return []database.Migration{
		{ID: "2026-11-03-session-device", Apply: func(db *database.Database) error {
			return addColumns(db, "sessions", map[string]database.ColumnKind{
				"user_agent":   database.KindString,
				"ip":           database.KindString,
				"last_seen_at": database.KindTime,
			})
		}},
		{ID: "2026-11-03-magic-token-browser-code", Apply: func(db *database.Database) error {
			return addColumns(db, "magic_tokens", map[string]database.ColumnKind{
				"browser_hash": database.KindString,
				"code_hash":    database.KindString,
				"attempts":     database.KindInt,
			})
		}},
		{ID: "2026-11-03-user-require-passkey", Apply: func(db *database.Database) error {
			return db.AddColumn("users", "require_passkey", database.KindInt)
		}},
		{ID: "2026-11-03-job-links-disabled", Apply: func(db *database.Database) error {
			return db.AddColumn("jobs", "links_disabled", database.KindInt)
		}},
	}
}

func addColumns(db *database.Database, table string, cols map[string]database.ColumnKind) error {
	for name, kind := range cols {
		if err := db.AddColumn(table, name, kind); err != nil {
			return err
		}
	}
	return nil
}
