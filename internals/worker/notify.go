package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sade/internals/emailtmpl"
	"sade/internals/mailer"
	"sade/internals/token"
)

// EmailNotifier is the production Notifier: it signs "preview" and "download"
// capability tokens for the preview asset and emails the recipient the
// frontend's /preview/<token> page (the embedded player + Stripe unlock,
// backed by the same signed token /p/<token> itself would verify) as the
// primary link, plus a direct /d/<token> download. It holds no state beyond
// its dependencies, so one instance is shared by every worker goroutine.
type EmailNotifier struct {
	mail      mailer.Mailer
	signer    *token.Signer
	publicURL string
	ttl       time.Duration

	// Branding, when set, names the sending company (and its logo PNG, if
	// any) for a preview asset, so the email reads as theirs.
	Branding func(ctx context.Context, previewID string) (company string, logoPNG []byte)
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

// PreviewReady emails recipient the frontend preview page (view + unlock)
// plus a direct download link for the preview asset.
func (n *EmailNotifier) PreviewReady(ctx context.Context, recipient, previewID string) error {
	viewTok, err := n.signer.Sign("preview", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign preview token: %w", err)
	}
	dlTok, err := n.signer.Sign("download", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign download token: %w", err)
	}
	company, logo := "", []byte(nil)
	if n.Branding != nil {
		company, logo = n.Branding(ctx, previewID)
	}
	sender, subject := "Someone", "Your preview is ready"
	if company != "" {
		sender, subject = company, company+" sent you a preview"
	}
	viewLink := n.publicURL + "/preview/" + viewTok
	dlLink := n.publicURL + "/d/" + dlTok
	human := emailtmpl.HumanDuration(n.ttl)
	body := fmt.Sprintf(
		sender+" shared a watermarked preview with you.\n\n"+
			"View it:     %s\n"+
			"Download it: %s\n\n"+
			"The links are valid for %s.\n"+
			"If you were not expecting this, you can ignore this email.\n",
		viewLink, dlLink, n.ttl,
	)
	html, err := emailtmpl.Render(emailtmpl.Data{
		Subject:   subject,
		Preheader: "Your watermarked preview is ready to view",
		Heading:   subject,
		Intro:     sender + " shared a watermarked media preview with you through SADE. View it in your browser, or download it directly.",
		BrandName: company,
		BrandLogo: len(logo) > 0,
		Buttons: []emailtmpl.Button{
			{Label: "View preview", URL: viewLink, Primary: true},
			{Label: "Download", URL: dlLink},
		},
		Note: "These links are valid for " + human + ".",
	})
	if err != nil {
		return fmt.Errorf("render preview email: %w", err)
	}
	inline := []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}}
	if len(logo) > 0 {
		inline = []mailer.Inline{{CID: emailtmpl.BrandLogoCID, ContentType: "image/png", Data: logo}}
	}
	return n.mail.Send(ctx, mailer.Message{
		To:      recipient,
		Subject: subject,
		Text:    body,
		HTML:    html,
		Inline:  inline,
	})
}
