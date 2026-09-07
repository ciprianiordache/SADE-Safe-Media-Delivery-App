package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"sade/internals/mailer"
	"sade/internals/token"
)

type capMailer struct{ last mailer.Message }

func (c *capMailer) Send(_ context.Context, m mailer.Message) error {
	c.last = m
	return nil
}

func TestEmailNotifierSendsSignedViewAndDownloadLinks(t *testing.T) {
	signer, err := token.New("notifier-test-secret-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	cm := &capMailer{}
	n := NewEmailNotifier(cm, signer, "https://sade.example/", time.Hour)

	if err := n.PreviewReady(context.Background(), "client@example.com", "prev-42"); err != nil {
		t.Fatalf("PreviewReady: %v", err)
	}

	if cm.last.To != "client@example.com" || cm.last.Subject == "" {
		t.Errorf("message envelope = %+v", cm.last)
	}
	body := cm.last.Text

	view := extractLink(t, body, "https://sade.example/p/")
	if sub, err := signer.Verify("preview", strings.TrimPrefix(view, "https://sade.example/p/")); err != nil || sub != "prev-42" {
		t.Errorf("view token: subject=%q err=%v", sub, err)
	}

	dl := extractLink(t, body, "https://sade.example/d/")
	if sub, err := signer.Verify("download", strings.TrimPrefix(dl, "https://sade.example/d/")); err != nil || sub != "prev-42" {
		t.Errorf("download token: subject=%q err=%v", sub, err)
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
