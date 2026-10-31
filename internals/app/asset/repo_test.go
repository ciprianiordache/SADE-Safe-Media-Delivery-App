package asset_test

import (
	"errors"
	"testing"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/user"
	"sade/internals/testutil"

	crud "github.com/ciprianiordache/crud-depot"
)

// seedJob creates the user + job an asset needs to satisfy the foreign key.
func seedJob(t *testing.T, c *crud.CRUD) string {
	t.Helper()
	uid, err := c.Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jid, err := c.Create(&job.Job{
		UserID: uid, MediaType: job.MediaVideo, RecipientEmail: "client@example.com",
	})
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return jid
}

func TestRepoCRUD(t *testing.T) {
	db := testutil.DB(t, user.User{}, job.Job{}, asset.Asset{})
	r := asset.NewRepo(db)
	jid := seedJob(t, db.CRUD())

	oid, err := r.Create(&asset.Asset{
		JobID: jid, Kind: asset.KindOriginal, StorageKey: "originals/" + jid + "/original.mp4",
		Filename: "clip.mp4", MIME: "video/mp4", SizeBytes: 1234, Checksum: "abc",
	})
	if err != nil {
		t.Fatalf("Create original: %v", err)
	}
	if oid == "" {
		t.Fatal("Create returned empty id")
	}

	// A second asset a moment later, so ListByJob ordering is observable.
	time.Sleep(2 * time.Millisecond)
	if _, err := r.Create(&asset.Asset{
		JobID: jid, Kind: asset.KindPreview, StorageKey: "previews/" + jid + "/preview.mp4",
		Filename: "clip.mp4", MIME: "video/mp4", SizeBytes: 987,
	}); err != nil {
		t.Fatalf("Create preview: %v", err)
	}

	got, err := r.GetByID(oid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Kind != asset.KindOriginal || got.Checksum != "abc" || got.CreatedAt.IsZero() {
		t.Errorf("GetByID = %+v", got)
	}

	list, err := r.ListByJob(jid)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListByJob = %d rows, %v", len(list), err)
	}
	if list[0].Kind != asset.KindOriginal || list[1].Kind != asset.KindPreview {
		t.Errorf("ListByJob not oldest-first: %q, %q", list[0].Kind, list[1].Kind)
	}

	prev, err := r.GetByJobAndKind(jid, asset.KindPreview)
	if err != nil || prev.SizeBytes != 987 {
		t.Errorf("GetByJobAndKind(preview) = %+v, %v", prev, err)
	}
	if _, err := r.GetByJobAndKind(jid, "nope"); !errors.Is(err, asset.ErrNotFound) {
		t.Errorf("GetByJobAndKind(unknown) = %v, want ErrNotFound", err)
	}

	if _, err := r.GetByID("missing"); !errors.Is(err, asset.ErrNotFound) {
		t.Errorf("GetByID(missing) = %v, want ErrNotFound", err)
	}
	if empty, err := r.ListByJob("missing"); err != nil || len(empty) != 0 {
		t.Errorf("ListByJob(missing) = %d rows, %v", len(empty), err)
	}

	if err := r.Delete(oid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(oid); !errors.Is(err, asset.ErrNotFound) {
		t.Errorf("GetByID after Delete = %v, want ErrNotFound", err)
	}
	if err := r.Delete("missing"); !errors.Is(err, asset.ErrNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrNotFound", err)
	}
}

func TestUpsertPreviewKeepsOneRowAndItsID(t *testing.T) {
	db := testutil.DB(t, user.User{}, job.Job{}, asset.Asset{})
	r := asset.NewRepo(db)
	jid := seedJob(t, db.CRUD())

	first, err := r.UpsertPreview(&asset.Asset{
		JobID: jid, StorageKey: "previews/" + jid + "/a-preview.mp4",
		Filename: "a-preview.mp4", MIME: "video/mp4", SizeBytes: 10,
	})
	if err != nil {
		t.Fatalf("first UpsertPreview: %v", err)
	}
	second, err := r.UpsertPreview(&asset.Asset{
		JobID: jid, StorageKey: "previews/" + jid + "/a-preview.mp4",
		Filename: "a-preview.mp4", MIME: "video/mp4", SizeBytes: 20, Checksum: "new",
	})
	if err != nil {
		t.Fatalf("second UpsertPreview: %v", err)
	}
	if first != second {
		t.Errorf("reprocessing changed the preview id (%s -> %s): emailed links would break", first, second)
	}
	all, err := r.ListByJob(jid)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].SizeBytes != 20 || all[0].Checksum != "new" {
		t.Errorf("want one updated preview row, got %+v", all)
	}
}
