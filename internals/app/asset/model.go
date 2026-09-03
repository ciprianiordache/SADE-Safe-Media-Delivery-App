// Package asset is a stored file belonging to a job: the uploaded original or
// a watermarked preview. Only metadata lives in the database; the bytes are
// held by internals/storage under StorageKey. Assets are created by the
// upload handler (original) and the worker (preview), never by a client
// directly, so there is no create/update request DTO.
package asset

import "time"

// Kind of file.
const (
	KindOriginal = "original"
	KindPreview  = "preview"
)

// Asset is one file on disk (or in object storage) plus its metadata.
type Asset struct {
	ID         string    `db:"id,primary_key,uuid"`
	JobID      string    `db:"job_id,notnull,index,references:jobs(id),on_delete:cascade"`
	Kind       string    `db:"kind,notnull"`
	StorageKey string    `db:"storage_key,notnull"` // key within internals/storage
	Filename   string    `db:"filename,notnull"`    // original client filename
	MIME       string    `db:"mime,notnull"`
	SizeBytes  int64     `db:"size_bytes,notnull"`
	Checksum   string    `db:"checksum"` // sha256 hex, "" until computed
	CreatedAt  time.Time `db:"created_at,oncreate"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Asset) TableName() string { return "assets" }

// Response is the API view of an asset (metadata only; the bytes are served
// through signed share links, not this struct).
type Response struct {
	ID        string    `json:"id"`
	JobID     string    `json:"jobId"`
	Kind      string    `json:"kind"`
	Filename  string    `json:"filename"`
	MIME      string    `json:"mime"`
	SizeBytes int64     `json:"sizeBytes"`
	Checksum  string    `json:"checksum,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func toResponse(a Asset) Response {
	return Response{
		ID:        a.ID,
		JobID:     a.JobID,
		Kind:      a.Kind,
		Filename:  a.Filename,
		MIME:      a.MIME,
		SizeBytes: a.SizeBytes,
		Checksum:  a.Checksum,
		CreatedAt: a.CreatedAt,
	}
}
