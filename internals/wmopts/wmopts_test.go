package wmopts

import (
	"strings"
	"testing"
	"time"
)

func TestRenderText(t *testing.T) {
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cases := []struct{ tpl, company, want string }{
		{"{company} · {recipient} · {date}", "Studio X", "Studio X · a@b.c · 2026-11-03"},
		{"{company} · {recipient} · {date}", "", "a@b.c · 2026-11-03"},
		{"", "", "a@b.c · 2026-11-03"},
		{"Confidential - {company}", "", "Confidential · a@b.c"}, // the recipient is never left out
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

// A long text gives up the company first, then the date - never the
// recipient, which is what ties a leaked file to whoever received it.
func TestRenderTextFitKeepsTheRecipient(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	tpl := "{company} · {recipient} · {date}"
	company := strings.Repeat("Studio Foto si Video ", 4) // 84 runes
	rcpt := "alice.popescu@example.com"

	full := RenderText(tpl, company, rcpt, now)
	if got := RenderTextFit(tpl, company, rcpt, now, 0); got != full {
		t.Errorf("no budget: %q, want the full text", got)
	}
	cases := []struct {
		budget int
		want   string
	}{
		// No room for any company: dropped with its separator.
		{38, "alice.popescu@example.com · 2026-11-07"},
		// No room for the date either.
		{30, "alice.popescu@example.com"},
		// Not even the recipient fits: it stays whole (the font shrinks instead).
		{10, "alice.popescu@example.com"},
	}
	for _, c := range cases {
		got := RenderTextFit(tpl, company, rcpt, now, c.budget)
		if got != c.want {
			t.Errorf("budget %d: %q, want %q", c.budget, got, c.want)
		}
		if !strings.Contains(got, rcpt) {
			t.Errorf("budget %d: recipient cut: %q", c.budget, got)
		}
	}
	short := RenderTextFit(tpl, company, rcpt, now, 50)
	if !strings.HasSuffix(strings.SplitN(short, " · ", 2)[0], Ellipsis) || len([]rune(short)) > 50 {
		t.Errorf("budget 50: %q, want a shortened company ending in %q within 50 runes", short, Ellipsis)
	}

	// The operator's own template without a company: only the date can go.
	if got := RenderTextFit("{recipient} · {date}", company, rcpt, now, 30); got != rcpt {
		t.Errorf("no-company template: %q", got)
	}
}

func TestWithRecipient(t *testing.T) {
	cases := []struct {
		text, to string
		max      int
		want     string
	}{
		{"Doar pentru tine", "a@b.ro", 200, "Doar pentru tine · a@b.ro"},
		{"Pentru A@B.ro", "a@b.ro", 200, "Pentru A@B.ro"},
		{"", "a@b.ro", 200, "a@b.ro"},
		{"0123456789", "a@b.ro", 16, "0123... · a@b.ro"}, // the address is never cut
		{"text", "a-very-long@b.ro", 10, "a-very-long@b.ro"},
	}
	for _, c := range cases {
		if got := WithRecipient(c.text, c.to, c.max); got != c.want {
			t.Errorf("WithRecipient(%q, %q, %d) = %q, want %q", c.text, c.to, c.max, got, c.want)
		}
	}
	if got := RenderText("{company}", "Studio", "a@b.ro", time.Now()); got != "Studio · a@b.ro" {
		t.Errorf("template without {recipient} = %q", got)
	}
}
