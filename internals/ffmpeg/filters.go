package ffmpeg

import (
	"fmt"
	"strings"
)

func needsLogo(overlay string) bool { return overlay == OverlayLogo || overlay == OverlayBoth }
func needsText(overlay string) bool { return overlay == OverlayText || overlay == OverlayBoth }

// visualFilter builds the -filter_complex graph for a video/image watermark
// and returns it together with the label of the final video pad ("[vout]").
// The source video is [0:v]; the logo, when present, is always input [1:v].
func (e *Engine) visualFilter(overlay, text string) (graph, outLabel string) {
	switch {
	case needsLogo(overlay) && needsText(overlay):
		return strings.Join([]string{
			e.logoChain(),
			fmt.Sprintf("[0:v][wm]overlay=%s[tmp]", overlayXY(e.cfg.Position, e.cfg.MarginPx)),
			fmt.Sprintf("[tmp]%s[vout]", e.drawtext(text)),
		}, ";"), "[vout]"

	case needsText(overlay):
		return fmt.Sprintf("[0:v]%s[vout]", e.drawtext(text)), "[vout]"

	default: // logo only
		return strings.Join([]string{
			e.logoChain(),
			fmt.Sprintf("[0:v][wm]overlay=%s[vout]", overlayXY(e.cfg.Position, e.cfg.MarginPx)),
		}, ";"), "[vout]"
	}
}

// logoChain scales the logo, gives it an alpha channel and applies the
// configured opacity, leaving it on pad [wm].
func (e *Engine) logoChain() string {
	opacity := float64(clamp(e.cfg.OpacityPct, 0, 100)) / 100
	width := e.cfg.LogoWidthPx
	if width <= 0 {
		width = -1 // keep native width
	}
	return fmt.Sprintf("[1:v]scale=%d:-1,format=rgba,colorchannelmixer=aa=%.2f[wm]", width, opacity)
}

// drawtext renders one drawtext filter, positioned like the logo.
func (e *Engine) drawtext(text string) string {
	return fmt.Sprintf(
		"drawtext=fontfile='%s':text='%s':fontcolor=white@0.9:fontsize=%d:"+
			"box=1:boxcolor=black@0.35:boxborderw=8:%s",
		escapeFontPath(e.cfg.FontPath),
		escapeDrawtextText(text),
		e.cfg.FontSizePx,
		drawtextXY(e.cfg.Position, e.cfg.MarginPx),
	)
}

// overlayXY maps a position name + margin to an ffmpeg overlay x:y expression
// (W/H are the base frame, w/h the overlay).
func overlayXY(pos string, margin int) string {
	m := itoa(margin)
	switch pos {
	case "top-left":
		return m + ":" + m
	case "top-right":
		return "W-w-" + m + ":" + m
	case "bottom-left":
		return m + ":H-h-" + m
	case "center":
		return "(W-w)/2:(H-h)/2"
	case "tiled":
		// TODO: real tiling; for now pin to the corner.
		return "W-w-" + m + ":H-h-" + m
	default: // bottom-right
		return "W-w-" + m + ":H-h-" + m
	}
}

// drawtextXY maps a position name + margin to drawtext x=/y= expressions
// (w/h are the frame, text_w/text_h the rendered text box).
func drawtextXY(pos string, margin int) string {
	m := itoa(margin)
	switch pos {
	case "top-left":
		return "x=" + m + ":y=" + m
	case "top-right":
		return "x=w-text_w-" + m + ":y=" + m
	case "bottom-left":
		return "x=" + m + ":y=h-text_h-" + m
	case "center":
		return "x=(w-text_w)/2:y=(h-text_h)/2"
	default: // bottom-right / tiled
		return "x=w-text_w-" + m + ":y=h-text_h-" + m
	}
}

// escapeDrawtextText escapes a user string for use inside a single-quoted
// drawtext text= value in a filtergraph (which is passed as one exec arg, so
// only ffmpeg's parser matters, not a shell).
func escapeDrawtextText(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`%`, `\%`,
		"\n", ` `,
		"\r", ``,
	)
	return r.Replace(s)
}

// escapeFontPath normalises a font path for a filtergraph: forward slashes,
// and the Windows drive colon escaped so ffmpeg does not read it as an option
// separator.
func escapeFontPath(p string) string {
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.ReplaceAll(p, `:`, `\:`)
	return p
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
