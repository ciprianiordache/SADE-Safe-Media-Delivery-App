package auth

// RequestBody is the payload of POST /api/auth/request.
type RequestBody struct {
	Email string `json:"email"`
}
