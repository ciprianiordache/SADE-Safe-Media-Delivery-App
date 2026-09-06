// Package job is the watermarking unit of work: one uploaded media file, its
// watermark settings, and the processing lifecycle. The database row is the
// source of truth for job state - the worker owns the status transitions.
// A job's files are rows in the asset domain (asset.JobID), not columns here.
package job

import (
	"time"

	"sade/internals/app/asset"
)

// Status lifecycle: pending -> processing -> done | failed.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusFailed     = "failed"
)

// MediaType of the source file.
const (
	MediaVideo = "video"
	MediaAudio = "audio"
	MediaImage = "image"
)

// WatermarkKind selects what the engine overlays.
const (
	WatermarkLogo = "logo"
	WatermarkText = "text"
	WatermarkBoth = "both"
)

// Job is one watermarking request.
type Job struct {
	ID             string    `db:"id,primary_key,uuid"`
	UserID         string    `db:"user_id,notnull,index,references:users(id),on_delete:cascade"`
	Status         string    `db:"status,notnull,index,default:pending"`
	MediaType      string    `db:"media_type,notnull"`
	RecipientEmail string    `db:"recipient_email,notnull"`
	WatermarkKind  string    `db:"watermark_kind,notnull,default:both"`
	WatermarkText  string    `db:"watermark_text"` // "" = render from the configured template
	WatermarkOpts  string    `db:"watermark_opts"` // JSON overriding ffmpeg defaults; "" = defaults
	Attempts       int       `db:"attempts,notnull,default:0"`
	Error          string    `db:"error"` // last failure message; "" = none
	CreatedAt      time.Time `db:"created_at,oncreate"`
	UpdatedAt      time.Time `db:"updated_at,onwrite"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Job) TableName() string { return "jobs" }

// CreateRequest is the non-file part of a POST /api/jobs multipart request;
// the media file travels as a separate form part and its type is detected
// server-side, not taken from the client.
type CreateRequest struct {
	RecipientEmail string `json:"recipientEmail"`
	WatermarkKind  string `json:"watermarkKind"`
	WatermarkText  string `json:"watermarkText"`
	WatermarkOpts  string `json:"watermarkOpts"`
}

// Response is the API view of a job. Status transitions are internal (the
// worker owns them), so there is no update request DTO. Assets is populated
// only by the single-job detail endpoint; the list endpoint leaves it nil.
type Response struct {
	ID             string           `json:"id"`
	Status         string           `json:"status"`
	MediaType      string           `json:"mediaType"`
	RecipientEmail string           `json:"recipientEmail"`
	WatermarkKind  string           `json:"watermarkKind"`
	WatermarkText  string           `json:"watermarkText,omitempty"`
	Attempts       int              `json:"attempts"`
	Error          string           `json:"error,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
	Assets         []asset.Response `json:"assets,omitempty"`
}

func toResponse(j Job) Response {
	return Response{
		ID:             j.ID,
		Status:         j.Status,
		MediaType:      j.MediaType,
		RecipientEmail: j.RecipientEmail,
		WatermarkKind:  j.WatermarkKind,
		WatermarkText:  j.WatermarkText,
		Attempts:       j.Attempts,
		Error:          j.Error,
		CreatedAt:      j.CreatedAt,
		UpdatedAt:      j.UpdatedAt,
	}
}
