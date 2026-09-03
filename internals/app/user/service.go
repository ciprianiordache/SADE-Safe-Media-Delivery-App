package user

import (
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
)

// Service is the account domain's use cases. Reads are unlogged; the one
// mutation path (account creation, role change) is logged.
type Service interface {
	// EnsureByEmail returns the account for email, creating a fresh operator
	// account the first time an address is seen. This is the entry point for
	// magic-link login.
	EnsureByEmail(email string) (Response, error)
	GetByID(id string) (Response, error)
	List(offset, limit int) ([]Response, error)
	// SetRole changes an account's role (admin action). role must be a known
	// Role* value.
	SetRole(id, role string) (Response, error)
}

type service struct {
	repo Repo
	log  *slog.Logger
}

func NewService(repo Repo, log *slog.Logger) Service {
	return &service{repo: repo, log: log}
}

func (s *service) EnsureByEmail(email string) (Response, error) {
	norm, err := normalizeEmail(email)
	if err != nil {
		return Response{}, err
	}

	switch u, err := s.repo.GetByEmail(norm); {
	case err == nil:
		return toResponse(*u), nil
	case !errors.Is(err, ErrNotFound):
		return Response{}, fmt.Errorf("lookup user: %w", err)
	}

	u := &User{Email: norm, Role: RoleOperator}
	id, err := s.repo.Create(u)
	if err != nil {
		// Lost a race with a concurrent first login for the same address:
		// the row now exists, so read it back.
		if again, gErr := s.repo.GetByEmail(norm); gErr == nil {
			return toResponse(*again), nil
		}
		s.log.Error("failed to create user", "email", norm, "error", err)
		return Response{}, fmt.Errorf("create user: %w", err)
	}
	u.ID = id
	s.log.Info("user created", "id", id, "email", norm)
	return toResponse(*u), nil
}

func (s *service) GetByID(id string) (Response, error) {
	u, err := s.repo.GetByID(id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*u), nil
}

func (s *service) List(offset, limit int) ([]Response, error) {
	users, err := s.repo.List(offset, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Response, len(users))
	for i, u := range users {
		out[i] = toResponse(u)
	}
	return out, nil
}

func (s *service) SetRole(id, role string) (Response, error) {
	if !validRole(role) {
		return Response{}, ErrInvalidInput
	}
	u, err := s.repo.GetByID(id)
	if err != nil {
		return Response{}, err
	}
	if u.Role == role {
		return toResponse(*u), nil
	}
	u.Role = role
	if err := s.repo.Update(u); err != nil {
		s.log.Error("failed to update user role", "id", id, "role", role, "error", err)
		return Response{}, fmt.Errorf("update user: %w", err)
	}
	s.log.Info("user role changed", "id", id, "role", role)
	return toResponse(*u), nil
}

func normalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrInvalidInput
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidInput
	}
	// ParseAddress accepts "Name <a@b>"; keep only the address, lowercased.
	return strings.ToLower(addr.Address), nil
}

func validRole(role string) bool {
	return role == RoleOperator || role == RoleAdmin
}
