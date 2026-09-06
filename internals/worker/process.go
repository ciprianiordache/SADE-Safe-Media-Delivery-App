package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"sade/internals/ffmpeg"
)

// process runs the watermark pipeline for one claimed job: locate the
// original, probe it, watermark it into storage, record the preview asset,
// and notify the recipient. Any returned error is recorded on the job row and
// drives a retry or a permanent failure.
func (p *Pool) process(ctx context.Context, j Job) error {
	orig, err := p.assets.Original(ctx, j.ID)
	if err != nil {
		return fmt.Errorf("load original asset: %w", err)
	}

	srcPath, ok := p.blob.LocalPath(orig.StorageKey)
	if !ok {
		// TODO(worker): stage a temp copy for non-local (S3) backends.
		return fmt.Errorf("original %q has no local path (non-local storage not supported yet)", orig.StorageKey)
	}

	probe, err := p.engine.Probe(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("probe original: %w", err)
	}

	name := previewFilename(orig.Filename, probe.Media)
	previewKey := "previews/" + j.ID + "/" + name
	outPath, ok := p.blob.LocalPath(previewKey)
	if !ok {
		return fmt.Errorf("preview key %q has no local path", previewKey)
	}

	if err := p.engine.Watermark(ctx, ffmpeg.Request{
		SourcePath:  srcPath,
		OutputPath:  outPath,
		Media:       probe.Media,
		Overlay:     overlayKind(j.WatermarkKind),
		Text:        p.renderText(j),
		DurationSec: probe.DurationSec,
	}); err != nil {
		return fmt.Errorf("watermark: %w", err)
	}

	fi, err := p.blob.Stat(ctx, previewKey)
	if err != nil {
		return fmt.Errorf("stat preview: %w", err)
	}
	if fi.Size == 0 {
		return fmt.Errorf("watermark produced an empty file")
	}

	sum, err := p.checksum(ctx, previewKey)
	if err != nil {
		return fmt.Errorf("checksum preview: %w", err)
	}

	previewID, err := p.assets.AddPreview(ctx, PreviewInput{
		JobID:      j.ID,
		StorageKey: previewKey,
		Filename:   name,
		MIME:       previewMIME(name),
		SizeBytes:  fi.Size,
		Checksum:   sum,
	})
	if err != nil {
		return fmt.Errorf("record preview asset: %w", err)
	}

	// The preview is stored and recorded; a failed email must not fail the
	// job (the link can be re-sent). Log and move on.
	if err := p.notifier.PreviewReady(ctx, j.RecipientEmail, previewID); err != nil {
		p.log.Error("worker: preview-ready notification failed",
			"job", j.ID, "recipient", j.RecipientEmail, "error", err)
	}
	return nil
}

func (p *Pool) checksum(ctx context.Context, key string) (string, error) {
	rc, err := p.blob.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (p *Pool) renderText(j Job) string {
	if s := strings.TrimSpace(j.WatermarkText); s != "" {
		return s
	}
	t := p.cfg.TextTemplate
	if t == "" {
		t = "{recipient} · {date}"
	}
	t = strings.ReplaceAll(t, "{recipient}", j.RecipientEmail)
	t = strings.ReplaceAll(t, "{date}", time.Now().Format("2006-01-02"))
	return t
}

// overlayKind normalises a job's watermark_kind to an ffmpeg overlay kind,
// defaulting to "both".
func overlayKind(k string) string {
	switch k {
	case ffmpeg.OverlayLogo, ffmpeg.OverlayText, ffmpeg.OverlayBoth:
		return k
	default:
		return ffmpeg.OverlayBoth
	}
}

// previewFilename derives "<base>-preview<ext>" from the original name, with
// an extension the engine's re-encode actually produces for that media kind.
func previewFilename(original, media string) string {
	base := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	if base == "" || base == "." {
		base = "preview"
	}
	return base + "-preview" + previewExt(media, strings.ToLower(filepath.Ext(original)))
}

func previewExt(media, srcExt string) string {
	switch media {
	case ffmpeg.MediaVideo:
		return ".mp4" // H.264 / AAC / +faststart
	case ffmpeg.MediaAudio:
		return ".m4a" // AAC in an MP4 container
	default: // image: keep the source container
		if srcExt == "" {
			return ".png"
		}
		return srcExt
	}
}

func previewMIME(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return "application/octet-stream"
}
