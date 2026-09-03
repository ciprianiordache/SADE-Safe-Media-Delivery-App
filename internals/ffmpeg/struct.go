package ffmpeg

import (
	"log/slog"

	"sade/config"
)

// Media kinds (mirror job.MediaType).
const (
	MediaVideo = "video"
	MediaAudio = "audio"
	MediaImage = "image"
)

// Overlay kinds (mirror job.WatermarkKind).
const (
	OverlayLogo = "logo"
	OverlayText = "text"
	OverlayBoth = "both"
)

// Engine wraps the ffmpeg / ffprobe binaries and turns a watermark Request
// into a single ffmpeg invocation. It holds no per-job state, so one Engine
// is shared by every worker goroutine.
type Engine struct {
	cfg   config.FFmpegConfig
	log   *slog.Logger
	bin   string // resolved absolute path to ffmpeg
	probe string // resolved absolute path to ffprobe
}

// Request is one watermarking job for the engine. The worker fills it in
// from a job.Job row and the resolved storage paths.
type Request struct {
	SourcePath string // input media, on local disk
	OutputPath string // where to write the watermarked preview
	Media      string // MediaVideo | MediaAudio | MediaImage
	Overlay    string // OverlayLogo | OverlayText | OverlayBoth (ignored for audio)
	Text       string // text to burn in when Overlay includes text; caller renders any template

	// DurationSec, when > 0, lets Watermark report Percent without a
	// second ffprobe call. The worker already has it from Probe.
	DurationSec float64
	// OnProgress, when non-nil, is called for each ffmpeg -progress block.
	// It runs on a background goroutine; keep it quick and non-blocking.
	OnProgress func(Progress)
}

// ProbeResult is the subset of ffprobe output the pipeline needs.
type ProbeResult struct {
	Media       string // derived: video | audio | image
	DurationSec float64
	Width       int
	Height      int
	HasVideo    bool
	HasAudio    bool
	FormatName  string
}
