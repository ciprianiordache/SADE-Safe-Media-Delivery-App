package job

import (
	"context"
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
	in.WatermarkOpts = `{"logoKey":"brands/someone-else/x.png"}` // client opts are ignored
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
}
