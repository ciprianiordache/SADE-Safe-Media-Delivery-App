package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sade/config"
)

func baseConfig() config.LoggerConfig {
	return config.LoggerConfig{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		MaxSize:    10,
		MaxAge:     7,
		MaxBackups: 3,
	}
}

func TestNewStdoutHasNoopCloser(t *testing.T) {
	lg, closer, err := New(baseConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if lg == nil {
		t.Fatal("nil logger")
	}
	if _, ok := closer.(nopCloser); !ok {
		t.Errorf("stdout closer = %T, want nopCloser", closer)
	}
	if err := closer.Close(); err != nil {
		t.Errorf("nopCloser.Close: %v", err)
	}
}

func TestNewFileWritesAndCloses(t *testing.T) {
	dir := t.TempDir()
	cfg := baseConfig()
	cfg.Output = "file"
	cfg.FilePath = filepath.Join(dir, "logs")

	lg, closer, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lg.Info("hello", "k", "v")

	path := filepath.Join(cfg.FilePath, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), `"msg":"hello"`) || !strings.Contains(string(data), `"k":"v"`) {
		t.Errorf("log line missing fields: %s", data)
	}
	if err := closer.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewTextFormat(t *testing.T) {
	dir := t.TempDir()
	cfg := baseConfig()
	cfg.Format = "text"
	cfg.Output = "file"
	cfg.FilePath = dir

	lg, closer, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer.Close()
	lg.Warn("careful")

	data, _ := os.ReadFile(filepath.Join(dir, logFileName))
	if !strings.Contains(string(data), "level=WARN") || !strings.Contains(string(data), `msg=careful`) {
		t.Errorf("text handler output unexpected: %s", data)
	}
}

func TestNewAddSource(t *testing.T) {
	dir := t.TempDir()
	cfg := baseConfig()
	cfg.Output = "file"
	cfg.FilePath = dir
	cfg.AddSource = true

	lg, closer, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer.Close()
	lg.Info("traced")

	data, _ := os.ReadFile(filepath.Join(dir, logFileName))
	if !strings.Contains(string(data), `"source"`) || !strings.Contains(string(data), `main_test.go`) {
		t.Errorf("expected source location in output: %s", data)
	}
}

func TestNewRejectsBadValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*config.LoggerConfig)
	}{
		{"level", func(c *config.LoggerConfig) { c.Level = "loud" }},
		{"format", func(c *config.LoggerConfig) { c.Format = "xml" }},
		{"output", func(c *config.LoggerConfig) { c.Output = "carrier-pigeon" }},
		{"file without path", func(c *config.LoggerConfig) { c.Output = "file"; c.FilePath = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseConfig()
			tc.mut(&cfg)
			if _, _, err := New(cfg); err == nil {
				t.Fatalf("expected error for bad %s", tc.name)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]string{
		"debug": "DEBUG", "INFO": "INFO", "warn": "WARN",
		"warning": "WARN", "error": "ERROR", "": "INFO",
	} {
		got, err := parseLevel(in)
		if err != nil {
			t.Fatalf("parseLevel(%q): %v", in, err)
		}
		if got.String() != want {
			t.Errorf("parseLevel(%q) = %s, want %s", in, got, want)
		}
	}
}
