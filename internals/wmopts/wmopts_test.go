package wmopts

import (
	"testing"
	"time"
)

func TestRenderText(t *testing.T) {
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cases := []struct{ tpl, company, want string }{
		{"{company} · {recipient} · {date}", "Studio X", "Studio X · a@b.c · 2026-11-03"},
		{"{company} · {recipient} · {date}", "", "a@b.c · 2026-11-03"},
		{"", "", "a@b.c · 2026-11-03"},
		{"Confidential - {company}", "", "Confidential"},
	}
	for _, c := range cases {
		if got := RenderText(c.tpl, c.company, "a@b.c", day); got != c.want {
			t.Errorf("RenderText(%q, %q) = %q, want %q", c.tpl, c.company, got, c.want)
		}
	}
}

func TestDecodeRejectsForeignLogoKeys(t *testing.T) {
	if o := Decode(`{"logoKey":"originals/x/original.mp4"}`); o.LogoKey != "" {
		t.Errorf("a non-brand key survived: %q", o.LogoKey)
	}
	if o := Decode(`{"logoKey":"brands/../originals/x"}`); o.LogoKey != "" {
		t.Errorf("a traversal key survived: %q", o.LogoKey)
	}
	if o := Decode(`not json`); o != (Opts{}) {
		t.Errorf("garbage decoded to %+v", o)
	}
	round := Decode(Encode(Opts{Company: "X", LogoKey: "brands/u/abc.png", Position: "tiled"}))
	if round.Company != "X" || round.LogoKey != "brands/u/abc.png" || round.V != Version {
		t.Errorf("round trip = %+v", round)
	}
}
