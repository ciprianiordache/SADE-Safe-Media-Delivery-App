package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"sade/config"
)

func newTestStore(t *testing.T) Storage {
	t.Helper()
	s, err := New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestPutOpenStatDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	body := []byte("watermarked bytes")

	fi, err := s.Put(ctx, "previews/job1/asset1.mp4", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if fi.Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", fi.Size, len(body))
	}
	if fi.ContentType != "video/mp4" {
		t.Errorf("ContentType = %q, want video/mp4", fi.ContentType)
	}

	rc, err := s.Open(ctx, "previews/job1/asset1.mp4")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Errorf("round-trip mismatch: %q", got)
	}

	if err := s.Delete(ctx, "previews/job1/asset1.mp4"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Open(ctx, "previews/job1/asset1.mp4"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open after Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "previews/job1/asset1.mp4"); err != nil {
		t.Errorf("Delete of missing key should be nil, got %v", err)
	}
}

func TestPutOverwriteIsAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Put(ctx, "a/b.txt", strings.NewReader("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "a/b.txt", strings.NewReader("v2-longer")); err != nil {
		t.Fatal(err)
	}
	rc, _ := s.Open(ctx, "a/b.txt")
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "v2-longer" {
		t.Errorf("got %q, want v2-longer", got)
	}
}

func TestBadKeysRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, k := range []string{"", "   ", "/", "../escape", "a/../../etc/passwd", "a/../../.."} {
		if _, err := s.Put(ctx, k, strings.NewReader("x")); !errors.Is(err, ErrBadKey) {
			t.Errorf("Put(%q): err = %v, want ErrBadKey", k, err)
		}
	}
}

func TestLocalPathInsideRoot(t *testing.T) {
	dir := t.TempDir()
	s, err := New(config.StorageConfig{Driver: "local", LocalPath: dir},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.LocalPath("originals/j/a.bin")
	if !ok {
		t.Fatal("LocalPath ok = false for local backend")
	}
	absDir, _ := filepath.Abs(dir)
	if !strings.HasPrefix(p, absDir) {
		t.Errorf("LocalPath %q escapes root %q", p, absDir)
	}
	if _, ok := s.LocalPath("../nope"); ok {
		t.Error("LocalPath ok = true for a traversal key")
	}
}

func TestS3NotImplemented(t *testing.T) {
	if _, err := New(config.StorageConfig{Driver: "s3", S3Bucket: "b", S3Region: "r"}, nil); err == nil {
		t.Fatal("expected s3 driver to return an error for now")
	}
	if _, err := New(config.StorageConfig{Driver: "weird"}, nil); err == nil {
		t.Fatal("expected unknown driver to error")
	}
}
