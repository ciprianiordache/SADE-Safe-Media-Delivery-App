package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/mail"
	"path/filepath"
	"slices"
	"strings"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/storage"
)

// Service is the job domain's use cases: accept an upload (creating the job
// row + its original asset), and read a caller's own jobs. Status transitions
// are not here - the worker owns them.
type Service interface {
	// Create validates in, streams the uploaded original into storage, and
	// writes the Job row (status pending) plus its original Asset row. The
	// upload is read in a single pass; on any later failure the stored blob
	// and the job row are rolled back.
	Create(ctx context.Context, userID string, in NewUpload) (Response, error)
	// Get returns one job with its assets, or ErrNotFound if it does not
	// exist or belongs to another operator.
	Get(userID, jobID string) (Response, error)
	// Asset returns the raw asset row (storage key included) for one of the
	// caller's own jobs, or ErrNotFound if the job doesn't exist, belongs to
	// another operator, or assetID isn't one of that job's assets. Used by
	// Handler.Content, which needs the storage key Get's Response omits.
	Asset(userID, jobID, assetID string) (asset.Asset, error)
	// List returns the caller's jobs newest-first (without assets).
	List(userID string, offset, limit int) ([]Response, error)
}

// NewUpload is the parsed POST /api/jobs request: the non-file fields plus the
// file part as a stream. The handler fills it in from the multipart form; the
// media type is derived from Filename here, never trusted from the client.
type NewUpload struct {
	RecipientEmail string
	WatermarkKind  string // "" defaults to WatermarkBoth
	WatermarkText  string // "" renders from the configured template
	WatermarkOpts  string // raw JSON overriding ffmpeg defaults; "" = defaults

	Filename string
	// DeclaredSize is the part's self-reported size (0 when unknown). It only
	// enables a fast rejection; the cap is still enforced while streaming.
	DeclaredSize int64
	File         io.Reader
}

type service struct {
	repo   Repo
	assets asset.Repo
	store  storage.Storage
	cfg    config.UploadConfig
	log    *slog.Logger
}

// NewService wires the job service. store is the blob backend for originals;
// assets is the asset repository (composed here the way auth composes
// magic_token). Mutations are logged; reads are not.
func NewService(repo Repo, assets asset.Repo, store storage.Storage, cfg config.UploadConfig, log *slog.Logger) Service {
	return &service{repo: repo, assets: assets, store: store, cfg: cfg, log: log}
}

func (s *service) Create(ctx context.Context, userID string, in NewUpload) (Response, error) {
	recipient, err := normalizeEmail(in.RecipientEmail)
	if err != nil {
		return Response{}, err
	}
	kind := strings.TrimSpace(in.WatermarkKind)
	if kind == "" {
		kind = WatermarkBoth
	}
	if !validKind(kind) {
		return Response{}, ErrInvalidInput
	}
	media, ext, err := s.detectMedia(in.Filename)
	if err != nil {
		return Response{}, err
	}

	maxBytes := int64(s.cfg.MaxSizeMiB) << 20
	if maxBytes > 0 && in.DeclaredSize > maxBytes {
		return Response{}, ErrFileTooLarge
	}

	j := &Job{
		UserID:         userID,
		Status:         StatusPending,
		MediaType:      media,
		RecipientEmail: recipient,
		WatermarkKind:  kind,
		WatermarkText:  strings.TrimSpace(in.WatermarkText),
		WatermarkOpts:  strings.TrimSpace(in.WatermarkOpts),
	}
	id, err := s.repo.Create(j)
	if err != nil {
		s.log.Error("create job row", "user", userID, "error", err)
		return Response{}, fmt.Errorf("job: create: %w", err)
	}
	j.ID = id

	key := "originals/" + id + "/original" + ext
	hasher := sha256.New()
	reader := in.File
	if maxBytes > 0 {
		reader = io.LimitReader(in.File, maxBytes+1)
	}
	fi, err := s.store.Put(ctx, key, io.TeeReader(reader, hasher))
	if err != nil {
		s.rollback(ctx, id, "")
		s.log.Error("store original", "job", id, "error", err)
		return Response{}, fmt.Errorf("job: store original: %w", err)
	}
	switch {
	case fi.Size == 0:
		s.rollback(ctx, id, key)
		return Response{}, ErrEmptyFile
	case maxBytes > 0 && fi.Size > maxBytes:
		s.rollback(ctx, id, key)
		return Response{}, ErrFileTooLarge
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	a := &asset.Asset{
		JobID:      id,
		Kind:       asset.KindOriginal,
		StorageKey: key,
		Filename:   safeFilename(in.Filename),
		MIME:       mimeType,
		SizeBytes:  fi.Size,
		Checksum:   hex.EncodeToString(hasher.Sum(nil)),
	}
	if _, err := s.assets.Create(a); err != nil {
		s.rollback(ctx, id, key)
		s.log.Error("create original asset", "job", id, "error", err)
		return Response{}, fmt.Errorf("job: create asset: %w", err)
	}

	s.log.Info("job created", "job", id, "user", userID, "media", media, "size", fi.Size)
	resp := toResponse(*j)
	resp.Assets = []asset.Response{asset.ToResponse(*a)}
	return resp, nil
}

// rollback best-effort undoes a half-built job: remove the stored blob (if
// any) then the job row (which cascades any asset rows).
func (s *service) rollback(ctx context.Context, jobID, key string) {
	if key != "" {
		if err := s.store.Delete(ctx, key); err != nil {
			s.log.Error("rollback: delete blob", "job", jobID, "key", key, "error", err)
		}
	}
	if err := s.repo.Delete(jobID); err != nil {
		s.log.Error("rollback: delete job", "job", jobID, "error", err)
	}
}

func (s *service) Get(userID, jobID string) (Response, error) {
	j, err := s.repo.GetByID(jobID)
	if err != nil {
		return Response{}, err
	}
	if j.UserID != userID {
		return Response{}, ErrNotFound // don't reveal another operator's job
	}
	as, err := s.assets.ListByJob(jobID)
	if err != nil {
		return Response{}, fmt.Errorf("job: load assets: %w", err)
	}
	resp := toResponse(*j)
	resp.Assets = asset.ToResponses(as)
	return resp, nil
}

func (s *service) Asset(userID, jobID, assetID string) (asset.Asset, error) {
	j, err := s.repo.GetByID(jobID)
	if err != nil {
		return asset.Asset{}, err
	}
	if j.UserID != userID {
		return asset.Asset{}, ErrNotFound // don't reveal another operator's job
	}
	a, err := s.assets.GetByID(assetID)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return asset.Asset{}, ErrNotFound
		}
		return asset.Asset{}, err
	}
	if a.JobID != jobID {
		return asset.Asset{}, ErrNotFound // asset exists, but not under this job
	}
	return *a, nil
}

func (s *service) List(userID string, offset, limit int) ([]Response, error) {
	jobs, err := s.repo.ListByUser(userID, offset, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Response, len(jobs))
	for i := range jobs {
		out[i] = toResponse(jobs[i])
	}
	return out, nil
}

// detectMedia classifies an upload by its filename extension against the
// configured allow-lists. It returns the media type, the lower-cased
// extension (with dot), or ErrUnsupportedMedia.
func (s *service) detectMedia(filename string) (media, ext string, err error) {
	ext = strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if ext == "" {
		return "", "", ErrUnsupportedMedia
	}
	switch {
	case slices.Contains(s.cfg.AllowedVideo, ext):
		return MediaVideo, ext, nil
	case slices.Contains(s.cfg.AllowedAudio, ext):
		return MediaAudio, ext, nil
	case slices.Contains(s.cfg.AllowedImage, ext):
		return MediaImage, ext, nil
	default:
		return "", "", ErrUnsupportedMedia
	}
}

func normalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrInvalidInput
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidInput
	}
	return strings.ToLower(addr.Address), nil
}

func validKind(k string) bool {
	return k == WatermarkLogo || k == WatermarkText || k == WatermarkBoth
}

// safeFilename keeps only the base name and drops path separators, so a
// client cannot smuggle a path into the stored metadata.
func safeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(name)
	if base == "." || base == "/" || base == "" {
		return "upload"
	}
	return base
}
