package ffmpeg

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sade/config"
)

// The shared examples also document BrandPreview.svelte's numbers: a change
// here that breaks a row must be mirrored there.
func TestFitTextFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "textfit.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name       string `json:"name"`
		FrameW     int    `json:"frameW"`
		Pos        string `json:"pos"`
		BasePx     int    `json:"basePx"`
		Chars      int    `json:"chars"`
		WantPx     int    `json:"wantPx"`
		WantBudget int    `json:"wantBudget"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no fixture rows")
	}
	for _, r := range rows {
		fit := FitText(r.FrameW, r.Pos, r.BasePx)
		if got := fit.FontPx(r.Chars); got != r.WantPx {
			t.Errorf("%s: FontPx(%d) = %d, want %d", r.Name, r.Chars, got, r.WantPx)
		}
		if got := fit.Budget(); got != r.WantBudget {
			t.Errorf("%s: Budget() = %d, want %d", r.Name, got, r.WantBudget)
		}
	}
}

// fitText asks the caller for text within the budget and sizes the font to
// it; text that fits keeps exactly the frame's normal size.
func TestFitTextSizesTheFont(t *testing.T) {
	e := testEngine()
	var asked int
	long := strings.Repeat("x", 115)
	req := Request{
		Style:   Style{FrameWidth: 1920, Position: "bottom-right"},
		TextFor: func(budget int) string { asked = budget; return long },
	}
	if got := e.fitText(&req); got != long {
		t.Fatalf("text = %q", got)
	}
	if asked != 110 || req.Style.FontPx != 24 {
		t.Errorf("budget = %d, font = %d; want 110, 24", asked, req.Style.FontPx)
	}

	short := Request{Style: Style{FrameWidth: 1920, Position: "bottom-right"}, Text: "client@example.com · 2026-11-07"}
	e.fitText(&short)
	if short.Style.FontPx != 42 { // 2.2% of 1920: unchanged from before fitting existed
		t.Errorf("short text font = %d, want the unchanged 42", short.Style.FontPx)
	}

	// The fitted size reaches the graph.
	short.textFile, short.SourcePath, short.OutputPath, short.Media, short.Overlay = "t.txt", "in.jpg", "out.jpg", MediaImage, OverlayText
	req.textFile, req.SourcePath, req.OutputPath, req.Media, req.Overlay = "t.txt", "in.jpg", "out.jpg", MediaImage, OverlayText
	args, err := e.buildArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	if fc := args[indexOf(args, "-filter_complex")+1]; !strings.Contains(fc, "fontsize=24") {
		t.Errorf("fitted size not in the graph: %s", fc)
	}
}

func TestQuarterTurn(t *testing.T) {
	for deg, want := range map[float64]bool{0: false, 90: true, -90: true, 180: false, 270: true, -270: true, 89.6: true} {
		if got := quarterTurn(deg); got != want {
			t.Errorf("quarterTurn(%v) = %v, want %v", deg, got, want)
		}
	}
	side := []struct {
		Rotation float64 `json:"rotation"`
	}{{Rotation: -90}}
	if got := rotationOf("", side); got != -90 {
		t.Errorf("display matrix rotation = %v", got)
	}
	if got := rotationOf("90", nil); got != 90 {
		t.Errorf("legacy rotate tag = %v", got)
	}
}

// A phone video stored landscape with a 90 degree rotation is rendered
// portrait: Probe must report the width ffmpeg will actually draw on.
func TestProbeRotatedVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	plain, rot := filepath.Join(dir, "plain.mp4"), filepath.Join(dir, "rot.mp4")
	for _, args := range [][]string{
		{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=320x180:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", plain},
		{"-hide_banner", "-loglevel", "error", "-y", "-display_rotation", "90", "-i", plain, "-c", "copy", rot},
	} {
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("make rotated video: %v: %s", err, out)
		}
	}
	e, err := New(config.Defaults().FFmpeg, testEngine().log)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.Probe(context.Background(), rot)
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 180 || res.Height != 320 {
		t.Errorf("rotated video = %dx%d, want 180x320 (as rendered)", res.Width, res.Height)
	}
}
