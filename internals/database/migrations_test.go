package database

import (
	"testing"
	"time"
)

type gadgetV1 struct {
	ID   string `db:"id,primary_key,uuid"`
	Name string `db:"name,notnull"`
}

func (gadgetV1) TableName() string { return "gadgets" }

// gadgetV2 is the same table after a migration added four columns.
type gadgetV2 struct {
	ID       string    `db:"id,primary_key,uuid"`
	Name     string    `db:"name,notnull"`
	Label    string    `db:"label"`
	Count    int       `db:"count"`
	Enabled  int       `db:"enabled"`
	LastSeen time.Time `db:"last_seen"`
}

func (gadgetV2) TableName() string { return "gadgets" }

func TestRunMigrationsAddsColumnsReadableByCRUD(t *testing.T) {
	db := connected(t)
	if err := db.Migrate(gadgetV1{}); err != nil {
		t.Fatal(err)
	}
	id, err := db.CRUD().Create(&gadgetV1{Name: "old row"})
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	m := Migration{ID: "test-gadgets-v2", Apply: func(db *Database) error {
		calls++
		for col, kind := range map[string]ColumnKind{
			"label": KindString, "count": KindInt, "enabled": KindInt, "last_seen": KindTime,
		} {
			if err := db.AddColumn("gadgets", col, kind); err != nil {
				return err
			}
		}
		return nil
	}}
	if err := db.RunMigrations(m); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	// A second run (next startup) must skip it.
	if err := db.RunMigrations(m); err != nil {
		t.Fatalf("RunMigrations again: %v", err)
	}
	if calls != 1 {
		t.Fatalf("migration applied %d times, want 1", calls)
	}

	// The pre-existing row must scan into the new struct with zero values.
	var g gadgetV2
	if err := db.CRUD().ReadOne(gadgetV2{}, "id", id, &g); err != nil {
		t.Fatalf("read old row through new struct: %v", err)
	}
	if g.Name != "old row" || g.Label != "" || g.Count != 0 || g.Enabled != 0 || !g.LastSeen.IsZero() {
		t.Errorf("old row = %+v, want zero values in new columns", g)
	}

	// And new rows round-trip the new columns.
	now := time.Now().UTC().Truncate(time.Second)
	id2, err := db.CRUD().Create(&gadgetV2{Name: "new", Label: "x", Count: 3, Enabled: 1, LastSeen: now})
	if err != nil {
		t.Fatal(err)
	}
	var g2 gadgetV2
	if err := db.CRUD().ReadOne(gadgetV2{}, "id", id2, &g2); err != nil {
		t.Fatal(err)
	}
	if g2.Label != "x" || g2.Count != 3 || g2.Enabled != 1 || !g2.LastSeen.Equal(now) {
		t.Errorf("new row = %+v", g2)
	}
}

func TestAddColumnIsNoOpWhenPresent(t *testing.T) {
	db := connected(t)
	if err := db.Migrate(gadgetV2{}); err != nil {
		t.Fatal(err)
	}
	// Fresh database: the table already has the column from the struct.
	if err := db.AddColumn("gadgets", "label", KindString); err != nil {
		t.Fatalf("AddColumn on existing column: %v", err)
	}
}

func TestRunMigrationsRejectsDuplicateIDs(t *testing.T) {
	db := connected(t)
	noop := func(*Database) error { return nil }
	if err := db.RunMigrations(Migration{ID: "a", Apply: noop}, Migration{ID: "a", Apply: noop}); err == nil {
		t.Fatal("want an error for a duplicated id")
	}
}
