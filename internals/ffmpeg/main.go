// Package ffmpeg is the watermark engine. It shells out to the ffmpeg and
// ffprobe binaries (which must be installed / on PATH, or given as absolute
// paths in config) rather than going through a Go DAG builder: a hand-written
// -filter_complex is clearer for overlay + drawtext + opacity + positioning,
// and lets the whole invocation be unit-tested as an argument list without
// ffmpeg present.
//
// One Engine is shared by all workers. Every run takes a context, so
// WORKER_JOB_TIMEOUT cancels a stuck ffmpeg process.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"sade/config"
)

// New resolves the ffmpeg/ffprobe binaries and returns an Engine. It fails if
// either binary cannot be found, so a misconfigured host is caught at startup
// rather than on the first job.
func New(cfg config.FFmpegConfig, log *slog.Logger) (*Engine, error) {
	bin, err := resolveBinary(cfg.BinPath)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: locate ffmpeg (%q): %w", cfg.BinPath, err)
	}
	probe, err := resolveBinary(cfg.ProbePath)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: locate ffprobe (%q): %w", cfg.ProbePath, err)
	}
	return &Engine{cfg: cfg, log: log, bin: bin, probe: probe}, nil
}

func resolveBinary(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}
	return exec.LookPath(p)
}

// Probe runs ffprobe against path and reports what kind of media it is.
func (e *Engine) Probe(ctx context.Context, path string) (*ProbeResult, error) {
	cmd := exec.CommandContext(ctx, e.probe,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: probe %q: %w: %s", path, err, tail(errb.String(), 300))
	}

	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			NbFrames  string `json:"nb_frames"`
		} `json:"streams"`
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("ffmpeg: parse ffprobe output for %q: %w", path, err)
	}

	res := &ProbeResult{FormatName: raw.Format.FormatName}
	res.DurationSec, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			res.HasVideo = true
			if s.Width > res.Width {
				res.Width, res.Height = s.Width, s.Height
			}
		case "audio":
			res.HasAudio = true
		}
	}

	switch {
	case res.HasVideo && res.DurationSec == 0:
		res.Media = MediaImage
	case res.HasVideo:
		res.Media = MediaVideo
	case res.HasAudio:
		res.Media = MediaAudio
	default:
		return nil, fmt.Errorf("ffmpeg: %q has no video or audio stream", path)
	}
	return res, nil
}

// Watermark applies the configured watermark to req.SourcePath and writes the
// result to req.OutputPath. The output directory is created if missing.
func (e *Engine) Watermark(ctx context.Context, req Request) error {
	if req.SourcePath == "" || req.OutputPath == "" {
		return fmt.Errorf("ffmpeg: source and output paths are required")
	}
	args, err := e.buildArgs(req)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return fmt.Errorf("ffmpeg: create output dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, e.bin, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	e.log.Debug("ffmpeg run", "media", req.Media, "overlay", req.Overlay, "args", strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: watermark %s: %w: %s", req.Media, err, tail(errb.String(), 500))
	}
	return nil
}

// buildArgs is split out so the whole invocation can be asserted in tests
// without ffmpeg installed.
func (e *Engine) buildArgs(req Request) ([]string, error) {
	switch req.Media {
	case MediaVideo:
		return e.videoArgs(req), nil
	case MediaImage:
		return e.imageArgs(req), nil
	case MediaAudio:
		return e.audioArgs(req), nil
	default:
		return nil, fmt.Errorf("ffmpeg: unsupported media %q", req.Media)
	}
}

func (e *Engine) videoArgs(req Request) []string {
	fc, outLabel := e.visualFilter(req.Overlay, req.Text)
	args := []string{"-y", "-i", req.SourcePath}
	if needsLogo(req.Overlay) {
		args = append(args, "-i", e.cfg.LogoPath)
	}
	args = append(args,
		"-filter_complex", fc,
		"-map", outLabel, "-map", "0:a?",
		"-c:v", e.cfg.VideoCodec, "-crf", itoa(e.cfg.VideoCRF), "-preset", e.cfg.VideoPreset,
		"-c:a", e.cfg.AudioCodec, "-b:a", e.cfg.AudioBitrate,
		"-movflags", "+faststart",
		req.OutputPath,
	)
	return args
}

func (e *Engine) imageArgs(req Request) []string {
	fc, outLabel := e.visualFilter(req.Overlay, req.Text)
	args := []string{"-y", "-i", req.SourcePath}
	if needsLogo(req.Overlay) {
		args = append(args, "-i", e.cfg.LogoPath)
	}
	args = append(args,
		"-filter_complex", fc,
		"-map", outLabel,
		"-frames:v", "1",
		req.OutputPath,
	)
	return args
}

func (e *Engine) audioArgs(req Request) []string {
	// A continuous low-volume watermark clip mixed over the whole track.
	// The clip loops for the source's full length; -shortest + duration=first
	// keep the output the same length as the source. AudioIntervalSec is
	// reserved for a future periodic-beep mode.
	fc := fmt.Sprintf(
		"[1:a]volume=%ddB[wm];[0:a][wm]amix=inputs=2:duration=first:dropout_transition=0[aout]",
		e.cfg.AudioGainDB,
	)
	return []string{
		"-y",
		"-i", req.SourcePath,
		"-stream_loop", "-1", "-i", e.cfg.AudioWatermarkPath,
		"-filter_complex", fc,
		"-map", "[aout]",
		"-c:a", e.cfg.AudioCodec, "-b:a", e.cfg.AudioBitrate,
		"-shortest",
		req.OutputPath,
	}
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func itoa(n int) string { return strconv.Itoa(n) }
