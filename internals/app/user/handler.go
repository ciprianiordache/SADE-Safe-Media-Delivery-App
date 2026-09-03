package user

import (
	"errors"
	"log/slog"
	"net/http"

	"sade/internals/httpx"
)

// Handler is the admin-facing HTTP surface for accounts. The current-user
// endpoint (/api/me) lives in the auth package, since it is about the
// session, not account management. Routes are registered centrally in
// internals/app/router.go.
type Handler struct {
	svc Service
	log *slog.Logger
}

func NewHandler(svc Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// List handles GET /api/users?offset=&limit=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	offset, limit := httpx.Page(r, 50, 200)
	users, err := h.svc.List(offset, limit)
	if err != nil {
		h.log.Error("list users", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list users")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, users)
}

// Get handles GET /api/users/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetByID(r.PathValue("id"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "user not found")
	case err != nil:
		h.log.Error("get user", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load user")
	default:
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}

// SetRole handles PATCH /api/users/{id} with body {"role": "..."}.
func (h *Handler) SetRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.svc.SetRole(r.PathValue("id"), body.Role)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "user not found")
	case errors.Is(err, ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, "unknown role")
	case err != nil:
		h.log.Error("set user role", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not update user")
	default:
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}
