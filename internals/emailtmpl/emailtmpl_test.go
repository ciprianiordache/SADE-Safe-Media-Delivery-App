package emailtmpl

import (
	"strings"
	"testing"
	"time"
)

func TestRenderIncludesDynamicValues(t *testing.T) {
	html, err := Render(Data{
		Subject:     "Sign in to SADE",
		Preheader:   "Your sign-in link",
		Heading:     "Sign in to SADE",
		Intro:       "Click below to sign in.",
		Buttons:     []Button{{Label: "Sign in", URL: "https://sade.example/api/auth/callback?token=abc&x=1", Primary: true}},
		FallbackURL: "https://sade.example/api/auth/callback?token=abc&x=1",
		Note:        "This link expires in 15 minutes.",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"<title>Sign in to SADE</title>",
		"Sign in to SADE",
		"Click below to sign in.",
		">Sign in</a>",
		"cid:" + LogoCID,
		"This link expires in 15 minutes.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
	// The URL's "&" must come out HTML-escaped (as &amp;), not raw, or the
	// document is malformed - html/template does this automatically, this
	// just pins the behavior.
	if !strings.Contains(html, "token=abc&amp;x=1") {
		t.Errorf("expected escaped ampersand in URL, got:\n%s", html)
	}
}

func TestRenderEscapesUserSuppliedText(t *testing.T) {
	html, err := Render(Data{
		Subject: "s", Heading: `<script>alert(1)</script>`,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("heading was not escaped:\n%s", html)
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{15 * time.Minute, "15 minutes"},
		{time.Minute, "1 minute"},
		{90 * time.Minute, "2 hours"}, // rounds to the nearest whole hour
		{time.Hour, "1 hour"},
		{2 * time.Hour, "2 hours"},
		{24 * time.Hour, "1 day"},
		{720 * time.Hour, "30 days"},
	}
	for _, c := range cases {
		if got := HumanDuration(c.d); got != c.want {
			t.Errorf("HumanDuration(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestHumanDurationInRomanian(t *testing.T) {
	cases := map[time.Duration]string{
		time.Minute:      "1 minut",
		15 * time.Minute: "15 minute",
		time.Hour:        "1 oră",
		2 * time.Hour:    "2 ore",
		20 * time.Hour:   "20 de ore",
		24 * time.Hour:   "1 zi",
		720 * time.Hour:  "30 de zile",
	}
	for d, want := range cases {
		if got := HumanDurationIn("ro", d); got != want {
			t.Errorf("HumanDurationIn(ro, %s) = %q, want %q", d, got, want)
		}
	}
}

func TestRenderLanguageAndRows(t *testing.T) {
	html, err := Render(Data{
		Subject: "s", Heading: "h",
		Rows: []Row{{Label: "Fișier", Value: "nunta.mp4"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<html lang="ro"`, "<h1 ", "nunta.mp4", "Livrare sigură de fișiere media", "Reddit+Sans"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
	en, _ := Render(Data{Lang: "en", Subject: "s", Heading: "h"})
	if !strings.Contains(en, `<html lang="en"`) || !strings.Contains(en, "Safe media delivery") {
		t.Errorf("English chrome missing")
	}
}

func TestFormatTimeIsBucharestTime(t *testing.T) {
	at := time.Date(2026, 11, 12, 9, 41, 0, 0, time.UTC) // 11:41 in Bucharest (EET)
	if got := FormatTime("ro", at); got != "12 nov. 2026, 11:41" {
		t.Errorf("FormatTime = %q", got)
	}
}

// html/template strips comments written in the template, so the Outlook-only
// blocks must arrive as trusted values: a fixed 560px column and a VML pill
// per button, with the ordinary <a> hidden from Outlook.
func TestRenderKeepsOutlookMarkup(t *testing.T) {
	html, err := Render(Data{Subject: "s", Heading: "h",
		Buttons: []Button{{Label: "Deschide fișierul", URL: "https://sade.example/preview/a&b", Primary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<!--[if mso]><table role="presentation" width="560"`,
		`<v:roundrect`, `href="https://sade.example/preview/a&amp;b"`, `fillcolor="#e8631c"`,
		`<!--[if !mso]><!-->`, `<!--<![endif]-->`,
		`<o:PixelsPerInch>96</o:PixelsPerInch>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

func TestRenderGroupsTheCode(t *testing.T) {
	html, _ := Render(Data{Subject: "s", Heading: "h", Code: "482913", CodeLabel: "Cod"})
	if !strings.Contains(html, `482<span style="display:inline-block;width:12px;"></span>913`) {
		t.Errorf("code not drawn as two groups:\n%s", html)
	}
	if strings.Contains(html, "482 913") {
		t.Errorf("the gap must be drawn, not typed, so a copy pastes as 482913")
	}
}

func TestTextFooter(t *testing.T) {
	if got := TextFooter("ro", "De ce."); got != "--\nSADE · Livrare sigură de fișiere media\nDe ce.\n" {
		t.Errorf("TextFooter = %q", got)
	}
}
