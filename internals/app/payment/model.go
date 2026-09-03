// Package payment is the (backlog) record of a transaction that unlocks a
// job's original file. It is not wired into the app yet - the model exists so
// its table and repository can be built alongside the core flow and the
// schema stays complete.
package payment

import "time"

// Status lifecycle.
const (
	StatusPending  = "pending"
	StatusPaid     = "paid"
	StatusFailed   = "failed"
	StatusRefunded = "refunded"
)

// Provider values.
const (
	ProviderStripe = "stripe"
)

// Payment is one attempt to pay for access to a job's original file.
type Payment struct {
	ID          string    `db:"id,primary_key,uuid"`
	JobID       string    `db:"job_id,notnull,index,references:jobs(id),on_delete:cascade"`
	Provider    string    `db:"provider,notnull"`
	ProviderRef string    `db:"provider_ref,index"` // provider-side checkout / session id
	Status      string    `db:"status,notnull,default:pending"`
	AmountCents int64     `db:"amount_cents,notnull"`
	Currency    string    `db:"currency,notnull,default:eur"`
	CreatedAt   time.Time `db:"created_at,oncreate"`
	UpdatedAt   time.Time `db:"updated_at,onwrite"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Payment) TableName() string { return "payments" }

// Response is the API view of a payment.
type Response struct {
	ID          string    `json:"id"`
	JobID       string    `json:"jobId"`
	Provider    string    `json:"provider"`
	Status      string    `json:"status"`
	AmountCents int64     `json:"amountCents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func toResponse(p Payment) Response {
	return Response{
		ID:          p.ID,
		JobID:       p.JobID,
		Provider:    p.Provider,
		Status:      p.Status,
		AmountCents: p.AmountCents,
		Currency:    p.Currency,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
