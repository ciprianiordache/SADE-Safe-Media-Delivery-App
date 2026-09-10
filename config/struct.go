package config

import (
	"fmt"
	"reflect"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration so it round-trips as a human string ("15s",
// "30m", "1h") in both YAML and environment variables. An integer is also
// accepted and read as a nanosecond count. Call Std to get a time.Duration.
type Duration time.Duration

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

// MarshalYAML renders the value as a duration string.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// UnmarshalYAML accepts either a duration string or an integer nanosecond count.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err == nil {
		parsed, perr := time.ParseDuration(s)
		if perr != nil {
			return fmt.Errorf("invalid duration %q: %w", s, perr)
		}
		*d = Duration(parsed)
		return nil
	}
	var n int64
	if err := node.Decode(&n); err != nil {
		return fmt.Errorf(`duration must be a string like "15s" or an integer nanosecond count`)
	}
	*d = Duration(n)
	return nil
}

// durationType is used by the reflection-based loader to recognise Duration
// fields (whose Kind is Int64) and parse their values with time.ParseDuration.
var durationType = reflect.TypeFor[Duration]()

// Config holds the entire configuration for the application.
type Config struct {
	App      AppConfig      `yaml:"app"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logger   LoggerConfig   `yaml:"logger"`
	Storage  StorageConfig  `yaml:"storage"`
	Mailer   MailerConfig   `yaml:"mailer"`
	Auth     AuthConfig     `yaml:"auth"`
	FFmpeg   FFmpegConfig   `yaml:"ffmpeg"`
	Worker   WorkerConfig   `yaml:"worker"`
	Upload   UploadConfig   `yaml:"upload"`
}

// AppConfig holds top-level application settings.
//
// PublicURL is the externally reachable base URL of the Go API - it is what
// goes into the links SADE emails (magic-link callback, share links), so it
// must point at this server, not the frontend. FrontendURL is where the API
// redirects a browser after a successful magic-link login (the Vite dev
// server in development; PublicURL itself once the Go binary serves the
// built frontend). DataDir is the single root under which storage, logs and
// scratch space live by default. FrontendDir is the SvelteKit adapter-static
// build the router serves as a SPA fallback for any non-API path; a missing
// directory (frontend not built yet) just disables that handler.
type AppConfig struct {
	Name        string `yaml:"name" env:"APP_NAME" default:"SADE (Safe Media Delivery)"`
	Version     string `yaml:"version" env:"APP_VERSION" default:"v0.1.0"`
	Environment string `yaml:"environment" env:"APP_ENVIRONMENT" default:"development"` // development | staging | production | test
	Debug       bool   `yaml:"debug" env:"APP_DEBUG" default:"true"`                    // forces logger level to debug
	PublicURL   string `yaml:"public_url" env:"APP_PUBLIC_URL" default:"http://localhost:8080"`
	FrontendURL string `yaml:"frontend_url" env:"APP_FRONTEND_URL" default:"http://localhost:5173"`
	DataDir     string `yaml:"data_dir" env:"APP_DATA_DIR" default:"./.data"`
	TempDir     string `yaml:"temp_dir" env:"APP_TEMP_DIR" default:"./.data/tmp"`
	FrontendDir string `yaml:"frontend_dir" env:"APP_FRONTEND_DIR" default:"./frontend/build"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Host               string    `yaml:"host" env:"SERVER_HOST" default:"localhost"`
	Port               int       `yaml:"port" env:"SERVER_PORT" default:"8080"`
	ReadTimeout        Duration  `yaml:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"15s"`
	WriteTimeout       Duration  `yaml:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"15s"`
	IdleTimeout        Duration  `yaml:"idle_timeout" env:"SERVER_IDLE_TIMEOUT" default:"60s"`
	ShutdownTimeout    Duration  `yaml:"shutdown_timeout" env:"SERVER_SHUTDOWN_TIMEOUT" default:"15s"`
	MaxHeaderBytes     int       `yaml:"max_header_bytes" env:"SERVER_MAX_HEADER_BYTES" default:"1048576"` // 1 MiB
	CORSAllowedOrigins []string  `yaml:"cors_allowed_origins" env:"SERVER_CORS_ALLOWED_ORIGINS" default:"http://localhost:5173"`
	TLS                TLSConfig `yaml:"tls"`
}

// TLSConfig configures optional inbound TLS. None of these are secret values
// (a flag and two file paths); the private key material lives in KeyFile.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled" env:"SERVER_TLS_ENABLED" default:"false"`
	CertFile string `yaml:"cert_file" env:"SERVER_TLS_CERT_FILE"`
	KeyFile  string `yaml:"key_file" env:"SERVER_TLS_KEY_FILE"`
}

// DatabaseConfig holds the PostgreSQL connection settings. Driver "pgx" is
// github.com/jackc/pgx/v5/stdlib registered under that name.
type DatabaseConfig struct {
	Driver          string   `yaml:"driver" env:"DATABASE_DRIVER" default:"pgx"`
	Host            string   `yaml:"host" env:"DATABASE_HOST" default:"localhost"`
	Port            int      `yaml:"port" env:"DATABASE_PORT" default:"5433"` // docker-compose maps 5433:5432 to dodge a local 5432
	User            string   `yaml:"user" env:"DATABASE_USER" default:"postgres"`
	Password        string   `yaml:"password" env:"DATABASE_PASSWORD" secret:"true"`
	Name            string   `yaml:"name" env:"DATABASE_NAME" default:"sade"`
	SSLMode         string   `yaml:"ssl_mode" env:"DATABASE_SSL_MODE" default:"disable"`
	Timezone        string   `yaml:"timezone" env:"DATABASE_TIMEZONE" default:"UTC"`
	MaxOpenConns    int      `yaml:"max_open_conns" env:"DATABASE_MAX_OPEN_CONNS" default:"20"`
	MaxIdleConns    int      `yaml:"max_idle_conns" env:"DATABASE_MAX_IDLE_CONNS" default:"10"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime" env:"DATABASE_CONN_MAX_LIFETIME" default:"1h"`
	ConnMaxIdleTime Duration `yaml:"conn_max_idle_time" env:"DATABASE_CONN_MAX_IDLE_TIME" default:"30m"`
}

// LoggerConfig configures the slog logger built by internals/logger.
type LoggerConfig struct {
	Level      string `yaml:"level" env:"LOGGER_LEVEL" default:"info"`   // debug | info | warn | error
	Format     string `yaml:"format" env:"LOGGER_FORMAT" default:"json"` // json | text
	Output     string `yaml:"output" env:"LOGGER_OUTPUT" default:"both"` // stdout | file | both
	FilePath   string `yaml:"file_path" env:"LOGGER_FILE_PATH" default:"./.data/logs"`
	MaxSize    int    `yaml:"max_size" env:"LOGGER_MAX_SIZE" default:"50"` // MiB per file before rotation
	MaxAge     int    `yaml:"max_age" env:"LOGGER_MAX_AGE" default:"30"`   // days
	MaxBackups int    `yaml:"max_backups" env:"LOGGER_MAX_BACKUPS" default:"10"`
	Compress   bool   `yaml:"compress" env:"LOGGER_COMPRESS" default:"true"`
	AddSource  bool   `yaml:"add_source" env:"LOGGER_ADD_SOURCE" default:"false"` // include caller file:line
}

// StorageConfig selects and configures the blob store for originals and
// previews. Driver "local" uses LocalPath; "s3" uses the S3* fields and reads
// credentials from the standard AWS credential chain (env, profile, instance
// role) - there are deliberately no secret fields here.
type StorageConfig struct {
	Driver     string `yaml:"driver" env:"STORAGE_DRIVER" default:"local"` // local | s3
	LocalPath  string `yaml:"local_path" env:"STORAGE_LOCAL_PATH" default:"./.data/storage"`
	S3Bucket   string `yaml:"s3_bucket" env:"STORAGE_S3_BUCKET"`
	S3Region   string `yaml:"s3_region" env:"STORAGE_S3_REGION"`
	S3Endpoint string `yaml:"s3_endpoint" env:"STORAGE_S3_ENDPOINT"` // set for MinIO / non-AWS
}

// MailerConfig configures outbound email. Transport "log" (the default in
// development) renders each message to the logger instead of sending it -
// that is how magic-link login works locally without an SMTP account.
// "noop" drops messages silently; "smtp" sends via SMTP.
type MailerConfig struct {
	Transport string     `yaml:"transport" env:"MAILER_TRANSPORT" default:"log"` // smtp | log | noop
	SMTP      SMTPConfig `yaml:"smtp"`
	FromName  string     `yaml:"from_name" env:"MAILER_FROM_NAME" default:"Safe Media Delivery"`
	FromAddr  string     `yaml:"from_addr" env:"MAILER_FROM_ADDR" default:"no-reply@sade.local"`
}

// SMTPConfig holds SMTP credentials. Password is not tagged secret because it
// is only required when Mailer.Transport == "smtp"; that dependency is
// enforced by Config.Validate rather than the unconditional secret check.
type SMTPConfig struct {
	Host     string `yaml:"host" env:"SMTP_HOST" default:"localhost"`
	Port     int    `yaml:"port" env:"SMTP_PORT" default:"587"`
	User     string `yaml:"user" env:"SMTP_USER"`
	Password string `yaml:"password" env:"SMTP_PASSWORD"`
}

// AuthConfig configures magic-link login and signed share tokens. HMACSecret
// signs share tokens and is auto-generated (32 random bytes) on first run;
// callers must domain-separate uses (prefix the signed message with a context
// string such as "share" or "session").
type AuthConfig struct {
	HMACSecret          string   `yaml:"hmac_secret" env:"AUTH_HMAC_SECRET" secret:"true" generate:"rand32"`
	SessionTTL          Duration `yaml:"session_ttl" env:"AUTH_SESSION_TTL" default:"168h"` // 7 days
	MagicLinkTTL        Duration `yaml:"magic_link_ttl" env:"AUTH_MAGIC_LINK_TTL" default:"15m"`
	ShareTokenTTL       Duration `yaml:"share_token_ttl" env:"AUTH_SHARE_TOKEN_TTL" default:"720h"` // 30 days; signed /p and /d links
	SessionCookieName   string   `yaml:"session_cookie_name" env:"AUTH_SESSION_COOKIE_NAME" default:"sade_session"`
	SessionCookieSecure bool     `yaml:"session_cookie_secure" env:"AUTH_SESSION_COOKIE_SECURE" default:"false"` // true behind HTTPS
}

// FFmpegConfig configures the watermark engine. It owns binary locations, the
// watermark asset paths and the render/encode knobs - it does NOT own storage
// directories (the worker hands it explicit input/output paths). Relative
// asset paths resolve against App.DataDir.
type FFmpegConfig struct {
	BinPath   string `yaml:"bin_path" env:"FFMPEG_BIN_PATH" default:"ffmpeg"` // looked up on PATH when not absolute
	ProbePath string `yaml:"probe_path" env:"FFMPEG_PROBE_PATH" default:"ffprobe"`

	LogoPath           string `yaml:"logo_path" env:"FFMPEG_LOGO_PATH" default:"./assets/watermark/logo.png"` // PNG overlay: image + video
	AudioWatermarkPath string `yaml:"audio_watermark_path" env:"FFMPEG_AUDIO_WATERMARK_PATH" default:"./assets/watermark/audio.mp3"`
	FontPath           string `yaml:"font_path" env:"FFMPEG_FONT_PATH" default:"./assets/fonts/NotoSans-Regular.ttf"`

	Position     string `yaml:"position" env:"FFMPEG_POSITION" default:"bottom-right"` // top-left|top-right|bottom-left|bottom-right|center|tiled
	OpacityPct   int    `yaml:"opacity_pct" env:"FFMPEG_OPACITY_PCT" default:"35"`     // 0..100
	MarginPx     int    `yaml:"margin_px" env:"FFMPEG_MARGIN_PX" default:"24"`
	LogoWidthPx  int    `yaml:"logo_width_px" env:"FFMPEG_LOGO_WIDTH_PX" default:"240"` // 0 = native size
	TextTemplate string `yaml:"text_template" env:"FFMPEG_TEXT_TEMPLATE" default:"{recipient} · {date}"`
	FontSizePx   int    `yaml:"font_size_px" env:"FFMPEG_FONT_SIZE_PX" default:"18"`

	AudioIntervalSec int `yaml:"audio_interval_sec" env:"FFMPEG_AUDIO_INTERVAL_SEC" default:"20"` // repeat the audio tag every N seconds
	AudioGainDB      int `yaml:"audio_gain_db" env:"FFMPEG_AUDIO_GAIN_DB" default:"-18"`          // watermark level relative to source

	VideoCodec   string `yaml:"video_codec" env:"FFMPEG_VIDEO_CODEC" default:"libx264"`
	VideoCRF     int    `yaml:"video_crf" env:"FFMPEG_VIDEO_CRF" default:"23"`
	VideoPreset  string `yaml:"video_preset" env:"FFMPEG_VIDEO_PRESET" default:"veryfast"`
	AudioCodec   string `yaml:"audio_codec" env:"FFMPEG_AUDIO_CODEC" default:"aac"`
	AudioBitrate string `yaml:"audio_bitrate" env:"FFMPEG_AUDIO_BITRATE" default:"160k"`
}

// WorkerConfig configures the in-process job pool. ffmpeg is CPU-heavy so
// Concurrency stays low; the database is the source of truth and rows are
// claimed with FOR UPDATE SKIP LOCKED.
type WorkerConfig struct {
	Concurrency     int      `yaml:"concurrency" env:"WORKER_CONCURRENCY" default:"2"`
	PollInterval    Duration `yaml:"poll_interval" env:"WORKER_POLL_INTERVAL" default:"5s"`
	ClaimBatchSize  int      `yaml:"claim_batch_size" env:"WORKER_CLAIM_BATCH_SIZE" default:"10"`
	MaxRetries      int      `yaml:"max_retries" env:"WORKER_MAX_RETRIES" default:"3"`
	RetryBackoff    Duration `yaml:"retry_backoff" env:"WORKER_RETRY_BACKOFF" default:"30s"` // base, exponential per attempt
	JobTimeout      Duration `yaml:"job_timeout" env:"WORKER_JOB_TIMEOUT" default:"30m"`
	StuckJobTimeout Duration `yaml:"stuck_job_timeout" env:"WORKER_STUCK_JOB_TIMEOUT" default:"1h"` // reset stale "processing" rows on boot
	ShutdownGrace   Duration `yaml:"shutdown_grace" env:"WORKER_SHUTDOWN_GRACE" default:"1m"`
}

// UploadConfig bounds the multipart upload endpoint. Sizes are in MiB. The
// extension lists are the coarse allowlist; handlers additionally sniff the
// content type and validate with ffprobe.
type UploadConfig struct {
	MaxSizeMiB    int      `yaml:"max_size_mib" env:"UPLOAD_MAX_SIZE_MIB" default:"2048"`   // per-file cap (2 GiB)
	MemoryBufMiB  int      `yaml:"memory_buf_mib" env:"UPLOAD_MEMORY_BUF_MIB" default:"32"` // ParseMultipartForm in-memory threshold
	TempDir       string   `yaml:"temp_dir" env:"UPLOAD_TEMP_DIR" default:"./.data/tmp/uploads"`
	AllowedVideo  []string `yaml:"allowed_video" env:"UPLOAD_ALLOWED_VIDEO" default:".mp4,.mov,.mkv,.webm,.avi"`
	AllowedAudio  []string `yaml:"allowed_audio" env:"UPLOAD_ALLOWED_AUDIO" default:".mp3,.wav,.m4a,.aac,.flac,.ogg"`
	AllowedImage  []string `yaml:"allowed_image" env:"UPLOAD_ALLOWED_IMAGE" default:".jpg,.jpeg,.png,.webp,.tiff"`
	KeepOriginals bool     `yaml:"keep_originals" env:"UPLOAD_KEEP_ORIGINALS" default:"true"` // false = delete original once preview is ready
}
