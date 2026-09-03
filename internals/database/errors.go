package database

import "errors"

// ErrNotConnected is returned by the query methods when Connect has not run
// (or failed). crud-depot's own crud.ErrNotFound is what repositories map to
// their per-package "not found" errors - that one does not live here.
var ErrNotConnected = errors.New("database: not connected (call Connect first)")
