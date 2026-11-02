package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/webauthn"
)

// softAuthenticator is a minimal in-memory WebAuthn authenticator: one
// P-256 key, "none" attestation. It produces exactly the JSON the frontend
// (src/lib/webauthn.ts) sends, so these tests cover the real wire format.
type softAuthenticator struct {
	t      *testing.T
	rpID   string
	origin string
	key    *ecdsa.PrivateKey
	credID []byte
	userID []byte
	count  uint32
}

func newSoftAuthenticator(t *testing.T, rpID, origin string) *softAuthenticator {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softAuthenticator{t: t, rpID: rpID, origin: origin, key: key, credID: id}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (a *softAuthenticator) clientData(typ string, challenge protocol.URLEncodedBase64) []byte {
	cd, _ := json.Marshal(map[string]any{
		"type": typ, "challenge": b64(challenge), "origin": a.origin, "crossOrigin": false,
	})
	return cd
}

func (a *softAuthenticator) authData(flags byte, attested []byte) []byte {
	rp := sha256.Sum256([]byte(a.rpID))
	out := append([]byte{}, rp[:]...)
	out = append(out, flags)
	cnt := make([]byte, 4)
	binary.BigEndian.PutUint32(cnt, a.count)
	out = append(out, cnt...)
	return append(out, attested...)
}

// create answers navigator.credentials.create().
func (a *softAuthenticator) create(ch *PasskeyChallenge) []byte {
	opts := ch.Options.(*protocol.CredentialCreation)
	a.userID = []byte(opts.Response.User.ID.(protocol.URLEncodedBase64))

	pub := a.key.PublicKey
	cose, err := webauthncbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1,
		-2: pub.X.FillBytes(make([]byte, 32)),
		-3: pub.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		a.t.Fatal(err)
	}
	attested := make([]byte, 16) // AAGUID: zero
	l := make([]byte, 2)
	binary.BigEndian.PutUint16(l, uint16(len(a.credID)))
	attested = append(attested, l...)
	attested = append(attested, a.credID...)
	attested = append(attested, cose...)

	attObj, err := webauthncbor.Marshal(map[string]any{
		"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(0x45, attested), // UP|UV|AT
	})
	if err != nil {
		a.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64(a.credID), "rawId": b64(a.credID), "type": "public-key",
		"clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64(a.clientData("webauthn.create", opts.Response.Challenge)),
			"attestationObject": b64(attObj),
			"transports":        []string{"internal"},
		},
	})
	return body
}

// get answers navigator.credentials.get().
func (a *softAuthenticator) get(ch *PasskeyChallenge) []byte {
	opts := ch.Options.(*protocol.CredentialAssertion)
	a.count++
	ad := a.authData(0x05, nil) // UP|UV
	cd := a.clientData("webauthn.get", opts.Response.Challenge)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64(a.credID), "rawId": b64(a.credID), "type": "public-key",
		"clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64(cd),
			"authenticatorData": b64(ad),
			"signature":         b64(sig),
			"userHandle":        b64(a.userID),
		},
	})
	return body
}

func TestPasskeyEndToEnd(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	wa, err := webauthn.New(&webauthn.Config{RPID: "app.test", RPDisplayName: "SADE", RPOrigins: []string{"http://app.test"}})
	if err != nil {
		t.Fatal(err)
	}
	svc.UsePasskeys(svc.passkeys, wa)

	// Signed in by email first.
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	if _, _, err := completeT(svc, ctx, linkToken(t, cm)); err != nil {
		t.Fatal(err)
	}
	u, _ := svc.users.FindByEmail("op@example.com")
	auth := newSoftAuthenticator(t, "app.test", "http://app.test")

	// Register.
	reg, err := svc.BeginPasskeyRegistration(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := svc.FinishPasskeyRegistration(ctx, u.ID, reg.CeremonyID, "Test key", auth.create(reg))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if list, _ := svc.ListPasskeys(ctx, u.ID); len(list) != 1 || list[0].ID != pk.ID || list[0].Name != "Test key" {
		t.Fatalf("passkeys = %+v", list)
	}
	// The ceremony is single-use.
	if _, err := svc.FinishPasskeyRegistration(ctx, u.ID, reg.CeremonyID, "again", auth.create(reg)); err == nil {
		t.Error("a registration ceremony was accepted twice")
	}

	// Passkey-only sign-in (no email).
	login, err := svc.BeginPasskeyLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := svc.FinishPasskeyLogin(ctx, login.CeremonyID, auth.get(login), chromeWin)
	if err != nil {
		t.Fatalf("passkey sign-in: %v", err)
	}
	if got, err := svc.Authenticate(ctx, secret); err != nil || got.ID != u.ID {
		t.Fatalf("passkey session = %+v, %v", got, err)
	}

	// A different key claiming the same credential is refused.
	impostor := newSoftAuthenticator(t, "app.test", "http://app.test")
	impostor.credID, impostor.userID = auth.credID, auth.userID
	login2, _ := svc.BeginPasskeyLogin(ctx)
	if _, _, err := svc.FinishPasskeyLogin(ctx, login2.CeremonyID, impostor.get(login2), chromeWin); err == nil {
		t.Fatal("an assertion signed by the wrong key was accepted")
	}

	// Require it: the email step then waits on the passkey.
	if _, err := svc.SetRequirePasskey(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	res, err := svc.Complete(ctx, linkToken(t, cm), testNonce, chromeWin)
	if err != nil || res.Passkey == nil {
		t.Fatalf("email step = %+v, %v", res, err)
	}
	secret2, _, err := svc.FinishPasskeyLogin(ctx, res.Passkey.CeremonyID, auth.get(res.Passkey), chromeWin)
	if err != nil {
		t.Fatalf("second factor: %v", err)
	}
	if got, err := svc.Authenticate(ctx, secret2); err != nil || got.ID != u.ID {
		t.Fatalf("second-factor session = %+v, %v", got, err)
	}

	acts, _ := svc.Activity(ctx, u.ID)
	passkeyLogins := 0
	for _, a := range acts {
		if a.Method == "passkey" {
			passkeyLogins++
		}
	}
	if passkeyLogins != 2 {
		t.Errorf("passkey sign-ins recorded = %d, want 2", passkeyLogins)
	}
}
