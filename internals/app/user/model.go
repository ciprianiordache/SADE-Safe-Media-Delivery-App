// Package user is the account domain. A user is created the first time an
// email address requests a magic link - there is no password. Authentication
// happens entirely through the magic_token and session domains.
package user

import "time"

// Role values. Operators manage their own uploads and jobs; admins can also
// manage other accounts.
const (
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

// User is an account, keyed by email and carrying no credentials.
type User struct {
	ID        string    `db:"id,primary_key,uuid"`
	Email     string    `db:"email,notnull,unique"`
	Role      string    `db:"role,notnull,default:operator"`
	CreatedAt time.Time `db:"created_at,oncreate"`
	UpdatedAt time.Time `db:"updated_at,onwrite"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (User) TableName() string { return "users" }

// CreateRequest provisions an account. Normally implicit - the auth service
// calls this on the first magic-link request for an unknown address.
type CreateRequest struct {
	Email string `json:"email"`
}

// UpdateRequest changes an existing account. Only the role is mutable; the
// email is the identity and never changes.
type UpdateRequest struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// Response is the API view of a user. The db-tagged struct never leaves the
// service layer.
type Response struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toResponse(u User) Response {
	return Response{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
