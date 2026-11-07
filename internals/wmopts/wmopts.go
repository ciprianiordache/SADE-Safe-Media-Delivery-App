// Package wmopts is the brand snapshot stored on each job (jobs.watermark_opts):
// the operator's company, logo and watermark look at upload time. Freezing it
// on the job means changing the brand later never alters a file already in
// the queue or already sent. Written by internals/app/brand (via job.Create),
// read by the worker (to watermark), the share handler (the recipient page's
// branding) and the preview-ready email.
package wmopts

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// Opts is the snapshot. Every field is optional; zero means "use the server
// default".
type Opts struct {
	V            int    `json:"v"`
	Company      string `json:"company,omitempty"`
	LogoKey      string `json:"logoKey,omitempty"` // storage key of the logo PNG
	Position     string `json:"position,omitempty"`
	OpacityPct   int    `json:"opacityPct,omitempty"`
	LogoScalePct int    `json:"logoScalePct,omitempty"`
	TextTemplate string `json:"textTemplate,omitempty"`
}

// Version is the current snapshot format.
const Version = 1

// Encode renders o as the JSON stored on the job.
func Encode(o Opts) string {
	o.V = Version
	b, _ := json.Marshal(o)
	return string(b)
}

// Decode parses a stored snapshot. Anything unreadable - including the
// free-form JSON older jobs may carry - is an empty Opts (server defaults).
func Decode(s string) Opts {
	var o Opts
	if strings.TrimSpace(s) == "" || json.Unmarshal([]byte(s), &o) != nil {
		return Opts{}
	}
	// Only keys this server wrote can point into storage.
	if !strings.HasPrefix(o.LogoKey, "brands/") || strings.Contains(o.LogoKey, "..") {
		o.LogoKey = ""
	}
	return o
}

// DefaultTemplate is the watermark text when neither the upload nor the
// brand says otherwise.
const DefaultTemplate = "{recipient} · {date}"

// RenderText fills a text template: {company}, {recipient}, {date}. An
// empty {company} takes its separator with it, so "{company} · {recipient}"
// reads "x@y.z", not " · x@y.z".
func RenderText(template, company, recipient string, now time.Time) string {
	return render(template, strings.TrimSpace(company), recipient, now.Format("2006-01-02"))
}

// Ellipsis marks a shortened company name. Plain ASCII: the deployment's
// font may lack U+2026.
const Ellipsis = "..."

// minCompanyRunes is the shortest company name worth keeping once shortened
// (a few letters plus Ellipsis); below it the company is dropped instead.
const minCompanyRunes = 3

// RenderTextFit is RenderText for a line that holds at most budget
// characters (see ffmpeg.FitText). Too-long text gives up, in order: the
// end of the company name (marked with Ellipsis), the company altogether,
// then the date. The recipient is never cut - it is what ties a leaked file
// to the person who received it; if it alone is still too long, the caller
// shrinks the font instead. budget <= 0 means no limit.
func RenderTextFit(template, company, recipient string, now time.Time, budget int) string {
	company = strings.TrimSpace(company)
	date := now.Format("2006-01-02")
	full := render(template, company, recipient, date)
	if budget <= 0 || runes(full) <= budget {
		return full
	}
	if company != "" {
		without := render(template, "", recipient, date)
		if runes(without) < runes(full) { // the template does show the company
			for n := runes(company) - 1; n >= minCompanyRunes; n-- {
				short := strings.TrimSpace(string([]rune(company)[:n])) + Ellipsis
				if t := render(template, short, recipient, date); runes(t) <= budget {
					return t
				}
			}
			if runes(without) <= budget {
				return without
			}
			full, company = without, ""
		}
	}
	if t := render(template, company, recipient, ""); runes(t) < runes(full) {
		return t
	}
	return full
}

// render fills the template; an empty company or date is removed together
// with the separator next to it.
func render(template, company, recipient, date string) string {
	t := strings.TrimSpace(template)
	if t == "" {
		t = DefaultTemplate
	}
	if company == "" {
		t = dropPlaceholder(t, "{company}")
	}
	if date == "" {
		t = dropPlaceholder(t, "{date}")
	}
	t = strings.ReplaceAll(t, "{company}", company)
	t = strings.ReplaceAll(t, "{recipient}", recipient)
	t = strings.ReplaceAll(t, "{date}", date)
	return strings.TrimSpace(t)
}

func dropPlaceholder(t, ph string) string {
	for _, sep := range []string{ph + " · ", " · " + ph, ph + " - ", " - " + ph, ph} {
		t = strings.ReplaceAll(t, sep, "")
	}
	return t
}

func runes(s string) int { return utf8.RuneCountInString(s) }
