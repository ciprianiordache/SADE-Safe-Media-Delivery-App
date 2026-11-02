package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"sade/internals/app/login_event"
	"sade/internals/app/passkey"
	"sade/internals/app/user"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// ceremonyTTL bounds how long a passkey prompt may stay open.
const ceremonyTTL = 5 * time.Minute

// maxPasskeys per account - plenty for phone + laptop + a security key.
const maxPasskeys = 10

// PasskeyChallenge is what the browser needs to run navigator.credentials:
// the options to pass it, and the id to hand back with the result.
type PasskeyChallenge struct {
	CeremonyID string `json:"ceremonyId"`
	Options    any    `json:"options"`
}

// PasskeyView is one row of the account's passkey list.
type PasskeyView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// --- ceremonies -------------------------------------------------------------

type ceremonyKind int

const (
	ceremonyRegister     ceremonyKind = iota
	ceremonyLogin                     // passkey-only sign-in (discoverable)
	ceremonySecondFactor              // passkey after the email step
)

type ceremony struct {
	kind    ceremonyKind
	userID  string // register / second factor: whose passkey it must be
	data    webauthn.SessionData
	expires time.Time
}

// ceremonyStore keeps open passkey challenges in memory. SADE is a single
// process, so a restart only cancels prompts that are open at that moment.
// Each ceremony is single-use: take removes it.
type ceremonyStore struct {
	mu sync.Mutex
	m  map[string]ceremony
}

func newCeremonyStore() *ceremonyStore { return &ceremonyStore{m: map[string]ceremony{}} }

func (c *ceremonyStore) put(cer ceremony) (string, error) {
	id, err := secret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	cer.expires = now.Add(ceremonyTTL)
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.m {
		if now.After(v.expires) {
			delete(c.m, k)
		}
	}
	c.m[id] = cer
	return id, nil
}

func (c *ceremonyStore) take(id string, kinds ...ceremonyKind) (ceremony, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cer, ok := c.m[id]
	if !ok {
		return ceremony{}, false
	}
	delete(c.m, id)
	if time.Now().After(cer.expires) {
		return ceremony{}, false
	}
	for _, k := range kinds {
		if cer.kind == k {
			return cer, true
		}
	}
	return ceremony{}, false
}

// --- go-webauthn user adapter -----------------------------------------------

type waUser struct {
	u     user.Response
	creds []webauthn.Credential
}

func (w waUser) WebAuthnID() []byte                         { return []byte(w.u.ID) }
func (w waUser) WebAuthnName() string                       { return w.u.Email }
func (w waUser) WebAuthnDisplayName() string                { return w.u.Email }
func (w waUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

func (s *Service) loadWAUser(userID string) (waUser, []passkey.Passkey, error) {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return waUser{}, nil, err
	}
	rows, err := s.passkeys.ListByUser(userID)
	if err != nil {
		return waUser{}, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(r.Credential), &c); err != nil {
			s.log.Error("corrupt passkey record", "passkey", r.ID, "error", err)
			continue
		}
		creds = append(creds, c)
	}
	return waUser{u: u, creds: creds}, rows, nil
}

func credID(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

// --- registration (signed in) ----------------------------------------------

// BeginPasskeyRegistration starts adding a passkey to userID's account.
func (s *Service) BeginPasskeyRegistration(_ context.Context, userID string) (*PasskeyChallenge, error) {
	if !s.PasskeysEnabled() {
		return nil, ErrPasskeysDisabled
	}
	wu, rows, err := s.loadWAUser(userID)
	if err != nil {
		return nil, fmt.Errorf("auth: load user for passkey: %w", err)
	}
	if len(rows) >= maxPasskeys {
		return nil, fmt.Errorf("%w: limit of %d reached", ErrInvalidPasskey, maxPasskeys)
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(wu.creds))
	for _, c := range wu.creds {
		exclude = append(exclude, c.Descriptor())
	}
	opts, data, err := s.webauthn.BeginRegistration(wu,
		webauthn.WithExclusions(exclude),
		// Resident ("discoverable") so the passkey can sign in on its own,
		// without typing an email first.
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("auth: begin passkey registration: %w", err)
	}
	id, err := s.ceremonies.put(ceremony{kind: ceremonyRegister, userID: userID, data: *data})
	if err != nil {
		return nil, err
	}
	return &PasskeyChallenge{CeremonyID: id, Options: opts}, nil
}

// FinishPasskeyRegistration verifies the browser's attestation and stores
// the new passkey under name.
func (s *Service) FinishPasskeyRegistration(_ context.Context, userID, ceremonyID, name string, credential []byte) (PasskeyView, error) {
	if !s.PasskeysEnabled() {
		return PasskeyView{}, ErrPasskeysDisabled
	}
	cer, ok := s.ceremonies.take(ceremonyID, ceremonyRegister)
	if !ok || cer.userID != userID {
		return PasskeyView{}, ErrInvalidPasskey
	}
	wu, _, err := s.loadWAUser(userID)
	if err != nil {
		return PasskeyView{}, fmt.Errorf("auth: load user for passkey: %w", err)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(credential)
	if err != nil {
		return PasskeyView{}, ErrInvalidPasskey
	}
	cred, err := s.webauthn.CreateCredential(wu, cer.data, parsed)
	if err != nil {
		s.log.Info("passkey registration rejected", "user", userID, "error", err)
		return PasskeyView{}, ErrInvalidPasskey
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return PasskeyView{}, err
	}
	name = truncate(name, 60)
	if name == "" {
		name = "Passkey"
	}
	p := &passkey.Passkey{UserID: userID, CredentialID: credID(cred.ID), Credential: string(raw), Name: name}
	id, err := s.passkeys.Create(p)
	if err != nil {
		return PasskeyView{}, fmt.Errorf("auth: store passkey: %w", err)
	}
	s.log.Info("passkey added", "user", userID, "passkey", id)
	return PasskeyView{ID: id, Name: name, CreatedAt: time.Now()}, nil
}

// ListPasskeys returns userID's passkeys.
func (s *Service) ListPasskeys(_ context.Context, userID string) ([]PasskeyView, error) {
	if !s.PasskeysEnabled() {
		return []PasskeyView{}, nil
	}
	rows, err := s.passkeys.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	out := make([]PasskeyView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PasskeyView{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

// DeletePasskey removes one of userID's passkeys. Removing the last one also
// turns "require a passkey" off - otherwise the owner would be locked out.
func (s *Service) DeletePasskey(_ context.Context, userID, id string) error {
	if !s.PasskeysEnabled() {
		return ErrPasskeysDisabled
	}
	if err := s.passkeys.Delete(userID, id); err != nil {
		if errors.Is(err, passkey.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	rest, err := s.passkeys.ListByUser(userID)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		if _, err := s.users.SetRequirePasskey(userID, false); err != nil {
			return err
		}
	}
	s.log.Info("passkey removed", "user", userID, "passkey", id)
	return nil
}

// SetRequirePasskey turns "a passkey is required at every sign-in" on or
// off. Turning it on needs at least one passkey.
func (s *Service) SetRequirePasskey(_ context.Context, userID string, on bool) (user.Response, error) {
	if on {
		if !s.PasskeysEnabled() {
			return user.Response{}, ErrPasskeysDisabled
		}
		rows, err := s.passkeys.ListByUser(userID)
		if err != nil {
			return user.Response{}, err
		}
		if len(rows) == 0 {
			return user.Response{}, ErrNoPasskey
		}
	}
	return s.users.SetRequirePasskey(userID, on)
}

// --- sign-in ------------------------------------------------------------------

// BeginPasskeyLogin starts a passkey-only sign-in: no email, the browser
// offers whichever passkey it holds for this site.
func (s *Service) BeginPasskeyLogin(_ context.Context) (*PasskeyChallenge, error) {
	if !s.PasskeysEnabled() {
		return nil, ErrPasskeysDisabled
	}
	opts, data, err := s.webauthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return nil, fmt.Errorf("auth: begin passkey login: %w", err)
	}
	id, err := s.ceremonies.put(ceremony{kind: ceremonyLogin, data: *data})
	if err != nil {
		return nil, err
	}
	return &PasskeyChallenge{CeremonyID: id, Options: opts}, nil
}

// beginSecondFactor starts the passkey step for an account that requires
// one, after its email step: only that account's passkeys qualify.
func (s *Service) beginSecondFactor(userID string) (*PasskeyChallenge, error) {
	wu, _, err := s.loadWAUser(userID)
	if err != nil {
		return nil, fmt.Errorf("auth: load user for passkey: %w", err)
	}
	opts, data, err := s.webauthn.BeginLogin(wu, webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return nil, fmt.Errorf("auth: begin passkey step: %w", err)
	}
	id, err := s.ceremonies.put(ceremony{kind: ceremonySecondFactor, userID: userID, data: *data})
	if err != nil {
		return nil, err
	}
	return &PasskeyChallenge{CeremonyID: id, Options: opts}, nil
}

// FinishPasskeyLogin verifies the browser's assertion for either kind of
// passkey sign-in and starts the session.
func (s *Service) FinishPasskeyLogin(ctx context.Context, ceremonyID string, credential []byte, c Client) (string, time.Time, error) {
	if !s.PasskeysEnabled() {
		return "", time.Time{}, ErrPasskeysDisabled
	}
	cer, ok := s.ceremonies.take(ceremonyID, ceremonyLogin, ceremonySecondFactor)
	if !ok {
		return "", time.Time{}, ErrInvalidPasskey
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return "", time.Time{}, ErrInvalidPasskey
	}

	var wu waUser
	var cred *webauthn.Credential
	if cer.kind == ceremonySecondFactor {
		wu, _, err = s.loadWAUser(cer.userID)
		if err != nil {
			return "", time.Time{}, ErrInvalidPasskey
		}
		cred, err = s.webauthn.ValidateLogin(wu, cer.data, parsed)
	} else {
		handler := func(_, userHandle []byte) (webauthn.User, error) {
			u, _, err := s.loadWAUser(string(userHandle))
			if err != nil {
				return nil, err
			}
			wu = u
			return u, nil
		}
		cred, err = s.webauthn.ValidateDiscoverableLogin(handler, cer.data, parsed)
	}
	if err != nil || cred == nil {
		s.log.Info("passkey sign-in rejected", "error", err)
		return "", time.Time{}, ErrInvalidPasskey
	}

	// Store the moved sign counter (clone detection) and the last use.
	row, err := s.passkeys.GetByCredentialID(credID(cred.ID))
	if err != nil || row.UserID != wu.u.ID {
		return "", time.Time{}, ErrInvalidPasskey
	}
	if raw, err := json.Marshal(cred); err == nil {
		if err := s.passkeys.Used(row.ID, string(raw), time.Now()); err != nil {
			s.log.Warn("update passkey", "passkey", row.ID, "error", err)
		}
	}
	return s.startSession(ctx, wu.u, login_event.MethodPasskey, c)
}
