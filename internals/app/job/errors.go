package job

import "errors"

var (
	// ErrNotFound is returned when no job matches the lookup, or when the job
	// exists but belongs to another operator (the two are deliberately
	// indistinguishable to a caller).
	ErrNotFound = errors.New("job: not found")
	// ErrInvalidInput is returned for a missing/malformed recipient email or
	// an unknown watermark kind.
	ErrInvalidInput = errors.New("job: invalid input")
	// ErrUnsupportedMedia is returned when the uploaded file's extension is
	// not in any of the configured allow-lists.
	ErrUnsupportedMedia = errors.New("job: unsupported media type")
	// ErrFileTooLarge is returned when the upload exceeds Upload.MaxSizeMiB.
	ErrFileTooLarge = errors.New("job: file too large")
	// ErrEmptyFile is returned when the upload part carries no bytes.
	ErrEmptyFile = errors.New("job: empty file")
	// ErrCorruptMedia is returned when the upload's extension claims a media
	// type ffprobe cannot back up - either the file isn't decodable at all,
	// or its actual stream kind (video/audio/image) doesn't match the
	// extension's allow-list. Only returned when ffmpeg is available; a
	// missing/unavailable ffprobe leaves extension-based detection as the
	// only check, same as before this existed.
	ErrCorruptMedia = errors.New("job: file content does not match its extension")
)
