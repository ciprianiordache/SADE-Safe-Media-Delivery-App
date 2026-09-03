package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// local is a Storage backed by a directory tree on the local filesystem.
type local struct {
	root string
	log  *slog.Logger
}

func newLocal(root string, log *slog.Logger) (*local, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("storage: local_path is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve local_path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create local_path %q: %w", abs, err)
	}
	return &local{root: abs, log: log}, nil
}

// resolve maps a forward-slash key to an absolute path under root. Keys must
// be relative, backslash-free, and contain no ".." segment; anything else is
// ErrBadKey.
func (l *local) resolve(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\\') {
		return "", ErrBadKey
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", ErrBadKey
		}
	}
	clean := path.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrBadKey
	}
	full := filepath.Join(l.root, filepath.FromSlash(clean))
	if full != l.root && !strings.HasPrefix(full, l.root+string(os.PathSeparator)) {
		return "", ErrBadKey
	}
	return full, nil
}

func (l *local) Put(ctx context.Context, key string, r io.Reader) (FileInfo, error) {
	full, err := l.resolve(key)
	if err != nil {
		return FileInfo{}, err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FileInfo{}, fmt.Errorf("storage: create %q: %w", dir, err)
	}

	// Write to a sibling temp file then rename, so a reader never sees a
	// half-written object and a failed write leaves the old one intact.
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return FileInfo{}, fmt.Errorf("storage: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return FileInfo{}, fmt.Errorf("storage: write %q: %w", key, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return FileInfo{}, fmt.Errorf("storage: sync %q: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return FileInfo{}, fmt.Errorf("storage: close %q: %w", key, err)
	}
	if err := os.Rename(tmpName, full); err != nil {
		return FileInfo{}, fmt.Errorf("storage: commit %q: %w", key, err)
	}

	fi, err := l.Stat(ctx, key)
	if err != nil {
		return FileInfo{}, err
	}
	l.log.Debug("storage put", "key", key, "size", fi.Size)
	return fi, nil
}

func (l *local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", key, err)
	}
	return f, nil
}

func (l *local) Stat(_ context.Context, key string) (FileInfo, error) {
	full, err := l.resolve(key)
	if err != nil {
		return FileInfo{}, err
	}
	st, err := os.Stat(full)
	if errors.Is(err, os.ErrNotExist) {
		return FileInfo{}, ErrNotFound
	}
	if err != nil {
		return FileInfo{}, fmt.Errorf("storage: stat %q: %w", key, err)
	}
	ct := mime.TypeByExtension(filepath.Ext(full))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return FileInfo{Key: key, Size: st.Size(), ModTime: st.ModTime(), ContentType: ct}, nil
}

func (l *local) Delete(_ context.Context, key string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

func (l *local) LocalPath(key string) (string, bool) {
	full, err := l.resolve(key)
	if err != nil {
		return "", false
	}
	return full, true
}
