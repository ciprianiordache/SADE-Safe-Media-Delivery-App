package emailtmpl

import (
	"strings"
	"testing"
	"time"
)

func TestRenderIncludesDynamicValues(t *testing.T) {
	html, err := Render(Data{
		PublicURL:   "https://sade.example",
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
		"https://sade.example/logo.png",
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
		Subject: "s", Heading: `<script>alert(1)</script>`, PublicURL: "https://sade.example",
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
