// Package job_event is a job's delivery history: emailed, opened, downloaded,
// paid, links switched off and on. The operator sees it as a timeline on the
// job page. Writers are the worker's notifier (sent), the public share
// handlers (opened / downloaded), payment (paid) and the operator's own
// actions (resent, links disabled/enabled). Internal - no DTOs.
package job_event

import "time"

// Event kinds.
const (
	KindSent               = "sent"
	KindResent             = "resent"
	KindViewed             = "viewed"
	KindDownloaded         = "downloaded"
	KindPaid               = "paid"
	KindOriginalDownloaded = "original_downloaded"
	KindLinksDisabled      = "links_disabled"
	KindLinksEnabled       = "links_enabled"
)

// JobEvent is one thing that happened to a job's delivery.
type JobEvent struct {
	ID     string `db:"id,primary_key,uuid"`
	JobID  string `db:"job_id,notnull,index,references:jobs(id),on_delete:cascade"`
	Kind   string `db:"kind,notnull"`
	Detail string `db:"detail"` // e.g. the address an email went to
	// CreatedAt is set explicitly (UTC) rather than via oncreate, so the
	// "at most once per window" check compares like with like.
	CreatedAt time.Time `db:"created_at,notnull,index"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (JobEvent) TableName() string { return "job_events" }
