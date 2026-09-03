package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loadInto runs Load against files inside a fresh temp dir. It always sets a
// real DATABASE_PASSWORD so the secret check passes; callers add more env via
// t.Setenv before calling.
func loadInto(t *testing.T) (*Config, string, string) {
	t.Helper()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "config.yaml")
	envFile := filepath.Join(dir, ".env")
	t.Setenv("DATABASE_PASSWORD", "testpass")

	cfg, err := Load(yamlFile, envFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg, yamlFile, envFile
}

func TestLoadDefaults(t *testing.T) {
	cfg, yamlFile, envFile := loadInto(t)

	if !fileExists(yamlFile) || !fileExists(envFile) {
		t.Fatalf("Load did not generate both files")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout.Std() != 15*time.Second {
		t.Errorf("Server.ReadTimeout = %s, want 15s", cfg.Server.ReadTimeout)
	}
	if cfg.Auth.SessionTTL.Std() != 168*time.Hour {
		t.Errorf("Auth.SessionTTL = %s, want 168h", cfg.Auth.SessionTTL)
	}
	if cfg.Worker.Concurrency != 2 {
		t.Errorf("Worker.Concurrency = %d, want 2", cfg.Worker.Concurrency)
	}
	if got := cfg.Server.CORSAllowedOrigins; len(got) != 1 || got[0] != "http://localhost:5173" {
		t.Errorf("CORSAllowedOrigins = %v, want [http://localhost:5173]", got)
	}
	if !contains(cfg.Upload.AllowedVideo, ".mp4") {
		t.Errorf("Upload.AllowedVideo = %v, missing .mp4", cfg.Upload.AllowedVideo)
	}
	if cfg.Mailer.Transport != "log" {
		t.Errorf("Mailer.Transport = %q, want log", cfg.Mailer.Transport)
	}
}

func TestHMACSecretGenerated(t *testing.T) {
	cfg, _, envFile := loadInto(t)

	if cfg.Auth.HMACSecret == "" || cfg.Auth.HMACSecret == placeholderSecret {
		t.Fatalf("HMACSecret not generated: %q", cfg.Auth.HMACSecret)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "AUTH_HMAC_SECRET=") {
		t.Fatalf(".env missing AUTH_HMAC_SECRET line:\n%s", data)
	}
	if strings.Contains(string(data), "AUTH_HMAC_SECRET="+placeholderSecret) {
		t.Fatal("AUTH_HMAC_SECRET was not randomly generated")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVER_READ_TIMEOUT", "5s")
	t.Setenv("SERVER_CORS_ALLOWED_ORIGINS", "http://a.test, http://b.test")
	t.Setenv("APP_DEBUG", "false")

	cfg, _, _ := loadInto(t)

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout.Std() != 5*time.Second {
		t.Errorf("Server.ReadTimeout = %s, want 5s", cfg.Server.ReadTimeout)
	}
	if len(cfg.Server.CORSAllowedOrigins) != 2 {
		t.Errorf("CORSAllowedOrigins = %v, want 2 entries", cfg.Server.CORSAllowedOrigins)
	}
	if cfg.App.Debug {
		t.Errorf("App.Debug = true, want false")
	}
}

func TestMissingSecretFails(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(filepath.Join(dir, "config.yaml"), filepath.Join(dir, ".env"))
	if err == nil {
		t.Fatal("expected Load to fail with placeholder DATABASE_PASSWORD")
	}
}

func TestValidateRejectsBadEnum(t *testing.T) {
	cfg, _, _ := loadInto(t)
	cfg.Logger.Level = "loud"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected Validate to reject logger.level=loud")
	}
}

func TestRegenerateDoesNotClobberEnv(t *testing.T) {
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "config.yaml")
	envFile := filepath.Join(dir, ".env")
	t.Setenv("DATABASE_PASSWORD", "testpass")

	if _, err := Load(yamlFile, envFile); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	before, _ := os.ReadFile(envFile)

	if err := os.Remove(yamlFile); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(yamlFile, envFile); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	after, _ := os.ReadFile(envFile)

	if string(before) != string(after) {
		t.Errorf(".env changed across regeneration:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
