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
	// maxLogoDim bounds the decoded picture (a decompression-bomb guard).
	maxLogoDim = 6000
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
}

// Service manages brand settings.
type Service struct {
	repo Repo
	blob Blob
	log  *slog.Logger
}

func NewService(repo Repo, blob Blob, log *slog.Logger) *Service {
	return &Service{repo: repo, blob: blob, log: log}
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
// brand at it. Logos are stored under their content hash and never deleted:
// jobs still in the queue may reference an older one.
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
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width > maxLogoDim || cfg.Height > maxLogoDim {
		return Response{}, ErrInvalidLogo
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Response{}, ErrInvalidLogo
	}
	img = downscale(img, logoStoreDim)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return Response{}, fmt.Errorf("brand: encode logo: %w", err)
	}
	sum := sha256.Sum256(out.Bytes())
	key := "brands/" + userID + "/" + hex.EncodeToString(sum[:8]) + ".png"
	if _, err := s.blob.Put(ctx, key, bytes.NewReader(out.Bytes())); err != nil {
		return Response{}, fmt.Errorf("brand: store logo: %w", err)
	}

	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	b.LogoKey = key
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
	s.log.Info("brand logo set", "user", userID, "key", key)
	return toResponse(b), nil
}

// RemoveLogo unsets the logo (the file stays - see SetLogo).
func (s *Service) RemoveLogo(userID string) (Response, error) {
	b, err := s.load(userID)
	if err != nil {
		return Response{}, err
	}
	if b.LogoKey == "" {
		return toResponse(b), nil
	}
	b.LogoKey = ""
	if err := s.repo.Save(&b); err != nil {
		return Response{}, err
	}
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
	src := image.NewNRGBA(b)
	draw.Draw(src, b, img, b.Min, draw.Src)
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
