package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sade/internals/mailer"
	"sade/internals/token"
)

// EmailNotifier is the production Notifier: it signs a "preview" capability
// token for the preview asset and emails the recipient the public /p/<token>
// link. It holds no state beyond its dependencies, so one instance is shared
// by every worker goroutine.
type EmailNotifier struct {
	mail      mailer.Mailer
	signer    *token.Signer
	publicURL string
	ttl       time.Duration
}

// NewEmailNotifier builds the notifier. publicURL is the API base that serves
// the share routes (config.App.PublicURL); ttl is how long the link stays
// valid (config.Auth.ShareTokenTTL).
func NewEmailNotifier(m mailer.Mailer, signer *token.Signer, publicURL string, ttl time.Duration) *EmailNotifier {
	return &EmailNotifier{
		mail:      m,
		signer:    signer,
		publicURL: strings.TrimRight(publicURL, "/"),
		ttl:       ttl,
	}
}

// PreviewReady emails recipient a signed link to the preview asset.
func (n *EmailNotifier) PreviewReady(ctx context.Context, recipient, previewID string) error {
	tok, err := n.signer.Sign("preview", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign preview token: %w", err)
	}
	link := n.publicURL + "/p/" + tok
	body := fmt.Sprintf(
		"Your watermarked preview is ready.\n\n"+
			"Open it here (the link is valid for %s):\n\n%s\n\n"+
			"If you were not expecting this, you can ignore this email.\n",
		n.ttl, link,
	)
	return n.mail.Send(ctx, mailer.Message{
		To:      recipient,
		Subject: "Your preview is ready",
		Text:    body,
	})
}
