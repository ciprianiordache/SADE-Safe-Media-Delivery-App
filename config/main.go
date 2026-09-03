package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// placeholderSecret is written for a required secret that could not be
// collected (non-interactive run). Load rejects it, so startup fails loudly
// until a real value is put in the .env file.
const placeholderSecret = "CHANGE_ME"

// Defaults returns a Config populated entirely from the `default` struct
// tags, without touching any file, env var, or secret. It is for standalone
// tools (e.g. cmd/watermark) and tests that need a section like FFmpeg
// without the full Load pipeline.
func Defaults() *Config {
	c := &Config{}
	applyDefaults(reflect.ValueOf(c).Elem())
	return c
}

// Load resolves configuration from a YAML file plus a .env file, generating
// whichever is missing (non-secret defaults into the YAML, secrets into the
// .env), then layers the sources: struct defaults < YAML < environment.
// It fails if a required secret is empty or still the placeholder, or if
// Validate finds an inconsistency.
func Load(yamlFile, envFile string) (*Config, error) {
	cfg := &Config{}

	if err := generateConfig(yamlFile, envFile); err != nil {
		return nil, err
	}

	root := reflect.ValueOf(cfg).Elem()
	applyDefaults(root)

	if fileExists(envFile) {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("load %s: %w", envFile, err)
		}
	}
	if fileExists(yamlFile) {
		if err := loadYAML(yamlFile, cfg); err != nil {
			return nil, err
		}
	}

	loadEnvOverrides(root)

	if err := validateSecrets(cfg, ""); err != nil {
		return nil, err
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// generateConfig writes only the files that do not yet exist, so a hand-edited
// .env is never clobbered when config.yaml is regenerated (or vice versa).
func generateConfig(yamlFile, envFile string) error {
	root := reflect.ValueOf(&Config{}).Elem()

	if !fileExists(yamlFile) {
		tree := map[string]any{}
		buildYAMLTree(root, tree)
		data, err := yaml.Marshal(tree)
		if err != nil {
			return fmt.Errorf("marshal generated config: %w", err)
		}
		if err := writeFile(yamlFile, data, 0o644); err != nil {
			return err
		}
	}

	if !fileExists(envFile) {
		env := map[string]string{}
		collectSecrets(root, env, term.IsTerminal(int(os.Stdin.Fd())))

		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, env[k])
		}
		if err := writeFile(envFile, []byte(b.String()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// buildYAMLTree fills out with the non-secret fields of the struct v, keyed by
// yaml tag, using each field's `default` tag for the value. Secret fields are
// skipped entirely - they belong in the .env file only.
func buildYAMLTree(v reflect.Value, out map[string]any) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		name := f.Tag.Get("yaml")
		if name == "" || name == "-" {
			continue
		}
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			child := map[string]any{}
			buildYAMLTree(fv, child)
			out[name] = child
			continue
		}
		if f.Tag.Get("secret") == "true" {
			continue
		}
		out[name] = yamlDefault(fv, f.Tag.Get("default"))
	}
}

// yamlDefault converts a `default` tag string into a typed value suitable for
// YAML marshalling of a field of the same kind as fv.
func yamlDefault(fv reflect.Value, def string) any {
	if fv.Type() == durationType {
		if def == "" {
			return "0s"
		}
		if d, err := time.ParseDuration(def); err == nil {
			return d.String()
		}
		return def
	}
	switch fv.Kind() {
	case reflect.Bool:
		b, _ := strconv.ParseBool(def)
		return b
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, _ := strconv.Atoi(def)
		return n
	case reflect.Float32, reflect.Float64:
		x, _ := strconv.ParseFloat(def, 64)
		return x
	case reflect.Slice:
		return splitList(def)
	default:
		return def
	}
}

// collectSecrets walks v for `secret:"true"` fields and produces an env-var
// name -> value map. A `generate:"rand32"` field gets fresh random bytes; any
// other secret is prompted for on an interactive terminal and otherwise set
// to the placeholder.
func collectSecrets(v reflect.Value, out map[string]string, interactive bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			collectSecrets(fv, out, interactive)
			continue
		}
		if f.Tag.Get("secret") != "true" {
			continue
		}
		key := f.Tag.Get("env")
		if key == "" {
			key = strings.ToUpper(f.Tag.Get("yaml"))
		}
		switch {
		case f.Tag.Get("generate") != "":
			out[key] = generateSecret(f.Tag.Get("generate"))
		case interactive:
			out[key] = promptSecret(key)
		default:
			out[key] = placeholderSecret
		}
	}
}

// applyDefaults sets every zero-valued field of v to its `default` tag, so a
// config.yaml written by an older build (missing newer keys) still yields a
// fully populated struct.
func applyDefaults(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			applyDefaults(fv)
			continue
		}
		def := f.Tag.Get("default")
		if def == "" || !fv.IsZero() {
			continue
		}
		setScalar(fv, def)
	}
}

// loadEnvOverrides replaces any field whose `env` variable is set in the
// process environment.
func loadEnvOverrides(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			loadEnvOverrides(fv)
			continue
		}
		key := f.Tag.Get("env")
		if key == "" {
			continue
		}
		val, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if f.Tag.Get("secret") == "true" && val == "" {
			continue
		}
		setScalar(fv, val)
	}
}

// setScalar parses raw into fv according to fv's type. Unparseable values are
// left untouched rather than zeroed.
func setScalar(fv reflect.Value, raw string) {
	if fv.Type() == durationType {
		if d, err := time.ParseDuration(raw); err == nil {
			fv.SetInt(int64(d))
		} else if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			fv.SetInt(n)
		}
		return
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		if b, err := strconv.ParseBool(raw); err == nil {
			fv.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			fv.SetInt(n)
		}
	case reflect.Float32, reflect.Float64:
		if x, err := strconv.ParseFloat(raw, 64); err == nil {
			fv.SetFloat(x)
		}
	case reflect.Slice:
		parts := splitList(raw)
		out := reflect.MakeSlice(fv.Type(), 0, len(parts))
		for _, p := range parts {
			out = reflect.Append(out, reflect.ValueOf(p).Convert(fv.Type().Elem()))
		}
		fv.Set(out)
	}
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validateSecrets fails if any `secret:"true"` field is empty or still the
// placeholder.
func validateSecrets(cfg any, path string) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		childPath := f.Name
		if path != "" {
			childPath = path + "." + f.Name
		}
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			if err := validateSecrets(fv.Addr().Interface(), childPath); err != nil {
				return err
			}
			continue
		}
		if f.Tag.Get("secret") != "true" {
			continue
		}
		if fv.Kind() != reflect.String {
			return fmt.Errorf("secret field %s must be a string", childPath)
		}
		switch strings.TrimSpace(fv.String()) {
		case "":
			return fmt.Errorf("missing secret: %s (set it in the .env file)", childPath)
		case placeholderSecret:
			return fmt.Errorf("secret %s is still %q; set a real value in the .env file", childPath, placeholderSecret)
		}
	}
	return nil
}

// Validate checks cross-field consistency and enum membership, aggregating all
// problems into one error.
func Validate(cfg *Config) error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	if !oneOf(cfg.App.Environment, "development", "staging", "production", "test") {
		add("app.environment %q must be development|staging|production|test", cfg.App.Environment)
	}
	if cfg.App.PublicURL == "" || cfg.App.FrontendURL == "" {
		add("app.public_url and app.frontend_url must both be set")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		add("server.port %d is out of range 1..65535", cfg.Server.Port)
	}
	if cfg.Server.TLS.Enabled && (cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "") {
		add("server.tls.enabled is true but cert_file/key_file are not both set")
	}
	if !oneOf(cfg.Database.Driver, "pgx", "postgres") {
		add("database.driver %q must be pgx or postgres", cfg.Database.Driver)
	}
	if !oneOf(cfg.Logger.Level, "debug", "info", "warn", "error") {
		add("logger.level %q must be debug|info|warn|error", cfg.Logger.Level)
	}
	if !oneOf(cfg.Logger.Format, "json", "text") {
		add("logger.format %q must be json|text", cfg.Logger.Format)
	}
	if !oneOf(cfg.Logger.Output, "stdout", "file", "both") {
		add("logger.output %q must be stdout|file|both", cfg.Logger.Output)
	}
	if !oneOf(cfg.Storage.Driver, "local", "s3") {
		add("storage.driver %q must be local or s3", cfg.Storage.Driver)
	}
	if cfg.Storage.Driver == "local" && cfg.Storage.LocalPath == "" {
		add("storage.local_path must be set when driver is local")
	}
	if cfg.Storage.Driver == "s3" && (cfg.Storage.S3Bucket == "" || cfg.Storage.S3Region == "") {
		add("storage.s3_bucket and storage.s3_region must be set when driver is s3")
	}
	if !oneOf(cfg.Mailer.Transport, "smtp", "log", "noop") {
		add("mailer.transport %q must be smtp|log|noop", cfg.Mailer.Transport)
	}
	if cfg.Mailer.Transport == "smtp" {
		if cfg.Mailer.SMTP.Host == "" || cfg.Mailer.SMTP.Port == 0 {
			add("mailer.transport is smtp but smtp.host/smtp.port are not set")
		}
		if cfg.Mailer.SMTP.User == "" || cfg.Mailer.SMTP.Password == "" {
			add("mailer.transport is smtp but smtp.user/smtp.password are not set")
		}
	}
	if cfg.Mailer.FromAddr == "" {
		add("mailer.from_addr must be set")
	}
	if !oneOf(cfg.FFmpeg.Position, "top-left", "top-right", "bottom-left", "bottom-right", "center", "tiled") {
		add("ffmpeg.position %q is invalid", cfg.FFmpeg.Position)
	}
	if cfg.FFmpeg.OpacityPct < 0 || cfg.FFmpeg.OpacityPct > 100 {
		add("ffmpeg.opacity_pct %d is out of range 0..100", cfg.FFmpeg.OpacityPct)
	}
	if cfg.Worker.Concurrency < 1 {
		add("worker.concurrency must be >= 1")
	}
	if cfg.Worker.MaxRetries < 0 {
		add("worker.max_retries must be >= 0")
	}
	if cfg.Upload.MaxSizeMiB < 1 {
		add("upload.max_size_mib must be >= 1")
	}
	if cfg.Upload.MemoryBufMiB < 1 {
		add("upload.memory_buf_mib must be >= 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func oneOf(v string, allowed ...string) bool {
	return slices.Contains(allowed, v)
}

func loadYAML(file string, cfg *Config) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse %s: %w", file, err)
	}
	return nil
}

// MaskSecrets overwrites every non-empty secret string with "******", for
// safe logging of the resolved config.
func MaskSecrets(cfg any) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct && fv.Type() != durationType {
			MaskSecrets(fv.Addr().Interface())
			continue
		}
		if f.Tag.Get("secret") == "true" && fv.Kind() == reflect.String && fv.String() != "" {
			fv.SetString("******")
		}
	}
}

func generateSecret(kind string) string {
	size := 32
	if n, err := strconv.Atoi(strings.TrimPrefix(kind, "rand")); err == nil && n > 0 {
		size = n
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return placeholderSecret
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func promptSecret(key string) string {
	fmt.Printf("Enter value for %s: ", key)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return placeholderSecret
	}
	if v := strings.TrimSpace(string(b)); v != "" {
		return v
	}
	return placeholderSecret
}
