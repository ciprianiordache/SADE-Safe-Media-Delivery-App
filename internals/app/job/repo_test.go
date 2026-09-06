package job

import (
	"testing"
	"time"

	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/testutil"
)

func seedRepo(t *testing.T) (Repo, *database.Database, string) {
	t.Helper()
	db := testutil.DB(t, user.User{}, Job{})
	uid, err := db.CRUD().Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewRepo(db), db, uid
}

func mkJob(t *testing.T, r Repo, uid, media string) string {
	t.Helper()
	id, err := r.Create(&Job{
		UserID: uid, Status: StatusPending, MediaType: media,
		RecipientEmail: "client@example.com", WatermarkKind: WatermarkBoth,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	return id
}

func status(t *testing.T, db *database.Database, id string) Job {
	t.Helper()
	var j Job
	if err := db.CRUD().ReadOne(Job{}, "id", id, &j); err != nil {
		t.Fatalf("read job %s: %v", id, err)
	}
	return j
}

func TestClaimPendingIsOrderedBatchedAndExclusive(t *testing.T) {
	r, db, uid := seedRepo(t)
	a := mkJob(t, r, uid, MediaVideo)
	time.Sleep(2 * time.Millisecond)
	b := mkJob(t, r, uid, MediaAudio)
	time.Sleep(2 * time.Millisecond)
	c := mkJob(t, r, uid, MediaImage)

	now := time.Now()
	first, err := r.ClaimPending(2, now)
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if len(first) != 2 || first[0].ID != a || first[1].ID != b {
		t.Fatalf("claim batch = %v, want [%s %s] oldest-first", ids(first), a, b)
	}
	for _, j := range first {
		if j.Status != StatusProcessing || j.Attempts != 1 {
			t.Errorf("claimed job %s: status=%q attempts=%d", j.ID, j.Status, j.Attempts)
		}
		if got := status(t, db, j.ID); got.Status != StatusProcessing || got.Attempts != 1 {
			t.Errorf("persisted job %s: status=%q attempts=%d", j.ID, got.Status, got.Attempts)
		}
	}

	rest, err := r.ClaimPending(10, now)
	if err != nil {
		t.Fatalf("ClaimPending (rest): %v", err)
	}
	if len(rest) != 1 || rest[0].ID != c {
		t.Fatalf("second claim = %v, want [%s]", ids(rest), c)
	}

	empty, err := r.ClaimPending(10, now)
	if err != nil {
		t.Fatalf("ClaimPending (empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("third claim returned %d rows, want 0", len(empty))
	}
}

func TestMarkForRetryHonoursBackoffWindow(t *testing.T) {
	r, db, uid := seedRepo(t)
	id := mkJob(t, r, uid, MediaVideo)

	now := time.Now()
	if _, err := r.ClaimPending(1, now); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := r.MarkForRetry(id, "ffmpeg blew up", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkForRetry: %v", err)
	}
	j := status(t, db, id)
	if j.Status != StatusPending || j.Error != "ffmpeg blew up" {
		t.Errorf("after retry: status=%q error=%q", j.Status, j.Error)
	}

	// Not eligible yet: next_attempt_at is in the future.
	if got, _ := r.ClaimPending(1, now); len(got) != 0 {
		t.Errorf("claimed a job still in backoff: %v", ids(got))
	}
	// Eligible once the window passes; this is the second attempt.
	got, err := r.ClaimPending(1, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("claim after backoff: %v", err)
	}
	if len(got) != 1 || got[0].Attempts != 2 {
		t.Fatalf("claim after backoff = %v (attempts want 2)", got)
	}
}

func TestMarkDoneAndMarkFailed(t *testing.T) {
	r, db, uid := seedRepo(t)
	done := mkJob(t, r, uid, MediaVideo)
	failed := mkJob(t, r, uid, MediaVideo)
	if _, err := r.ClaimPending(10, time.Now()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := r.MarkDone(done); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if err := r.MarkFailed(failed, "gave up"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if j := status(t, db, done); j.Status != StatusDone {
		t.Errorf("done job status = %q", j.Status)
	}
	if j := status(t, db, failed); j.Status != StatusFailed || j.Error != "gave up" {
		t.Errorf("failed job = status %q error %q", j.Status, j.Error)
	}

	// MarkDone only advances a row that is still processing.
	if err := r.MarkDone(failed); err != nil {
		t.Fatalf("MarkDone(non-processing): %v", err)
	}
	if j := status(t, db, failed); j.Status != StatusFailed {
		t.Errorf("MarkDone clobbered a failed job: %q", j.Status)
	}
}

func TestResetStuckRequeuesAbandonedProcessingRows(t *testing.T) {
	r, db, uid := seedRepo(t)
	stuck := mkJob(t, r, uid, MediaVideo)
	fresh := mkJob(t, r, uid, MediaVideo)
	if _, err := r.ClaimPending(10, time.Now()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Backdate one processing row to look abandoned.
	old := time.Now().Add(-2 * time.Hour)
	if _, err := db.Exec(rebindFor(db, "UPDATE jobs SET updated_at = ? WHERE id = ?"), old, stuck); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	n, err := r.ResetStuck(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ResetStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("ResetStuck affected %d rows, want 1", n)
	}
	if j := status(t, db, stuck); j.Status != StatusPending {
		t.Errorf("stuck job not requeued: %q", j.Status)
	}
	if j := status(t, db, fresh); j.Status != StatusProcessing {
		t.Errorf("fresh job wrongly reset: %q", j.Status)
	}
	// Requeued and immediately eligible again.
	if got, _ := r.ClaimPending(10, time.Now()); len(got) != 1 || got[0].ID != stuck {
		t.Errorf("requeued job not claimable: %v", ids(got))
	}
}

func ids(js []Job) []string {
	out := make([]string, len(js))
	for i, j := range js {
		out[i] = j.ID
	}
	return out
}

// rebindFor mirrors repo.rebind for the test's own raw UPDATE.
func rebindFor(db *database.Database, q string) string {
	return (&repo{db: db}).rebind(q)
}
