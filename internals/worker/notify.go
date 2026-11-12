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
	details   func(previewID string) (DeliveryInfo, error)
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

// DeliveryInfo is what the recipient's email says about a delivery: who
// sent it (the operator's account email - verified by their sign-in, and
// unable to pose as a bank's domain, unlike a self-declared company name,
// which stays off the email until senders are verified; TODOS.md), the file,
// and whether the original can be unlocked on the page.
type DeliveryInfo struct {
	Sender    string
	FileName  string
	MediaType string // "video" | "image" | "audio"
	SizeBytes int64
	Payments  bool
}

// UseDetails installs the lookup the email uses to name the sender and the
// file. Without it (or when it fails) the email still goes out, without them.
func (n *EmailNotifier) UseDetails(f func(previewID string) (DeliveryInfo, error)) {
	n.details = f
}

// PreviewReady emails recipient a link to the frontend preview page, where
// they watch the watermarked copy and, when payments are on, unlock the
// original. There is no direct download link: the page is the one place
// the reader lands, and where the unlock lives.
func (n *EmailNotifier) PreviewReady(ctx context.Context, recipient, previewID string) error {
	viewTok, err := n.signer.Sign("preview", previewID, n.ttl)
	if err != nil {
		return fmt.Errorf("sign preview token: %w", err)
	}
	viewLink := n.publicURL + "/preview/" + viewTok

	var info DeliveryInfo
	if n.details != nil {
		if d, err := n.details(previewID); err == nil {
			info = d
		}
	}
	human := emailtmpl.HumanDurationIn("ro", n.ttl)

	subject := "Ai primit un fișier prin SADE"
	if info.Sender != "" {
		subject = info.Sender + " ți-a trimis un fișier"
	}
	intro := "Îl vezi direct în browser."
	if info.Payments {
		intro += " Din aceeași pagină poți debloca originalul, fără filigran."
	}
	// The sender is in the heading already; the rows say what the file is.
	var rows []emailtmpl.Row
	if info.FileName != "" {
		rows = append(rows, emailtmpl.Row{Label: "Fișier", Value: info.FileName})
	}
	if t := mediaLabel(info.MediaType); t != "" {
		v := t
		if info.SizeBytes > 0 {
			v += " · " + sizeLabel(info.SizeBytes)
		}
		rows = append(rows, emailtmpl.Row{Label: "Tip", Value: v})
	}
	// Said where the reader decides whether to click, not at the bottom.
	reassure := "Nu ai nevoie de cont, iar SADE nu îți cere niciodată parola."
	note := "Copia din pagină poartă un filigran cu adresa ta de email. Linkul e valabil " + human + "."
	footer := "Ai primit acest email pentru că cineva ți-a trimis un fișier prin SADE. Dacă nu te așteptai la el, îl poți ignora."
	if info.Sender != "" {
		// Reply-To is the sender, so a doubtful reader can simply ask them.
		footer = "Ai primit acest email pentru că " + info.Sender + " ți-a trimis un fișier prin SADE. " +
			"Dacă răspunzi la el, mesajul ajunge direct la expeditor."
	}

	var b strings.Builder
	b.WriteString(subject + "\n\n" + intro + "\n\n")
	for _, r := range rows {
		b.WriteString(r.Label + ": " + r.Value + "\n")
	}
	if len(rows) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Deschide fișierul: " + viewLink + "\n" + reassure + "\n\n" + note + "\n\n")
	b.WriteString(emailtmpl.TextFooter("ro", footer))

	html, err := emailtmpl.Render(emailtmpl.Data{
		Lang:        "ro",
		Subject:     subject,
		Preheader:   preheader(info),
		Heading:     subject,
		Intro:       intro,
		Rows:        rows,
		Buttons:     []emailtmpl.Button{{Label: "Deschide fișierul", URL: viewLink, Primary: true}},
		ButtonNote:  reassure,
		FallbackURL: viewLink,
		Note:        note,
		Footer:      footer,
	})
	if err != nil {
		return fmt.Errorf("render preview email: %w", err)
	}
	return n.mail.Send(ctx, mailer.Message{
		To:      recipient,
		ReplyTo: info.Sender,
		Subject: subject,
		Text:    b.String(),
		HTML:    html,
		Inline:  []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}},
	})
}

func preheader(info DeliveryInfo) string {
	if info.FileName != "" {
		return info.FileName + " · se deschide în browser, fără cont"
	}
	return "Se deschide în browser, fără cont"
}

func mediaLabel(t string) string {
	switch t {
	case "video":
		return "Video"
	case "image":
		return "Imagine"
	case "audio":
		return "Audio"
	}
	return ""
}

// sizeLabel is a file size the Romanian way: "150 KB", "2,4 MB".
func sizeLabel(n int64) string {
	const k = 1024.0
	f := float64(n)
	switch {
	case f >= k*k*k:
		return strings.Replace(fmt.Sprintf("%.1f GB", f/(k*k*k)), ".", ",", 1)
	case f >= k*k:
		return strings.Replace(fmt.Sprintf("%.1f MB", f/(k*k)), ".", ",", 1)
	case f >= k:
		return fmt.Sprintf("%.0f KB", f/k)
	}
	return fmt.Sprintf("%d B", n)
}
