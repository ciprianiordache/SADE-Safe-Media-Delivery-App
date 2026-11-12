// Package emailtmpl renders SADE's branded transactional emails. It is a
// thin, dependency-free HTML layer shared by internals/app/auth (sign-in,
// new sign-in alert) and internals/worker (the recipient's delivery email) -
// both need the same chrome, so it lives here rather than duplicated in each
// domain, matching the shared-helper pattern of internals/httpx and
// internals/token. Every dynamic value passes through html/template's
// contextual auto-escaping; callers hand it plain data, never pre-built HTML.
//
// The look follows the app (frontend/src/app.css, DESIGN.md): Reddit Sans,
// ink #211c18 on warm paper #fbf8f4, headings at 500, pill buttons (#111 on
// the orange, never white) and hairline rows, on one white 12px card. The
// app's window bar (three dots and a crumb) stays out: in an inbox it is
// chrome from an app the recipient has never seen, and Outlook drops it.
// Email clients strip external stylesheets and most JS, so the tokens are
// inlined as hex values here, and the layout is table-based for Outlook's
// Word engine and fluid (max 560px) for phones. Web fonts load only in the
// clients that allow them; the fallback stack (Segoe UI, Roboto) is close in
// shape. Unlike the app, the email does not follow the reader's OS theme:
// mail-client dark modes invert colours inconsistently, so every email ships
// one light design, declared via <meta name="color-scheme" content="light">.
package emailtmpl

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"math"
	"strings"
	"time"
	_ "time/tzdata" // local times in emails work on servers without zoneinfo
	"unicode/utf8"
)

// LogoPNG is SADE's mark, embedded in the binary and sent as an inline
// (Content-ID) part rather than referenced by URL - see LogoCID. A remote
// URL would have to be fetched by the recipient's mail provider (Gmail and
// most webmail proxy every remote image through their own servers), which
// fails outright while SADE runs on a private/LAN address with no public
// domain; an inline part has no such dependency, in dev or in production.
//
//go:embed logo.png
var LogoPNG []byte

// LogoCID is the Content-ID the logo is attached under; the template below
// references it as "cid:sade-logo" literally; keep the two in sync.
const LogoCID = "sade-logo"

// Button is one call to action. One per email is the norm, as in the app.
type Button struct {
	Label   string
	URL     string
	Primary bool // the orange pill with ink text: the one thing to do
	Ink     bool // the ink pill: a serious action (sign out everywhere)
	// neither: a hairline pill, for a secondary action
}

// Row is one "label  value" line under the intro, drawn as the app's
// hairline rows (the facts of a delivery, of a sign-in).
type Row struct {
	Label string
	Value string
}

// Data is what Render assembles the branded email around.
type Data struct {
	// Lang is the document language ("ro", "en"); "" means "ro", SADE's
	// first language. It also picks the shared chrome's words.
	Lang      string
	Subject   string
	Preheader string // hidden preview text most inboxes show next to the subject
	Heading   string
	Intro     string
	Rows      []Row
	// Prompt is an ink line right above the buttons saying when to press
	// them ("Nu ai fost tu? ..."), so an action never comes before its reason.
	Prompt  string
	Buttons []Button
	// ButtonNote is a small line right under the buttons: the reassurance a
	// wary reader needs at the moment they decide whether to click.
	ButtonNote string
	// Code, when set, is shown large under the buttons (the sign-in email's
	// 6-digit code, drawn as two groups of three), with CodeLabel above it.
	Code      string
	CodeLabel string
	// FallbackURL is shown as copy-paste text when a button can't be
	// clicked; empty hides the line.
	FallbackURL string
	Note        string // small print under a hairline, e.g. link expiry
	// Footer is a line under the card saying why the reader got this email;
	// empty shows only SADE's name.
	Footer string
}

// view is Data plus the chrome words for its language and the Outlook-only
// markup. html/template drops every HTML comment written in the template
// text, so the <!--[if mso]> conditionals Outlook's Word engine reads have
// to enter as trusted values instead.
type view struct {
	Data
	Lang          string
	Tagline       string
	FallbackLabel string
	PreheaderPad  template.HTML
	MSOHead       template.HTML
	MSOOpen       template.HTML
	MSOClose      template.HTML
	CodeA, CodeB  string
	Btns          []buttonView
}

type buttonView struct {
	Button
	Open  template.HTML // the VML pill for Outlook + the start of the !mso block
	Close template.HTML
}

var chrome = map[string]struct{ tagline, fallback string }{
	"ro": {"Livrare sigură de fișiere media", "Butonul nu merge? Copiază linkul în browser:"},
	"en": {"Safe media delivery", "Button not working? Paste this link into your browser:"},
}

// preheaderPad keeps inboxes from pulling the body text into the preview
// line after the preheader.
const preheaderPad = template.HTML("&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;&#847;&zwnj;&nbsp;")

const (
	msoHead = template.HTML(`<!--[if mso]><noscript><xml><o:OfficeDocumentSettings><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript>
<style>table { border-collapse: collapse; } td, a, span, h1, center { font-family: 'Segoe UI', Arial, sans-serif !important; }</style><![endif]-->`)
	// Outlook ignores max-width, so it gets a fixed 560px column.
	msoOpen   = template.HTML(`<!--[if mso]><table role="presentation" width="560" align="center" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`)
	msoClose  = template.HTML(`<!--[if mso]></td></tr></table><![endif]-->`)
	notMSOEnd = template.HTML(`<!--<![endif]-->`)
)

var tmpl = template.Must(template.New("email").Parse(layoutTmpl))

// Render returns the full HTML document for d.
func Render(d Data) (string, error) {
	lang := normLang(d.Lang)
	c := chrome[lang]
	v := view{Data: d, Lang: lang, Tagline: c.tagline, FallbackLabel: c.fallback,
		PreheaderPad: preheaderPad, MSOHead: msoHead, MSOOpen: msoOpen, MSOClose: msoClose}
	v.CodeA, v.CodeB = splitCode(d.Code)
	for _, b := range d.Buttons {
		v.Btns = append(v.Btns, buttonView{Button: b, Open: vmlButton(b), Close: notMSOEnd})
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TextFooter is the plain-text twin of the HTML footer, so a text-only
// reader is told the same things as an HTML one.
func TextFooter(lang, footer string) string {
	s := "--\nSADE · " + chrome[normLang(lang)].tagline + "\n"
	if footer != "" {
		s += footer + "\n"
	}
	return s
}

// splitCode draws a 6-digit code as two groups of three ("482 913"), easier
// to read and to hold in mind while typing. The gap is drawn, not typed, so
// a copy still pastes as "482913".
func splitCode(code string) (string, string) {
	r := []rune(code)
	if len(r) != 6 {
		return code, ""
	}
	return string(r[:3]), string(r[3:])
}

// vmlButton is a button as Outlook's Word engine can draw it. Word ignores
// padding and border-radius on a link, which turns the pill into a thin strip
// behind the text, so Outlook gets a VML rounded rectangle and every other
// client the <a> that follows inside <!--[if !mso]>.
func vmlButton(b Button) template.HTML {
	fill, text, stroke := "#ffffff", "#211c18", `strokecolor="#d6cdc0" strokeweight="1px"`
	switch {
	case b.Primary:
		fill, text, stroke = "#e8631c", "#111111", `stroke="f"`
	case b.Ink:
		fill, text, stroke = "#211c18", "#fbf8f4", `stroke="f"`
	}
	// Segoe UI at 16px runs about 9px a character; the pill adds 28px a side.
	width := utf8.RuneCountInString(b.Label)*9 + 56
	return template.HTML(fmt.Sprintf(`<!--[if mso]><v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" href="%s" style="height:46px;v-text-anchor:middle;width:%dpx;" arcsize="50%%" %s fillcolor="%s"><w:anchorlock/><center style="color:%s;font-family:'Segoe UI',Arial,sans-serif;font-size:16px;font-weight:500;">%s</center></v:roundrect><![endif]--><!--[if !mso]><!-->`,
		html.EscapeString(b.URL), width, stroke, fill, text, html.EscapeString(b.Label)))
}

func normLang(l string) string {
	if strings.HasPrefix(strings.ToLower(l), "en") {
		return "en"
	}
	return "ro"
}

// HumanDuration renders d the way an English email reads it out loud
// ("15 minutes", "30 days") instead of Go's compact "15m0s"/"720h0m0s".
func HumanDuration(d time.Duration) string { return HumanDurationIn("en", d) }

// HumanDurationIn is HumanDuration in a language: "15 minute", "30 de zile"
// in Romanian. It rounds to the coarsest whole unit that still means
// something at email-copy precision.
func HumanDurationIn(lang string, d time.Duration) string {
	d = d.Round(time.Minute)
	var n int
	var u string
	switch {
	case d < time.Hour:
		n, u = int(d.Minutes()), "minute"
	case d < 24*time.Hour:
		n, u = int(math.Round(d.Hours())), "hour"
	default:
		n, u = int(math.Round(d.Hours()/24)), "day"
	}
	if normLang(lang) == "en" {
		if n == 1 {
			return fmt.Sprintf("1 %s", u)
		}
		return fmt.Sprintf("%d %ss", n, u)
	}
	ro := map[string][2]string{"minute": {"minut", "minute"}, "hour": {"oră", "ore"}, "day": {"zi", "zile"}}[u]
	if n == 1 {
		return "1 " + ro[0]
	}
	// Romanian puts "de" between a number from 20 up (or a round hundred)
	// and its noun: "15 minute", "24 de ore", "30 de zile".
	if r := n % 100; r == 0 || r >= 20 {
		return fmt.Sprintf("%d de %s", n, ro[1])
	}
	return fmt.Sprintf("%d %s", n, ro[1])
}

// bucharest is where SADE's operators are (Romania first); emails give
// times in it, never bare UTC.
var bucharest = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Bucharest"); err == nil {
		return loc
	}
	return time.UTC
}()

var roMonths = [...]string{"ian.", "feb.", "mar.", "apr.", "mai", "iun.", "iul.", "aug.", "sept.", "oct.", "nov.", "dec."}

// FormatTime is t as a reader in Romania expects it: "12 nov. 2026, 11:41"
// ("12 Nov 2026, 11:41" in English), in Bucharest time.
func FormatTime(lang string, t time.Time) string {
	t = t.In(bucharest)
	if normLang(lang) == "en" {
		return t.Format("2 Jan 2006, 15:04")
	}
	return fmt.Sprintf("%d %s %d, %s", t.Day(), roMonths[t.Month()-1], t.Year(), t.Format("15:04"))
}

// The palette, from app.css's light tokens: paper #fbf8f4, surface #ffffff,
// ink #211c18, muted #544b42 (8:1 on white), hint #6e655b (5:1 on paper),
// orange #e8631c with #111111 text (5.6:1), hairlines #ebe5dd, the code box
// #f7f4f0.
const layoutTmpl = `<!doctype html>
<html lang="{{.Lang}}" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>{{.Subject}}</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Reddit+Sans:wght@400;500&family=JetBrains+Mono:wght@500&display=swap">
{{.MSOHead}}
<style>
  body, table, td, a { -webkit-text-size-adjust: 100%; -ms-text-size-adjust: 100%; }
  table, td { mso-table-lspace: 0pt; mso-table-rspace: 0pt; }
  img { -ms-interpolation-mode: bicubic; border: 0; height: auto; line-height: 100%; outline: none; text-decoration: none; }
  body { margin: 0; padding: 0; width: 100% !important; background-color: #fbf8f4; }
  a { color: #b8481a; }
  @media screen and (max-width: 600px) {
    .sade-outer { padding: 20px 10px 32px !important; }
    .sade-pad { padding: 26px 22px 28px !important; }
    .sade-h1 { font-size: 22px !important; }
    .sade-btn-table { width: 100% !important; }
    .sade-btn-cell { display: block !important; width: 100% !important; padding: 0 0 10px 0 !important; }
    .sade-btn { display: block !important; text-align: center !important; }
    .sade-row-label { display: block !important; width: auto !important; padding: 10px 0 0 0 !important; border-top: 1px solid #ebe5dd !important; }
    .sade-row-value { display: block !important; padding: 2px 0 10px 0 !important; border-top: none !important; }
  }
</style>
</head>
<body style="margin:0;padding:0;background-color:#fbf8f4;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;">{{.Preheader}}{{.PreheaderPad}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#fbf8f4;">
<tr><td align="center" class="sade-outer" style="padding:40px 16px 48px;">
{{.MSOOpen}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px;">

<tr><td style="padding:0 2px 16px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="vertical-align:middle;padding-right:9px;"><img src="cid:sade-logo" width="26" height="26" alt="" style="display:block;border-radius:7px;"></td>
<td style="vertical-align:middle;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:17px;line-height:1;font-weight:500;letter-spacing:-0.02em;color:#211c18;">SADE</td>
</tr></table>
</td></tr>

<tr><td style="background-color:#ffffff;border:1px solid #e7e0d6;border-radius:12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td class="sade-pad" style="padding:34px 36px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="padding-bottom:12px;"><h1 class="sade-h1" style="margin:0;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:25px;line-height:1.2;font-weight:500;letter-spacing:-0.025em;color:#211c18;word-break:break-word;overflow-wrap:anywhere;">{{.Heading}}</h1></td></tr>
{{if .Intro}}<tr><td style="font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:16px;line-height:1.6;color:#544b42;padding-bottom:24px;">{{.Intro}}</td></tr>{{end}}

{{if .Rows}}<tr><td style="padding-bottom:26px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-bottom:1px solid #ebe5dd;">
{{range .Rows}}<tr>
<td class="sade-row-label" width="34%" style="width:34%;border-top:1px solid #ebe5dd;padding:11px 12px 11px 0;vertical-align:top;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:14px;line-height:1.45;color:#544b42;">{{.Label}}</td>
<td class="sade-row-value" style="border-top:1px solid #ebe5dd;padding:11px 0;vertical-align:top;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:14px;line-height:1.45;color:#211c18;word-break:break-word;">{{.Value}}</td>
</tr>{{end}}
</table>
</td></tr>{{end}}

{{if .Prompt}}<tr><td style="padding-bottom:16px;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:15px;line-height:1.55;color:#211c18;">{{.Prompt}}</td></tr>{{end}}

{{if .Btns}}<tr><td>
<table role="presentation" cellpadding="0" cellspacing="0" border="0" class="sade-btn-table"><tr>
{{range .Btns}}<td class="sade-btn-cell" style="padding-right:10px;">{{.Open}}{{if .Primary}}<a class="sade-btn" href="{{.URL}}" style="display:inline-block;background-color:#e8631c;color:#111111;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:16px;line-height:20px;font-weight:500;text-decoration:none;padding:13px 26px;border-radius:999px;">{{.Label}}</a>{{else if .Ink}}<a class="sade-btn" href="{{.URL}}" style="display:inline-block;background-color:#211c18;color:#fbf8f4;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:16px;line-height:20px;font-weight:500;text-decoration:none;padding:13px 26px;border-radius:999px;">{{.Label}}</a>{{else}}<a class="sade-btn" href="{{.URL}}" style="display:inline-block;background-color:#ffffff;color:#211c18;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:16px;line-height:20px;font-weight:500;text-decoration:none;padding:12px 25px;border-radius:999px;border:1px solid #d6cdc0;">{{.Label}}</a>{{end}}{{.Close}}</td>{{end}}
</tr></table>
</td></tr>{{end}}

{{if .ButtonNote}}<tr><td style="padding-top:12px;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:13px;line-height:1.55;color:#544b42;">{{.ButtonNote}}</td></tr>{{end}}

{{if .Code}}<tr><td style="padding-top:28px;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:14px;line-height:1.55;color:#544b42;">{{.CodeLabel}}</td></tr>
<tr><td style="padding-top:10px;"><span style="display:inline-block;font-family:'JetBrains Mono',ui-monospace,'SFMono-Regular',Consolas,Menlo,monospace;font-size:28px;line-height:1;font-weight:500;letter-spacing:3px;color:#211c18;background-color:#f7f4f0;border:1px solid #ebe5dd;border-radius:8px;padding:12px 15px 12px 18px;">{{.CodeA}}{{if .CodeB}}<span style="display:inline-block;width:12px;"></span>{{.CodeB}}{{end}}</span></td></tr>{{end}}

{{if .FallbackURL}}<tr><td style="padding-top:24px;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:13px;line-height:1.6;color:#544b42;">{{.FallbackLabel}}<br><a href="{{.FallbackURL}}" style="font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:12px;color:#544b42;text-decoration:underline;word-break:break-all;">{{.FallbackURL}}</a></td></tr>{{end}}

{{if .Note}}<tr><td style="padding-top:26px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="border-top:1px solid #ebe5dd;padding-top:16px;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:13px;line-height:1.6;color:#544b42;">{{.Note}}</td>
</tr></table>
</td></tr>{{end}}
</table>
</td></tr>
</table>
</td></tr>

<tr><td style="padding:22px 4px 0;font-family:'Reddit Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;font-size:12px;line-height:1.7;color:#6e655b;">
SADE &middot; {{.Tagline}}{{if .Footer}}<br>{{.Footer}}{{end}}
</td></tr>
</table>
{{.MSOClose}}
</td></tr>
</table>
</body>
</html>
`
