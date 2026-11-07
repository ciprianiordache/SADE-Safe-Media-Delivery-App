package ffmpeg

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStyleTiledLogoAndText(t *testing.T) {
	e := testEngine()
	args, err := e.buildArgs(Request{
		SourcePath: "in.jpg", OutputPath: "out.jpg", Media: MediaImage, Overlay: OverlayBoth, textFile: "t.txt",
		Style: Style{LogoPath: "/brands/u1/logo.png", Position: "tiled", OpacityPct: 50, LogoScalePct: 10, FrameWidth: 2000},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-i /brands/u1/logo.png") {
		t.Errorf("brand logo not used as input: %s", joined)
	}
	fc := args[indexOf(args, "-filter_complex")+1]
	if n := strings.Count(fc, "overlay="); n != 9 {
		t.Errorf("tiled logo overlays = %d, want 9: %s", n, fc)
	}
	if n := strings.Count(fc, "drawtext="); n != 4 {
		t.Errorf("tiled text copies = %d, want 4", n)
	}
	if !strings.Contains(fc, "scale=200:-1") { // 10% of 2000px
		t.Errorf("logo not scaled to 10%% of the frame: %s", fc)
	}
	if !strings.Contains(fc, "aa=0.50") {
		t.Errorf("brand opacity not applied: %s", fc)
	}
	if !strings.Contains(fc, "fontsize=44") { // 2.2% of 2000px
		t.Errorf("text not scaled with the frame: %s", fc)
	}
	if !strings.HasSuffix(fc, "[vout]") {
		t.Errorf("graph must end on [vout]: %s", fc)
	}
}

func TestStyleLogoOnlyCorner(t *testing.T) {
	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.mp4", OutputPath: "out.mp4", Media: MediaVideo, Overlay: OverlayLogo,
		Style: Style{Position: "top-left", FrameWidth: 1000},
	})
	fc := args[indexOf(args, "-filter_complex")+1]
	if !strings.Contains(fc, "overlay=24:24[vout]") {
		t.Errorf("top-left logo-only graph = %s", fc)
	}
	if strings.Contains(fc, "drawtext") {
		t.Errorf("logo-only graph has text: %s", fc)
	}
}

func TestStyleUnknownPositionFallsBack(t *testing.T) {
	l := testEngine().look(Style{Position: "; rm -rf /"})
	if l.position != testEngine().cfg.Position {
		t.Errorf("position = %q, want the configured default", l.position)
	}
}

func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func TestStyleTextClearsTheLogo(t *testing.T) {
	logo := filepath.Join(t.TempDir(), "logo.png")
	f, _ := os.Create(logo)
	_ = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 200, 100)))
	f.Close()

	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.jpg", OutputPath: "out.jpg", Media: MediaImage, Overlay: OverlayBoth, textFile: "t.txt",
		Style: Style{LogoPath: logo, Position: "bottom-right", LogoScalePct: 20, FrameWidth: 1000},
	})
	fc := args[indexOf(args, "-filter_complex")+1]
	// Logo 200px wide -> 100px tall; margin 24 -> text 100+12 above it.
	if !strings.Contains(fc, "y=h-text_h-24-112") {
		t.Errorf("text not lifted above the logo: %s", fc)
	}
}
