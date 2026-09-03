// Package logger builds the application's *slog.Logger from
// config.LoggerConfig. It deliberately adds no logging abstraction of its
// own: log/slog is already the standard, extensible logging interface in Go,
// so New just wires a slog.Handler according to config (level, json/text,
// stdout/file/both, optional source location) and returns a plain
// *slog.Logger that every other package takes by injection.
//
// Every SADE component that logs receives this one logger (or a child of it
// via With): the HTTP server, the database layer, the worker, and every
// feature package. app.go builds it once, tags it with the environment, and
// passes it down.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"sade/config"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// logFileName is the fixed file created under LoggerConfig.FilePath (which is
// a directory). Config gives us the directory; the name is ours.
const logFileName = "sade.log"

// New builds a *slog.Logger fully described by cfg. It returns an error for
// any unrecognised level/format/output rather than falling back silently, so
// a typo in config surfaces at startup.
//
// The returned io.Closer must be closed on shutdown (next to closing the
// database). For file output it closes the rotating log file - important on
// Windows, where an open file cannot be moved or deleted, which otherwise
// breaks rotation cleanup and temp-dir removal in tests. For stdout it is a
// no-op.
func New(cfg config.LoggerConfig) (*slog.Logger, io.Closer, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, nil, err
	}

	writer, closer, err := resolveWriter(cfg)
	if err != nil {
		return nil, nil, err
	}

	handler, err := newHandler(cfg.Format, writer, level, cfg.AddSource)
	if err != nil {
		return nil, nil, err
	}

	return slog.New(handler), closer, nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logger: unsupported level %q (want debug, info, warn or error)", level)
	}
}

// nopCloser is returned for outputs we must not close (stdout).
type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// resolveWriter turns cfg.Output into the log sink plus the Closer the caller
// owns. "file" wires rotation via lumberjack using cfg's Max* fields; "both"
// writes stdout and the rotating file at once (stdout is only visible while
// developing; the file is what you read back later).
func resolveWriter(cfg config.LoggerConfig) (io.Writer, io.Closer, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Output)) {
	case "stdout", "":
		return os.Stdout, nopCloser{}, nil
	case "file":
		lj, err := newRotatingFile(cfg)
		if err != nil {
			return nil, nil, err
		}
		return lj, lj, nil
	case "both":
		lj, err := newRotatingFile(cfg)
		if err != nil {
			return nil, nil, err
		}
		return io.MultiWriter(os.Stdout, lj), lj, nil
	default:
		return nil, nil, fmt.Errorf("logger: unsupported output %q (want stdout, file or both)", cfg.Output)
	}
}

func newRotatingFile(cfg config.LoggerConfig) (*lumberjack.Logger, error) {
	if strings.TrimSpace(cfg.FilePath) == "" {
		return nil, fmt.Errorf("logger: output is %q but file_path is empty", cfg.Output)
	}
	if err := os.MkdirAll(cfg.FilePath, 0o755); err != nil {
		return nil, fmt.Errorf("logger: create log directory %q: %w", cfg.FilePath, err)
	}
	return &lumberjack.Logger{
		Filename:   filepath.Join(cfg.FilePath, logFileName),
		MaxSize:    cfg.MaxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	}, nil
}

func newHandler(format string, w io.Writer, level slog.Level, addSource bool) (slog.Handler, error) {
	opts := &slog.HandlerOptions{Level: level, AddSource: addSource}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json", "":
		return slog.NewJSONHandler(w, opts), nil
	case "text":
		return slog.NewTextHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("logger: unsupported format %q (want json or text)", format)
	}
}
