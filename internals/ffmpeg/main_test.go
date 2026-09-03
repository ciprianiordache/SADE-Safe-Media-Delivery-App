package ffmpeg

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sade/config"
)

func testEngine() *Engine {
	return &Engine{
		cfg: config.FFmpegConfig{
			LogoPath:           "/assets/logo.png",
			AudioWatermarkPath: "/assets/wm.mp3",
			FontPath:           `C:\fonts\NotoSans-Regular.ttf`,
			Position:           "bottom-right",
			OpacityPct:         35,
			MarginPx:           24,
			LogoWidthPx:        240,
			FontSizePx:         18,
			AudioGainDB:        -18,
			VideoCodec:         "libx264",
			VideoCRF:           23,
			VideoPreset:        "veryfast",
			AudioCodec:         "aac",
			AudioBitrate:       "160k",
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// argAfter returns the argument following the first occurrence of flag.
func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestBuildArgsVideoLogo(t *testing.T) {
	e := testEngine()
	args, err := e.buildArgs(Request{
		SourcePath: "in.mp4", OutputPath: "out.mp4", Media: MediaVideo, Overlay: OverlayLogo,
	})
	if err != nil {
		t.Fatal(err)
	}
	// two inputs: source then logo
	if got := args[:4]; strings.Join(got, " ") != "-y -i in.mp4 -i" {
		t.Errorf("prefix = %v", got)
	}
	if argAfter(args, "-i") != "in.mp4" {
		t.Errorf("first -i = %q", argAfter(args, "-i"))
	}
	fc := argAfter(args, "-filter_complex")
	if !strings.Contains(fc, "[1:v]scale=240:-1,format=rgba,colorchannelmixer=aa=0.35[wm]") {
		t.Errorf("logo chain missing: %s", fc)
	}
	if !strings.Contains(fc, "[0:v][wm]overlay=W-w-24:H-h-24[vout]") {
		t.Errorf("overlay expr missing: %s", fc)
	}
	if argAfter(args, "-crf") != "23" || argAfter(args, "-preset") != "veryfast" {
		t.Errorf("encode opts wrong: %v", args)
	}
	if args[len(args)-1] != "out.mp4" {
		t.Errorf("output must be last arg, got %q", args[len(args)-1])
	}
	if !contains(args, "0:a?") {
		t.Errorf("expected optional audio map 0:a?: %v", args)
	}
}

func TestBuildArgsVideoTextOnlyHasNoLogoInput(t *testing.T) {
	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.mp4", OutputPath: "out.mp4", Media: MediaVideo, Overlay: OverlayText, Text: "Ana : 50%",
	})
	if n := countFlag(args, "-i"); n != 1 {
		t.Errorf("text-only overlay should have 1 input, got %d: %v", n, args)
	}
	fc := argAfter(args, "-filter_complex")
	if !strings.HasPrefix(fc, "[0:v]drawtext=") {
		t.Errorf("text filter should start from [0:v]: %s", fc)
	}
	// Inside a single-quoted value ':' is literal; only '%' (drawtext text
	// expansion), '\'' and '\\' need escaping.
	if !strings.Contains(fc, `text='Ana : 50\%'`) {
		t.Errorf("text not escaped for filtergraph: %s", fc)
	}
	if !strings.Contains(fc, `fontfile='C\:/fonts/NotoSans-Regular.ttf'`) {
		t.Errorf("font path not normalised/escaped: %s", fc)
	}
}

func TestBuildArgsBothChainsOverlayThenText(t *testing.T) {
	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.mp4", OutputPath: "out.mp4", Media: MediaVideo, Overlay: OverlayBoth, Text: "x",
	})
	fc := argAfter(args, "-filter_complex")
	if !strings.Contains(fc, "[wm]overlay=W-w-24:H-h-24[tmp]") || !strings.Contains(fc, "[tmp]drawtext=") {
		t.Errorf("both mode should overlay to [tmp] then drawtext: %s", fc)
	}
}

func TestBuildArgsImage(t *testing.T) {
	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.png", OutputPath: "out.png", Media: MediaImage, Overlay: OverlayLogo,
	})
	if !contains(args, "-frames:v") || argAfter(args, "-frames:v") != "1" {
		t.Errorf("image must request a single frame: %v", args)
	}
	if contains(args, "-c:a") || contains(args, "-movflags") {
		t.Errorf("image args should have no audio/faststart flags: %v", args)
	}
}

func TestBuildArgsAudio(t *testing.T) {
	e := testEngine()
	args, _ := e.buildArgs(Request{
		SourcePath: "in.mp3", OutputPath: "out.mp3", Media: MediaAudio,
	})
	if argAfter(args, "-stream_loop") != "-1" {
		t.Errorf("watermark input must loop: %v", args)
	}
	fc := argAfter(args, "-filter_complex")
	if !strings.Contains(fc, "[1:a]volume=-18dB[wm]") || !strings.Contains(fc, "amix=inputs=2:duration=first") {
		t.Errorf("audio mix filter wrong: %s", fc)
	}
	if !contains(args, "-shortest") {
		t.Errorf("audio output should be -shortest: %v", args)
	}
}

func TestBuildArgsUnsupportedMedia(t *testing.T) {
	if _, err := testEngine().buildArgs(Request{Media: "pdf", SourcePath: "a", OutputPath: "b"}); err == nil {
		t.Fatal("expected error for unsupported media")
	}
}

func TestNewRejectsMissingBinary(t *testing.T) {
	_, err := New(config.FFmpegConfig{
		BinPath:   filepath.Join(t.TempDir(), "nope-ffmpeg"),
		ProbePath: "ffprobe",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected New to fail when ffmpeg binary is absent")
	}
}

// TestWatermarkIntegration runs a real ffmpeg pass over a generated clip.
// Skipped when ffmpeg/ffprobe are not installed.
func TestWatermarkIntegration(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	// 1s test pattern with a tone.
	gen := exec.Command(bin, "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=15:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-c:a", "aac", "-shortest", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("could not generate test clip: %v: %s", err, out)
	}

	cfg := testEngine().cfg
	cfg.BinPath, cfg.ProbePath = "ffmpeg", "ffprobe"
	cfg.LogoPath, cfg.FontPath = "", ""
	e, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	pr, err := e.Probe(context.Background(), src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if pr.Media != MediaVideo || !pr.HasAudio || pr.Width != 320 {
		t.Errorf("probe = %+v", pr)
	}

	out := filepath.Join(dir, "out.mp4")
	if err := e.Watermark(context.Background(), Request{
		SourcePath: src, OutputPath: out, Media: MediaVideo, Overlay: OverlayText, Text: "SADE preview",
	}); err != nil {
		t.Fatalf("Watermark: %v", err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("output missing or empty: %v", err)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func countFlag(s []string, flag string) int {
	n := 0
	for _, v := range s {
		if v == flag {
			n++
		}
	}
	return n
}
