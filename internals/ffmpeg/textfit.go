package ffmpeg

import "unicode/utf8"

// Text fitting: the watermark text must stay inside the picture. ffmpeg's
// drawtext cannot size a font from the text's own width (text_w depends on
// the font size), so the size is computed here from an estimated glyph
// width: CharWidth x font size per character. Estimated, not measured -
// deliberately a little wide so wide glyphs and capitals still fit with the
// deployment's font.
//
// The same numbers drive the browser preview (frontend
// src/lib/components/BrandPreview.svelte); the shared examples live in
// testdata/textfit.json. Change both sides together.
const (
	// CharWidth is the estimated advance of one character, as a fraction
	// of the font size.
	CharWidth = 0.62
	// fitCorner / fitTiled are the share of the frame width one line of
	// text may use: one line at a corner or the centre, or one of the
	// copies of a tiled watermark (which sit a third of the width apart).
	fitCorner = 0.90
	fitTiled  = 0.30
	// minScale is how far the font may shrink (as a fraction of its normal
	// size) before the text itself is shortened.
	minScale = 0.6
	// absMinPx is the smallest font ever used - only reached when the
	// recipient alone is too long for the frame.
	absMinPx = 6
	// boxBorderPx is drawtext's boxborderw: the text box is this much wider
	// on each side.
	boxBorderPx = 8
	// FallbackFrameWidth is assumed when ffprobe reports no width.
	FallbackFrameWidth = 1280
)

// TextFit is how much text fits on one line of a frame.
type TextFit struct {
	BasePx  int // the normal font size for this frame
	MinPx   int // the smallest size before the text is shortened
	AvailPx int // usable line width, box border included
}

// FitText computes the text room of a frameW-wide picture with the watermark
// at pos. basePx is the frame's normal font size (see look).
func FitText(frameW int, pos string, basePx int) TextFit {
	if frameW <= 0 {
		frameW = FallbackFrameWidth
	}
	frac := fitCorner
	if pos == "tiled" {
		frac = fitTiled
	}
	return TextFit{
		BasePx:  basePx,
		MinPx:   max(absMinPx, int(float64(basePx)*minScale)),
		AvailPx: max(1, int(float64(frameW)*frac)-2*boxBorderPx),
	}
}

// Budget is how many characters fit at MinPx: text longer than this has to
// be shortened (see wmopts.RenderTextFit).
func (f TextFit) Budget() int {
	return max(1, int(float64(f.AvailPx)/(CharWidth*float64(f.MinPx))))
}

// FontPx is the font size for text of n characters: the normal size when it
// fits, otherwise as large as fits, never below absMinPx. Text that already
// fits keeps exactly BasePx - the fit only ever shrinks.
func (f TextFit) FontPx(n int) int {
	if n <= 0 {
		return f.BasePx
	}
	px := int(float64(f.AvailPx) / (CharWidth * float64(n)))
	return clamp(px, absMinPx, f.BasePx)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// fitText settles the text and its size for req: it asks req.TextFor for
// text within the budget (when set), sanitises it, and stores the fitted
// font size in req.Style.FontPx. Returns the text to burn in.
func (e *Engine) fitText(req *Request) string {
	l := e.look(req.Style)
	fit := FitText(req.Style.FrameWidth, l.position, l.fontSize)
	text := req.Text
	if req.TextFor != nil {
		text = req.TextFor(fit.Budget())
	}
	text = SanitizeText(text)
	req.Style.FontPx = fit.FontPx(runeLen(text))
	return text
}
