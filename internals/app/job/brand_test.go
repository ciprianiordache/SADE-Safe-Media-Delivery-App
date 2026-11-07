package job

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sade/config"
	"sade/internals/wmopts"
)

type fakeBrands struct{ snap string }

func (f fakeBrands) Snapshot(string) (string, error) { return f.snap, nil }

func TestCreateFreezesTheBrand(t *testing.T) {
	ctx := context.Background()

	// No logo of their own: the upload asked for logo+text, gets text only.
	svc, _, uid := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	svc.(BrandAware).UseBrands(fakeBrands{snap: wmopts.Encode(wmopts.Opts{Company: "Studio X"})})
	in := upload("clip.mp4", "fake-video-bytes")
	in.WatermarkKind = WatermarkBoth
	resp, err := svc.Create(ctx, uid, in)
	if err != nil {
		t.Fatal(err)
	}
	if resp.WatermarkKind != WatermarkText {
		t.Errorf("kind without a brand logo = %q, want text", resp.WatermarkKind)
	}

}

func TestCreateWithBrandLogoKeepsKindAndSnapshot(t *testing.T) {
	ctx := context.Background()
	// With a logo, the requested kind stands and the snapshot is stored.
	svc2, db, uid2 := newSvc(t, &memStore{}, nil, config.Defaults().Upload)
	snap := wmopts.Encode(wmopts.Opts{Company: "Studio X", LogoKey: "brands/u/abc.png", Position: "tiled"})
	svc2.(BrandAware).UseBrands(fakeBrands{snap: snap})
	resp, err := svc2.Create(ctx, uid2, upload("clip.mp4", "fake-video-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.WatermarkKind != WatermarkBoth {
		t.Errorf("kind with a brand logo = %q, want both", resp.WatermarkKind)
	}
	j, err := NewRepo(db).GetByID(resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := wmopts.Decode(j.WatermarkOpts); got.Company != "Studio X" || got.Position != "tiled" {
		t.Errorf("stored snapshot = %+v", got)
	}
	// The snapshot (with its storage key) stays server-side.
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "watermarkOpts") || strings.Contains(string(body), "brands/") {
		t.Errorf("job response leaks the brand snapshot: %s", body)
	}
}
