// Command emailpreview renders SADE's transactional email templates with
// sample data to local HTML files, for eyeballing changes to
// internals/emailtmpl without sending real mail. Not wired into the app;
// run manually with `go run ./cmd/emailpreview -out DIR`.
package main

import (
	"encoding/base64"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"sade/internals/emailtmpl"
)

func main() {
	out := flag.String("out", ".", "directory to write the rendered .html files into")
	flag.Parse()

	magicLink, err := emailtmpl.Render(emailtmpl.Data{
		Subject:     "Sign in to SADE",
		Preheader:   "Your sign-in link - valid for 15 minutes",
		Heading:     "Sign in to SADE",
		Intro:       "Click the button below to sign in. This link is valid for 15 minutes and can be used once.",
		Buttons:     []emailtmpl.Button{{Label: "Sign in", URL: "http://192.168.1.179:8080/api/auth/callback?token=UixDTPmKvHFjIGavva_Md3c3GtYzDrwUT69fs9W1sJQ", Primary: true}},
		FallbackURL: "http://192.168.1.179:8080/api/auth/callback?token=UixDTPmKvHFjIGavva_Md3c3GtYzDrwUT69fs9W1sJQ",
	})
	if err != nil {
		log.Fatal(err)
	}

	previewReady, err := emailtmpl.Render(emailtmpl.Data{
		Subject:   "Your preview is ready",
		Preheader: "Your watermarked preview is ready to view",
		Heading:   "Your preview is ready",
		Intro:     "Someone shared a watermarked media preview with you through SADE. View it in your browser, or download it directly.",
		Buttons: []emailtmpl.Button{
			{Label: "View preview", URL: "http://192.168.1.179:8080/preview/cHJldmlldy5hN2NlZDU4Ni04MzE4LTRhN2EtOTQ2Ni0zMDViYjM3ZWQ3MWUuMTc5MjE4MTI4Mg", Primary: true},
			{Label: "Download", URL: "http://192.168.1.179:8080/d/ZG93bmxvYWQuNTVmNDI5YmItNzAzMi00ZjkyLThlZDEtMDAyMDU1MTBhNjUy"},
		},
		Note: "These links are valid for 30 days.",
	})
	if err != nil {
		log.Fatal(err)
	}

	// The real message embeds the logo as a cid: inline part, which only
	// resolves inside an actual mail client. Swap in a data: URI so the
	// file opens correctly in a plain browser too.
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(emailtmpl.LogoPNG)
	forBrowser := func(html string) []byte {
		return []byte(strings.ReplaceAll(html, "cid:"+emailtmpl.LogoCID, dataURI))
	}

	if err := os.WriteFile(filepath.Join(*out, "magic-link.html"), forBrowser(magicLink), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "preview-ready.html"), forBrowser(previewReady), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote magic-link.html and preview-ready.html to", *out)
}
