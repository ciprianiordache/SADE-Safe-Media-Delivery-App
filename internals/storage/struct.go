package storage

import (
	"context"
	"io"
	"time"
)

// FileInfo is the metadata a backend reports for a stored object.
type FileInfo struct {
	Key         string
	Size        int64
	ModTime     time.Time
	ContentType string
}

// Storage is the blob store for job files - uploaded originals and
// watermarked previews. Callers address objects by forward-slash key
// (e.g. "originals/<jobID>/<assetID>.mp4"); the backend maps that to a
// location. Keys are cleaned and confined to the backend's root; "..",
// absolute paths and empty keys are rejected.
type Storage interface {
	// Put stores r under key (creating parents), overwriting any existing
	// object atomically, and returns its metadata.
	Put(ctx context.Context, key string, r io.Reader) (FileInfo, error)

	// Open returns the object's bytes. It returns ErrNotFound if key does
	// not exist.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Stat returns the object's metadata, or ErrNotFound.
	Stat(ctx context.Context, key string) (FileInfo, error)

	// Delete removes key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error

	// LocalPath returns the real filesystem path for key when the backend
	// is local disk, so the ffmpeg engine can read/write it directly. ok
	// is false for remote backends, where the worker must stage a temp
	// copy instead.
	LocalPath(key string) (path string, ok bool)
}
