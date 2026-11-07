package brand

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // registers JPEG for image.Decode
	"image/png"
	"io"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"sade/internals/ffmpeg"
	"sade/internals/storage"
	"sade/internals/wmopts"
)

// Errors the handler maps to status codes.
var (
	ErrInvalidInput = errors.New("brand: invalid input")
	ErrInvalidLogo  = errors.New("brand: logo must be a PNG or JPEG image")
	ErrLogoTooLarge = errors.New("brand: logo file is too large")
)

const (
	// MaxLogoBytes bounds the uploaded file.
	MaxLogoBytes = 3 << 20
	// maxLogoDim bounds each side and maxLogoPixels the whole decoded
	// picture (a decompression-bomb guard: a 3 MB PNG can still decode to a
	// huge bitmap). 4 MP is far more than a logo stored at 800 px needs.
	maxLogoDim    = 6000
	maxLogoPixels = 4_000_000
	// decodeSlots bounds how many logos are decoded at once across the whole
	// process (~16 MB each at the pixel cap), whatever the number of accounts.
	decodeSlots = 2
	// logoStoreDim is the longest side the stored logo is scaled down to:
	// plenty for a watermark on 4K, and small enough to inline in an email.
	logoStoreDim = 800

	maxCompanyRunes  = 80
	maxTemplateRunes = 120
)

// Blob is the storage the service writes logos to.
type Blob interface {
	Put(ctx context.Context, key string, r io.Reader) (storage.FileInfo, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Service manages brand settings.
type Service struct {
	repo   Repo
	blob   Blob
	log    *slog.Logger
	decode chan struct{} // semaphore, see decodeSlots
}

func NewService(repo Repo, blob Blob, log *slog.Logger) *Service {
	return &Service{repo: repo, blob: blob, log: log, decode: make(chan struct{}, decodeSlots)}
}

func (s *Service) load(userID string) (Brand, error) {
	b, err := s.repo.GetByUser(userID)
	if errors.Is(err, ErrNotFound) {
		return defaults(userID), nil
	}
	if err != nil {
		return Brand{}, err
	}
	return *b, nil
}

// Get returns the account's brand, or the defaults when none is saved.
func (s *Service) Get(userID string) (Response, error) {
	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	return toResponse(b), nil
}

// Update validates and saves the editable settings.
func (s *Service) Update(userID string, req UpdateRequest) (Response, error) {
	company := cleanText(req.CompanyName)
	template := cleanText(req.TextTemplate)
	switch {
	case utf8.RuneCountInString(company) > maxCompanyRunes,
		utf8.RuneCountInString(template) > maxTemplateRunes,
		!ffmpeg.ValidPosition(req.Position),
		req.OpacityPct < 10 || req.OpacityPct > 100,
		req.LogoScalePct < 5 || req.LogoScalePct > 50:
		return Response{}, ErrInvalidInput
	}
	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	b.CompanyName, b.TextTemplate = company, template
	b.Position, b.OpacityPct, b.LogoScalePct = req.Position, req.OpacityPct, req.LogoScalePct
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
	s.log.Info("brand updated", "user", userID)
	return toResponse(b), nil
}

// SetLogo validates an uploaded PNG/JPEG, normalises it (decoded and
// re-encoded as PNG - which also drops any metadata or payload smuggled in
// the file - and scaled down to logoStoreDim), stores it and points the
// brand at it. Only the current logo is kept: the previous one is deleted,
// unless an unfinished job still uses it (see dropLogo).
func (s *Service) SetLogo(ctx context.Context, userID string, r io.Reader) (Response, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxLogoBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("brand: read logo: %w", err)
	}
	if len(raw) > MaxLogoBytes {
		return Response{}, ErrLogoTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") {
		return Response{}, ErrInvalidLogo
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width > maxLogoDim || cfg.Height > maxLogoDim ||
		cfg.Width*cfg.Height > maxLogoPixels {
		return Response{}, ErrInvalidLogo
	}
	out, err := s.normalise(ctx, raw)
	if err != nil {
		return Response{}, err
	}
	sum := sha256.Sum256(out)
	key := "brands/" + userID + "/" + hex.EncodeToString(sum[:8]) + ".png"
	if _, err := s.blob.Put(ctx, key, bytes.NewReader(out)); err != nil {
		return Response{}, fmt.Errorf("brand: store logo: %w", err)
	}

	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	prev := b.LogoKey
	b.LogoKey = key
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
	s.log.Info("brand logo set", "user", userID, "key", key)
	s.dropLogo(ctx, userID, prev, key)
	return toResponse(b), nil
}

// normalise decodes, downscales and re-encodes a logo as PNG. At most
// decodeSlots run at once; a caller waiting for a slot gives up when its
// request ends.
func (s *Service) normalise(ctx context.Context, raw []byte) ([]byte, error) {
	select {
	case s.decode <- struct{}{}:
		defer func() { <-s.decode }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalidLogo
	}
	var out bytes.Buffer
	if err := png.Encode(&out, downscale(img, logoStoreDim)); err != nil {
		return nil, fmt.Errorf("brand: encode logo: %w", err)
	}
	return out.Bytes(), nil
}

// dropLogo deletes a replaced or removed logo file, so an account never
// holds more than its current one. It keeps the file when it is the one
// now in use (re-uploading the same picture gives the same key), or when a
// pending/processing job's snapshot still points at it - the worker reads
// it during the encode. A kept file is only a small orphan. Failures are
// logged, never returned: the brand itself is already saved.
//
// Two uploads racing on one account can each read the same previous key;
// the loser's new file is then left unreferenced (same small orphan).
func (s *Service) dropLogo(ctx context.Context, userID, old, current string) {
	if old == "" || old == current {
		return
	}
	inUse, err := s.repo.LogoInUse(old)
	if err != nil {
		s.log.Warn("brand: logo in-use check failed, keeping file", "user", userID, "key", old, "error", err)
		return
	}
	if inUse {
		s.log.Info("brand: old logo kept for an unfinished job", "user", userID, "key", old)
		return
	}
	if err := s.blob.Delete(ctx, old); err != nil {
		s.log.Warn("brand: delete old logo", "user", userID, "key", old, "error", err)
		return
	}
	s.log.Info("brand: old logo deleted", "user", userID, "key", old)
}

// RemoveLogo unsets the logo and deletes its file (see dropLogo).
func (s *Service) RemoveLogo(ctx context.Context, userID string) (Response, error) {
	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	if b.LogoKey == "" {
		return toResponse(b), nil
	}
	prev := b.LogoKey
	b.LogoKey = ""
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
	s.dropLogo(ctx, userID, prev, "")
	return toResponse(b), nil
}

// Logo opens the account's current logo; ErrNotFound when there is none.
func (s *Service) Logo(ctx context.Context, userID string) (io.ReadCloser, error) {
	b, err := s.load(userID)
	if err != nil {
		return nil, err
	}
	if b.LogoKey == "" {
		return nil, ErrNotFound
	}
	return s.blob.Open(ctx, b.LogoKey)
}

// CompleteOnboarding records that the welcome setup was finished (or
// skipped), so it is not shown again.
func (s *Service) CompleteOnboarding(userID string) (Response, error) {
	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	b.Onboarded = 1
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
	return toResponse(b), nil
}

// Snapshot is the brand frozen onto a new job (jobs.watermark_opts).
func (s *Service) Snapshot(userID string) (string, error) {
	b, err := s.load(userID)
	if err != nil {
		return "", err
	}
	return wmopts.Encode(wmopts.Opts{
		Company: b.CompanyName, LogoKey: b.LogoKey, Position: b.Position,
		OpacityPct: b.OpacityPct, LogoScalePct: b.LogoScalePct, TextTemplate: b.TextTemplate,
	}), nil
}

// cleanText flattens control characters and trims - the text ends up burned
// into pictures and in email headers.
func cleanText(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
}

// downscale shrinks img so its longest side is at most maxDim, averaging
// the source pixels each destination pixel covers (keeps edges clean, which
// matters for logos). Smaller images are returned as-is.
func downscale(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	var nw, nh int
	if w >= h {
		nw, nh = maxDim, max(1, h*maxDim/w)
	} else {
		nw, nh = max(1, w*maxDim/h), maxDim
	}
	// Already NRGBA (most PNGs): read it in place instead of copying.
	src, ok := img.(*image.NRGBA)
	if !ok {
		src = image.NewNRGBA(b)
		draw.Draw(src, b, img, b.Min, draw.Src)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max((x+1)*w/nw, x*w/nw+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := src.NRGBAAt(b.Min.X+sx, b.Min.Y+sy)
					// Weight colour by alpha so transparent pixels don't
					// darken the edges.
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			px := color.NRGBA{A: uint8(a / n)}
			if a > 0 {
				px.R, px.G, px.B = uint8(r/a), uint8(g/a), uint8(bl/a)
			}
			dst.SetNRGBA(x, y, px)
		}
	}
	return dst
}
