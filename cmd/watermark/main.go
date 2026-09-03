// Command watermark applies SADE's watermark to one media file and prints a
// live progress bar. It builds the real ffmpeg.Engine from config defaults
// (overridable by flags), so it doubles as a manual test of internals/ffmpeg.
// Requires the ffmpeg and ffprobe binaries on PATH.
//
//	go run ./cmd/watermark -in clip.mp4
//	go run ./cmd/watermark -in song.mp3 -out tagged.mp3
//	go run ./cmd/watermark -in photo.jpg -kind text -text "Ana - 2026-09-03"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"sade/config"
	"sade/internals/ffmpeg"
	"sade/internals/logger"
)

func main() {
	in := flag.String("in", "", "input media file (required)")
	out := flag.String("out", "", "output file (default: <in>.watermarked<ext>)")
	kind := flag.String("kind", "both", "watermark kind: logo | text | both")
	text := flag.String("text", "SADE preview", "text to burn in for kind=text|both")
	logo := flag.String("logo", "", "override the logo PNG path")
	font := flag.String("font", "", "override the drawtext font path")
	quiet := flag.Bool("quiet", false, "no progress bar, just the final line")
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: watermark -in FILE [-out FILE] [-kind both] [-text ...]")
		os.Exit(2)
	}
	if *out == "" {
		ext := filepath.Ext(*in)
		*out = strings.TrimSuffix(*in, ext) + ".watermarked" + ext
	}

	cfg := config.Defaults().FFmpeg
	if *logo != "" {
		cfg.LogoPath = *logo
	}
	if *font != "" {
		cfg.FontPath = *font
	}

	logCfg := config.Defaults().Logger
	logCfg.Output, logCfg.Level = "stdout", "warn"
	log, closeLog, err := logger.New(logCfg)
	if err != nil {
		fatal("logger: %v", err)
	}
	defer closeLog.Close()

	eng, err := ffmpeg.New(cfg, log)
	if err != nil {
		fatal("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	probe, err := eng.Probe(ctx, *in)
	if err != nil {
		fatal("probe %s: %v", *in, err)
	}
	fmt.Printf("in : %s  (%s, %.1fs, %dx%d)\nout: %s\n",
		*in, probe.Media, probe.DurationSec, probe.Width, probe.Height, *out)

	onProgress := func(p ffmpeg.Progress) { printBar(p) }
	if *quiet {
		onProgress = nil
	}

	start := time.Now()
	err = eng.Watermark(ctx, ffmpeg.Request{
		SourcePath:  *in,
		OutputPath:  *out,
		Media:       probe.Media,
		Overlay:     *kind,
		Text:        *text,
		DurationSec: probe.DurationSec,
		OnProgress:  onProgress,
	})
	if !*quiet {
		fmt.Println()
	}
	if err != nil {
		fatal("watermark: %v", err)
	}

	fi, _ := os.Stat(*out)
	fmt.Printf("done in %s  (%d bytes)\n", time.Since(start).Round(time.Millisecond), fi.Size())
}

func printBar(p ffmpeg.Progress) {
	const width = 32
	filled := int(p.Percent / 100 * width)
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	speed := ""
	if p.Speed > 0 {
		speed = fmt.Sprintf("%.1fx", p.Speed)
	}
	fmt.Printf("\r[%s] %5.1f%%  %-6s frame %-7d", bar, p.Percent, speed, p.Frame)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "watermark: "+format+"\n", a...)
	os.Exit(1)
}
