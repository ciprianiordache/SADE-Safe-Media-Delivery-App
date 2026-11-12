package job

import (
	"testing"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job_event"
	"sade/internals/app/payment"
	"sade/internals/app/user"
	"sade/internals/testutil"
)

func TestDeliverySummary(t *testing.T) {
	db := testutil.DB(t, user.User{}, Job{}, asset.Asset{}, job_event.JobEvent{}, payment.Payment{})
	uid, _ := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	mk := func(to string) string {
		id, err := db.CRUD().Create(&Job{UserID: uid, Status: StatusDone, MediaType: "video", RecipientEmail: to, WatermarkKind: "both"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	opened, paid, quiet := mk("o@x.y"), mk("p@x.y"), mk("q@x.y")

	old := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	recent := time.Now().UTC().Truncate(time.Second)
	for _, at := range []time.Time{old, recent} {
		if _, err := db.CRUD().Create(&job_event.JobEvent{JobID: opened, Kind: job_event.KindViewed, CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	// Other kinds don't count as "opened".
	_, _ = db.CRUD().Create(&job_event.JobEvent{JobID: quiet, Kind: job_event.KindSent, CreatedAt: recent})
	if _, err := db.CRUD().Create(&payment.Payment{JobID: paid, Provider: payment.ProviderStripe, Status: payment.StatusPaid, AmountCents: 100, Currency: "eur"}); err != nil {
		t.Fatal(err)
	}

	if _, err := db.CRUD().Create(&asset.Asset{JobID: paid, Kind: asset.KindOriginal, StorageKey: "k", Filename: "nunta.jpg", MIME: "image/jpeg", SizeBytes: 2048}); err != nil {
		t.Fatal(err)
	}

	sum, err := NewRepo(db).DeliverySummary([]string{opened, paid, quiet})
	if err != nil {
		t.Fatal(err)
	}
	if !sum[opened].OpenedAt.Equal(recent) || sum[opened].Paid {
		t.Errorf("opened job = %+v, want the latest view %v", sum[opened], recent)
	}
	if !sum[paid].Paid || !sum[paid].OpenedAt.IsZero() || sum[paid].FileName != "nunta.jpg" || sum[paid].SizeBytes != 2048 {
		t.Errorf("paid job = %+v", sum[paid])
	}
	if _, ok := sum[quiet]; ok {
		t.Errorf("a job with only a 'sent' event got a summary: %+v", sum[quiet])
	}
}
