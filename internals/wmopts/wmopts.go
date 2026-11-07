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
	t := strings.TrimSpace(template)
	if t == "" {
		t = DefaultTemplate
	}
	if strings.TrimSpace(company) == "" {
		for _, sep := range []string{"{company} · ", " · {company}", "{company} - ", " - {company}", "{company}"} {
			t = strings.ReplaceAll(t, sep, "")
		}
	}
	t = strings.ReplaceAll(t, "{company}", strings.TrimSpace(company))
	t = strings.ReplaceAll(t, "{recipient}", recipient)
	t = strings.ReplaceAll(t, "{date}", now.Format("2006-01-02"))
	return strings.TrimSpace(t)
}
