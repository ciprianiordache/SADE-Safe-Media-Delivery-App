package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"sade/internals/emailtmpl"
	"sade/internals/mailer"
	"sade/internals/token"
)

type capMailer struct{ last mailer.Message }

func (c *capMailer) Send(_ context.Context, m mailer.Message) error {
	c.last = m
	return nil
}

func TestEmailNotifierSendsOneSignedPageLink(t *testing.T) {
	signer, err := token.New("notifier-test-secret-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	cm := &capMailer{}
	n := NewEmailNotifier(cm, signer, "https://sade.example/", 720*time.Hour)

	if err := n.PreviewReady(context.Background(), "client@example.com", "prev-42"); err != nil {
		t.Fatalf("PreviewReady: %v", err)
	}

	if cm.last.To != "client@example.com" {
		t.Errorf("message envelope = %+v", cm.last)
	}
	// No lookup installed: the email still goes out, without a sender.
	if cm.last.Subject != "Ai primit un fișier prin SADE" {
		t.Errorf("subject = %q, want the anonymous one", cm.last.Subject)
	}
	// SADE's logo is the only inline image: no operator branding (TODOS.md).
	if len(cm.last.Inline) != 1 || cm.last.Inline[0].CID != emailtmpl.LogoCID {
		t.Errorf("inline images = %+v, want only SADE's logo", cm.last.Inline)
	}
	body := cm.last.Text

	view := extractLink(t, body, "https://sade.example/preview/")
	if sub, err := signer.Verify("preview", strings.TrimPrefix(view, "https://sade.example/preview/")); err != nil || sub != "prev-42" {
		t.Errorf("view token: subject=%q err=%v", sub, err)
	}
	// The page is the one destination: no raw download link that skips it.
	for _, m := range []string{body, cm.last.HTML} {
		if strings.Contains(m, "https://sade.example/d/") {
			t.Errorf("email carries a direct download link")
		}
	}
	// Durations read as words (the text body once said "720h0m0s").
	if !strings.Contains(body, "30 de zile") || strings.Contains(body, "720h") {
		t.Errorf("expiry not in words:\n%s", body)
	}
}

func TestEmailNotifierNamesTheSenderAndTheFile(t *testing.T) {
	signer, _ := token.New("notifier-test-secret-000000000000")
	cm := &capMailer{}
	n := NewEmailNotifier(cm, signer, "https://sade.example", time.Hour)
	n.UseDetails(func(id string) (DeliveryInfo, error) {
		return DeliveryInfo{Sender: "ana@studio.example", FileName: "nunta.mp4", MediaType: "video", SizeBytes: 5 << 20, Payments: true}, nil
	})
	if err := n.PreviewReady(context.Background(), "client@example.com", "prev-1"); err != nil {
		t.Fatal(err)
	}
	if cm.last.Subject != "ana@studio.example ți-a trimis un fișier" {
		t.Errorf("subject = %q", cm.last.Subject)
	}
	for _, want := range []string{"nunta.mp4", "Video · 5,0 MB", "originalul"} {
		if !strings.Contains(cm.last.Text, want) || !strings.Contains(cm.last.HTML, want) {
			t.Errorf("email lacks %q", want)
		}
	}
}

func extractLink(t *testing.T, body, prefix string) string {
	t.Helper()
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatalf("no %q link in body:\n%s", prefix, body)
	}
	link := body[i:]
	if j := strings.IndexAny(link, " \n\r\t"); j >= 0 {
		link = link[:j]
	}
	return link
}
