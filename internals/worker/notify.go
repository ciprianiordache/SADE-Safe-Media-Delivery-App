package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sade/internals/mailer"
	"sade/internals/token"
)

// EmailNotifier is the production Notifier: it signs "preview" and "download"
// capability tokens for the preview asset and emails the recipient the public
// /p/<token> (view) and /d/<token> (download) links. It holds no state beyond
// its dependencies, so one instance is shared by every worker goroutine.
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

// PreviewReady emails recipient signed view + download links for the preview
// asset.
func (n *EmailNotifier) PreviewReady(ctx context.Context, recipient, previewID string) error {
	viewTok, err := n.signer.Sign("preview", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign preview token: %w", err)
	}
	dlTok, err := n.signer.Sign("download", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign download token: %w", err)
	}
	body := fmt.Sprintf(
		"Your watermarked preview is ready.\n\n"+
			"View it:     %s/p/%s\n"+
			"Download it: %s/d/%s\n\n"+
			"The links are valid for %s.\n"+
			"If you were not expecting this, you can ignore this email.\n",
		n.publicURL, viewTok, n.publicURL, dlTok, n.ttl,
	)
	return n.mail.Send(ctx, mailer.Message{
		To:      recipient,
		Subject: "Your preview is ready",
		Text:    body,
	})
}
