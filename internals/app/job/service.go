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
	"time"
	"unicode/utf8"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/ffmpeg"
	"sade/internals/storage"
	"sade/internals/wmopts"
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
	// Usage reports the caller's consumption against their quotas.
	Usage(ctx context.Context, userID string) (Usage, error)
	// Delete removes one of the caller's jobs: its stored files, its asset
	// and payment rows, and the job row - freeing quota. ErrNotFound for
	// someone else's job, ErrBusy while processing, ErrPaid once a recipient
	// has paid for the original.
	Delete(ctx context.Context, userID, jobID string) error
}

// NewUpload is the parsed POST /api/jobs request: the non-file fields plus the
// file part as a stream. The handler fills it in from the multipart form; the
// media type is derived from Filename here, never trusted from the client.
type NewUpload struct {
	RecipientEmail string
	WatermarkKind  string // "" defaults to WatermarkBoth
	WatermarkText  string // "" renders from the configured template

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
	engine *ffmpeg.Engine
	cfg    config.UploadConfig
	log    *slog.Logger

	brands BrandSource // optional: the operator's brand, frozen onto each job
}

// BrandSource gives the brand snapshot (internals/wmopts JSON) to freeze on
// a new job. brand.Service satisfies it.
type BrandSource interface {
	Snapshot(userID string) (string, error)
}

// BrandAware is implemented by the service NewService returns; app.go uses
// it to plug in the brand domain (which tests leave out).
type BrandAware interface {
	UseBrands(BrandSource)
}

func (s *service) UseBrands(b BrandSource) { s.brands = b }

// NewService wires the job service. store is the blob backend for originals;
// assets is the asset repository (composed here the way auth composes
// magic_token). engine is optional (nil when ffmpeg/ffprobe are unavailable,
// mirroring the worker's own optionality - see app.go): when set, Create
// ffprobes the stored upload and rejects it if its actual stream kind
// doesn't match the extension it was accepted under; when nil, Create falls
// back to the extension-only check it always did. Mutations are logged;
// reads are not.
func NewService(repo Repo, assets asset.Repo, store storage.Storage, engine *ffmpeg.Engine, cfg config.UploadConfig, log *slog.Logger) Service {
	return &service{repo: repo, assets: assets, store: store, engine: engine, cfg: cfg, log: log}
}

func (s *service) Create(ctx context.Context, userID string, in NewUpload) (Response, error) {
	recipient, err := normalizeEmail(in.RecipientEmail)
	if err != nil {
		return Response{}, err
	}
	text := strings.TrimSpace(in.WatermarkText)
	if utf8.RuneCountInString(text) > ffmpeg.MaxTextRunes {
		return Response{}, ErrInvalidInput
	}
	text = ffmpeg.SanitizeText(text)
	// The look comes only from the operator's brand (below); there is no
	// client-sent watermark JSON. Without a BrandSource (tests): defaults.
	opts := ""
	kind := strings.TrimSpace(in.WatermarkKind)
	if kind == "" {
		kind = WatermarkBoth
	}
	if !validKind(kind) {
		return Response{}, ErrInvalidInput
	}
	if s.brands != nil {
		// The look comes from the operator's brand, frozen now so later
		// brand changes never alter this file.
		snap, err := s.brands.Snapshot(userID)
		if err != nil {
			return Response{}, fmt.Errorf("job: brand snapshot: %w", err)
		}
		opts = snap
		// No logo of their own: never stamp someone else's (the server's
		// default) on an operator's files - text only.
		if wmopts.Decode(snap).LogoKey == "" && kind != WatermarkText {
			kind = WatermarkText
		}
	}
	media, ext, err := s.detectMedia(in.Filename)
	if err != nil {
		return Response{}, err
	}

	maxBytes := int64(s.cfg.MaxSizeMiB) << 20
	if maxBytes > 0 && in.DeclaredSize > maxBytes {
		return Response{}, ErrFileTooLarge
	}
	usage, err := s.Usage(ctx, userID)
	if err != nil {
		return Response{}, err
	}
	if !usage.Unlimited {
		if usage.JobsPerDay > 0 && usage.JobsLast24h >= usage.JobsPerDay {
			return Response{}, ErrDailyLimit
		}
		if usage.StorageLimitBytes > 0 && usage.StorageBytes+in.DeclaredSize > usage.StorageLimitBytes {
			return Response{}, ErrStorageQuota
		}
	}

	j := &Job{
		UserID:         userID,
		Status:         StatusPending,
		MediaType:      media,
		RecipientEmail: recipient,
		WatermarkKind:  kind,
		WatermarkText:  text,
		WatermarkOpts:  opts,
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

	// DeclaredSize is the client's word; re-check the quota against what
	// was actually stored.
	if !usage.Unlimited && usage.StorageLimitBytes > 0 && usage.StorageBytes+fi.Size > usage.StorageLimitBytes {
		s.rollback(ctx, id, key)
		return Response{}, ErrStorageQuota
	}

	if err := s.verifyMedia(ctx, key, media); err != nil {
		s.rollback(ctx, id, key)
		return Response{}, err
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
	one := []Response{resp}
	s.addDelivery(one)
	return one[0], nil
}

// addDelivery fills the opened/paid signals in place. They are a nicety on
// top of the job data, so a failed lookup is logged, not returned.
func (s *service) addDelivery(rs []Response) {
	ids := make([]string, len(rs))
	for i := range rs {
		ids[i] = rs[i].ID
	}
	sum, err := s.repo.DeliverySummary(ids)
	if err != nil {
		s.log.Warn("job: delivery summary", "error", err)
		return
	}
	for i := range rs {
		d, ok := sum[rs[i].ID]
		if !ok {
			continue
		}
		rs[i].Paid = d.Paid
		if !d.OpenedAt.IsZero() {
			at := d.OpenedAt
			rs[i].OpenedAt = &at
		}
	}
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
	s.addDelivery(out)
	return out, nil
}

func (s *service) Delete(ctx context.Context, userID, jobID string) error {
	j, err := s.repo.GetByID(jobID)
	if err != nil {
		return err
	}
	if j.UserID != userID {
		return ErrNotFound
	}
	paid, err := s.repo.HasPaidPayment(jobID)
	if err != nil {
		return err
	}
	if paid {
		return ErrPaid
	}
	assets, err := s.assets.ListByJob(jobID)
	if err != nil {
		return fmt.Errorf("job: load assets: %w", err)
	}
	// Rows first: once the job row is gone no link resolves to these files
	// any more, so a blob that then fails to delete is an orphan on disk,
	// never a dangling database reference.
	deleted, err := s.repo.DeleteIfNotProcessing(jobID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrBusy
	}
	for _, a := range assets {
		if err := s.store.Delete(ctx, a.StorageKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
			s.log.Error("delete job: remove blob", "job", jobID, "key", a.StorageKey, "error", err)
		}
	}
	s.log.Info("job deleted", "job", jobID, "user", userID, "files", len(assets))
	return nil
}

func (s *service) Usage(ctx context.Context, userID string) (Usage, error) {
	if quotaExempt(ctx) {
		return Usage{Unlimited: true}, nil
	}
	jobs, err := s.repo.CountCreatedSince(userID, time.Now().Add(-24*time.Hour))
	if err != nil {
		return Usage{}, err
	}
	used, err := s.repo.StorageBytes(userID)
	if err != nil {
		return Usage{}, err
	}
	return Usage{
		JobsLast24h:       jobs,
		JobsPerDay:        s.cfg.JobsPerDay,
		StorageBytes:      used,
		StorageLimitBytes: int64(s.cfg.StorageQuotaMiB) << 20,
	}, nil
}

// verifyMedia ffprobes the just-stored blob and confirms its actual stream
// kind matches media (the extension-based classification detectMedia already
// made). It is a no-op when the engine is unavailable or the storage backend
// can't hand back a local path (S3, not implemented yet) - extension-based
// detection is the only signal in that case, same as before this check
// existed.
func (s *service) verifyMedia(ctx context.Context, key, media string) error {
	if s.engine == nil {
		return nil
	}
	path, ok := s.store.LocalPath(key)
	if !ok {
		return nil
	}
	res, err := s.engine.Probe(ctx, path)
	if err != nil {
		return ErrCorruptMedia
	}
	if res.Media != media {
		return ErrCorruptMedia
	}
	return nil
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
