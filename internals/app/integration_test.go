package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/magic_token"
	"sade/internals/app/payment"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// TestFullGraphRoundTrip exercises every model and every foreign key against a
// real engine: create a user, then a magic token, a session, a job, an
// original + preview asset, and a payment - each referencing its parent - and
// read them all back. It also checks that ON DELETE CASCADE removes the whole
// tree when the user is deleted.
func TestFullGraphRoundTrip(t *testing.T) {
	db := database.New(config.DatabaseConfig{Driver: "sqlite", Name: ":memory:"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := db.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(Models()...); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	c := db.CRUD()
	now := time.Now()

	uid, err := c.Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	var u user.User
	if err := c.ReadOne(user.User{}, "id", uid, &u); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if u.Role != user.RoleOperator {
		t.Errorf("role default not applied: %q", u.Role)
	}
	if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
		t.Errorf("oncreate/onwrite timestamps not set: %+v", u)
	}

	if _, err := c.Create(&magic_token.MagicToken{
		UserID: uid, TokenHash: "hash-mt", ExpiresAt: now.Add(15 * time.Minute),
	}); err != nil {
		t.Fatalf("create magic token: %v", err)
	}
	if _, err := c.Create(&session.Session{
		UserID: uid, TokenHash: "hash-sess", ExpiresAt: now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	jid, err := c.Create(&job.Job{
		UserID: uid, MediaType: job.MediaVideo, RecipientEmail: "client@example.com",
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	var j job.Job
	if err := c.ReadOne(job.Job{}, "id", jid, &j); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if j.Status != job.StatusPending || j.WatermarkKind != job.WatermarkBoth || j.Attempts != 0 {
		t.Errorf("job defaults wrong: status=%q kind=%q attempts=%d", j.Status, j.WatermarkKind, j.Attempts)
	}

	if _, err := c.Create(&asset.Asset{
		JobID: jid, Kind: asset.KindOriginal, StorageKey: "originals/" + jid + "/o.mp4",
		Filename: "clip.mp4", MIME: "video/mp4", SizeBytes: 1234,
	}); err != nil {
		t.Fatalf("create original asset: %v", err)
	}
	if _, err := c.Create(&asset.Asset{
		JobID: jid, Kind: asset.KindPreview, StorageKey: "previews/" + jid + "/p.mp4",
		Filename: "clip.mp4", MIME: "video/mp4", SizeBytes: 987,
	}); err != nil {
		t.Fatalf("create preview asset: %v", err)
	}
	var assets []asset.Asset
	if err := c.Read(asset.Asset{}, "job_id", jid, &assets); err != nil {
		t.Fatalf("read assets: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets for the job, got %d", len(assets))
	}

	if _, err := c.Create(&payment.Payment{
		JobID: jid, Provider: payment.ProviderStripe, AmountCents: 500, Currency: "eur",
	}); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	// ON DELETE CASCADE: dropping the user must take the whole tree with it.
	if err := c.Delete(user.User{}, "id", uid); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{"job", func() error { return c.ReadOne(job.Job{}, "id", jid, &job.Job{}) }},
		{"magic_token", func() error {
			var mt magic_token.MagicToken
			return c.ReadOne(magic_token.MagicToken{}, "user_id", uid, &mt)
		}},
		{"session", func() error {
			var s session.Session
			return c.ReadOne(session.Session{}, "user_id", uid, &s)
		}},
		{"asset", func() error {
			var a asset.Asset
			return c.ReadOne(asset.Asset{}, "job_id", jid, &a)
		}},
		{"payment", func() error {
			var p payment.Payment
			return c.ReadOne(payment.Payment{}, "job_id", jid, &p)
		}},
	} {
		if err := check.run(); !errors.Is(err, crud.ErrNotFound) {
			t.Errorf("%s survived user delete (cascade failed): err = %v", check.name, err)
		}
	}
}
