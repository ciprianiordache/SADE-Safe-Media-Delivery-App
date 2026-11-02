package job_event

import (
	"testing"
	"time"

	"sade/internals/app/job"
	"sade/internals/app/user"
	"sade/internals/testutil"
)

func TestRecordOnceCollapsesRepeats(t *testing.T) {
	db := testutil.DB(t, user.User{}, job.Job{}, JobEvent{})
	uid, _ := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	jid, err := db.CRUD().Create(&job.Job{UserID: uid, Status: "done", MediaType: "video", RecipientEmail: "r@x.y", WatermarkKind: "both"})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRepo(db)
	for i := 0; i < 3; i++ {
		if err := r.RecordOnce(jid, KindViewed, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.Record(jid, KindSent, "r@x.y")
	list, err := r.ListByJob(jid)
	if err != nil || len(list) != 2 {
		t.Fatalf("events = %+v, %v", list, err)
	}
	if n, _ := r.CountSince(jid, KindViewed, time.Now().UTC().Add(-time.Minute)); n != 1 {
		t.Errorf("viewed = %d, want 1", n)
	}
}
