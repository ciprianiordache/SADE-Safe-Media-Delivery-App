// Package emailtmpl renders SADE's branded transactional emails. It is a
// thin, dependency-free HTML layer shared by internals/app/auth (the
// magic-link email) and internals/worker (the preview-ready email) - both
// need the same card/button chrome, so it lives here rather than duplicated
// in each domain, matching the shared-helper pattern of internals/httpx and
// internals/token. Every dynamic value passes through html/template's
// contextual auto-escaping; callers hand it plain data, never pre-built
// HTML fragments.
//
// The palette, radius and font stack mirror frontend/src/app.css's design
// tokens exactly (warm parchment ground, one orange accent, IBM Plex Sans) -
// email clients strip external stylesheets and most JS, so the tokens are
// inlined here rather than shared via CSS custom properties, and the layout
// is table-based for Outlook's Word rendering engine. Unlike the app itself,
// the email deliberately does not follow the viewer's OS theme: mail-client
// dark-mode support is inconsistent and often auto-inverts colors in ways
// that clash with a hand-tuned palette, so every email ships one light,
// self-contained design (declared via <meta name="color-scheme" content="light">)
// rather than a broken half-dark one.
package emailtmpl

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"time"
)

// Button is one call to action in an email body.
type Button struct {
	Label   string
	URL     string
	Primary bool // solid accent fill vs. an outlined secondary style
}

// Data is what Render assembles the branded shell around.
type Data struct {
	PublicURL   string // config.App.PublicURL; the logo is served from here at /logo.png
	Subject     string
	Preheader   string // hidden preview text most inboxes show next to the subject
	Heading     string
	Intro       string
	Buttons     []Button
	FallbackURL string // shown as copy-paste text under the buttons when a button can't be clicked; empty hides the line
	Note        string // small print under everything, e.g. link expiry
}

var tmpl = template.Must(template.New("email").Parse(layoutTmpl))

// Render returns the full HTML document for d.
func Render(d Data) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// HumanDuration renders d the way an email reads it out loud ("15 minutes",
// "30 days") instead of Go's compact "15m0s"/"720h0m0s". It rounds to the
// coarsest whole unit that still means something at email-copy precision.
func HumanDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	switch {
	case d < time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return unit(int(math.Round(d.Hours())), "hour")
	default:
		return unit(int(math.Round(d.Hours()/24)), "day")
	}
}

func unit(n int, name string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", name)
	}
	return fmt.Sprintf("%d %ss", n, name)
}

const layoutTmpl = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>{{.Subject}}</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;600;700&family=IBM+Plex+Mono:wght@500&display=swap">
<!--[if mso]>
<style>table { border-collapse: collapse; }</style>
<![endif]-->
<style>
  body, table, td, a { -webkit-text-size-adjust: 100%; -ms-text-size-adjust: 100%; }
  table, td { mso-table-lspace: 0pt; mso-table-rspace: 0pt; }
  img { -ms-interpolation-mode: bicubic; border: 0; height: auto; line-height: 100%; outline: none; text-decoration: none; }
  body { margin: 0; padding: 0; width: 100% !important; background-color: #fbf8f4; }
  a { color: #e8631c; }
  @media screen and (max-width: 600px) {
    .sade-card { border-radius: 0 !important; border-left: none !important; border-right: none !important; }
    .sade-pad { padding-left: 24px !important; padding-right: 24px !important; }
  }
</style>
</head>
<body style="margin:0;padding:0;background-color:#fbf8f4;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;">{{.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#fbf8f4;">
<tr><td align="center" style="padding:48px 16px;">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:560px;max-width:560px;">
<tr><td align="center" style="padding-bottom:28px;">
<img src="{{.PublicURL}}/logo.png" width="44" height="44" alt="SADE" style="display:block;border-radius:10px;">
<div style="font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;font-weight:600;letter-spacing:.06em;color:#9c9287;text-transform:uppercase;margin-top:10px;">Safe Media Delivery</div>
</td></tr>
<tr><td class="sade-card sade-pad" style="background-color:#ffffff;border:1px solid #ebe3d7;border-radius:14px;padding:40px 44px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:22px;line-height:1.3;font-weight:700;color:#211c18;padding-bottom:12px;">{{.Heading}}</td></tr>
<tr><td style="font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#6e655b;padding-bottom:28px;">{{.Intro}}</td></tr>
<tr><td>
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
{{range .Buttons}}<td style="padding-right:12px;">{{if .Primary}}<a href="{{.URL}}" style="display:inline-block;background-color:#e8631c;color:#ffffff;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:15px;font-weight:600;text-decoration:none;padding:13px 26px;border-radius:9px;">{{.Label}}</a>{{else}}<a href="{{.URL}}" style="display:inline-block;background-color:#ffffff;color:#211c18;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:15px;font-weight:600;text-decoration:none;padding:12px 25px;border-radius:9px;border:1px solid #ddd3c4;">{{.Label}}</a>{{end}}</td>{{end}}
</tr></table>
</td></tr>
{{if .FallbackURL}}<tr><td style="padding-top:24px;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;line-height:1.6;color:#9c9287;">Button not working? Paste this link into your browser:<br><span style="font-family:'IBM Plex Mono',ui-monospace,'SFMono-Regular',Menlo,monospace;font-size:12px;color:#a8480f;word-break:break-all;">{{.FallbackURL}}</span></td></tr>{{end}}
{{if .Note}}<tr><td style="padding-top:24px;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;color:#9c9287;border-top:1px solid #ebe3d7;margin-top:24px;">{{.Note}}</td></tr>{{end}}
</table>
</td></tr>
<tr><td align="center" style="padding-top:24px;font-family:'IBM Plex Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.7;color:#b4a997;">SADE &mdash; Safe Media Delivery<br>If you weren't expecting this email, you can ignore it.</td></tr>
</table>
</td></tr>
</table>
</body>
</html>
`
