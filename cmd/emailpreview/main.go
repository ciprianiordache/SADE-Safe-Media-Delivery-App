// Command emailpreview renders SADE's transactional emails with sample data
// to local HTML files, for eyeballing changes to internals/emailtmpl without
// sending real mail. Not wired into the app; run manually with
// `go run ./cmd/emailpreview -out DIR`. With `-send ADDRESS` it also mails
// the three to that address through the mailer configured in config.yaml /
// .env (the real SMTP account), to see them in an actual inbox.
//
// The recipient's email is rendered by the real worker.EmailNotifier (into a
// capturing mailer), so it is exactly what is sent. The two account emails
// mirror the copy in internals/app/auth/service.go (sign-in) and
// internals/app/auth/devices.go (new sign-in alert); keep them in step.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sade/config"
	"sade/internals/emailtmpl"
	"sade/internals/mailer"
	"sade/internals/token"
	"sade/internals/worker"
)

const base = "https://sade.example"

type capture struct{ last mailer.Message }

func (c *capture) Send(_ context.Context, m mailer.Message) error {
	c.last = m
	return nil
}

func main() {
	out := flag.String("out", ".", "directory to write the rendered .html files into")
	sendTo := flag.String("send", "", "also mail the three emails to this address via the configured mailer")
	flag.Parse()

	signInLink := base + "/auth/confirm#token=UixDTPmKvHFjIGavva_Md3c3GtYzDrwUT69fs9W1sJQ"
	revokeLink := base + "/auth/revoke#token=cmV2b2tlLXNlc3Npb25zLnVzZXItMS4xNzYzNTQ0MDAw"
	signIn := must(emailtmpl.Render(emailtmpl.Data{
		Lang:        "ro",
		Subject:     "Conectare la SADE",
		Preheader:   "Codul tău de conectare: 482913",
		Heading:     "Conectează-te la SADE",
		Intro:       "Apasă butonul ca să intri în cont. Linkul merge doar în browserul în care ai cerut conectarea, e valabil 15 minute și se folosește o singură dată.",
		Buttons:     []emailtmpl.Button{{Label: "Conectează-te", URL: signInLink, Primary: true}},
		CodeLabel:   "Ești pe alt dispozitiv? Scrie acest cod în browserul în care ai cerut conectarea:",
		Code:        "482913",
		FallbackURL: signInLink,
		Note:        "Dacă nu ai cerut tu conectarea, ignoră emailul: nimeni nu poate intra fără el.",
		Footer:      "Ai primit acest email pentru că cineva a cerut conectarea la contul SADE cu această adresă.",
	}))

	newDevice := must(emailtmpl.Render(emailtmpl.Data{
		Lang:      "ro",
		Subject:   "Conectare nouă în contul tău SADE",
		Preheader: "Conectare nouă de pe Chrome pe Windows",
		Heading:   "Conectare nouă în cont",
		Intro:     "Cineva a intrat în contul tău SADE de pe un dispozitiv pe care nu l-ai mai folosit. Dacă ai fost tu, nu trebuie să faci nimic.",
		Rows: []emailtmpl.Row{
			{Label: "Dispozitiv", Value: "Chrome pe Windows"},
			{Label: "Ora", Value: emailtmpl.FormatTime("ro", time.Date(2026, 11, 12, 9, 41, 0, 0, time.UTC)) + " (ora României)"},
			{Label: "Metoda", Value: "link din email"},
		},
		Prompt:      "Nu ai fost tu? Închide toate sesiunile: contul se deconectează de pe toate dispozitivele, inclusiv de pe acesta, apoi te conectezi din nou.",
		Buttons:     []emailtmpl.Button{{Label: "Închide toate sesiunile", URL: revokeLink, Ink: true}},
		FallbackURL: revokeLink,
		Note:        "Linkul butonului e valabil 7 zile. Cine îți poate citi emailul poate intra și în cont. Adaugă o cheie de acces în Setări › Securitate, unde găsești și adresa IP a fiecărei conectări.",
		Footer:      "Îți trimitem acest email ca să-ți protejăm contul SADE.",
	}))

	// The recipient's email, through the real notifier.
	signer, err := token.New("emailpreview-secret-0000000000000000")
	if err != nil {
		log.Fatal(err)
	}
	cm := &capture{}
	n := worker.NewEmailNotifier(cm, signer, base, 720*time.Hour)
	n.UseDetails(func(string) (worker.DeliveryInfo, error) {
		return worker.DeliveryInfo{Sender: "ana@studio-arcadia.ro", FileName: "nunta-andreea-final.mp4", MediaType: "video", SizeBytes: 248 << 20, Payments: true}, nil
	})
	if err := n.PreviewReady(context.Background(), "andreea@example.com", "prev-1"); err != nil {
		log.Fatal(err)
	}
	delivery := cm.last.HTML

	// The real message embeds the logo as a cid: inline part, which only
	// resolves inside an actual mail client. Swap in a data: URI so the
	// file opens correctly in a plain browser too.
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(emailtmpl.LogoPNG)
	for name, html := range map[string]string{
		"sign-in.html":       signIn,
		"preview-ready.html": delivery,
		"new-device.html":    newDevice,
	} {
		body := strings.ReplaceAll(html, "cid:"+emailtmpl.LogoCID, dataURI)
		if err := os.WriteFile(filepath.Join(*out, name), []byte(body), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("wrote sign-in.html, preview-ready.html and new-device.html to", *out)

	if *sendTo == "" {
		return
	}
	cfg, err := config.Load("config.yaml", ".env")
	if err != nil {
		log.Fatal(err)
	}
	m, err := mailer.New(cfg.Mailer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		log.Fatal(err)
	}
	logo := []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}}
	deliveryMsg := cm.last
	deliveryMsg.To = *sendTo
	for _, msg := range []mailer.Message{
		{To: *sendTo, Subject: "Conectare la SADE", Text: "Previzualizare: emailul de conectare.", HTML: signIn, Inline: logo},
		{To: *sendTo, Subject: "Conectare nouă în contul tău SADE", Text: "Previzualizare: alerta de conectare nouă.", HTML: newDevice, Inline: logo},
		deliveryMsg,
	} {
		if err := m.Send(context.Background(), msg); err != nil {
			log.Fatalf("send %q: %v", msg.Subject, err)
		}
		log.Printf("sent %q via %s", msg.Subject, cfg.Mailer.Transport)
	}
}

func must(s string, err error) string {
	if err != nil {
		log.Fatal(err)
	}
	return s
}
