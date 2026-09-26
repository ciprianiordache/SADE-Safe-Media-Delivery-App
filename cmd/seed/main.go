// Command seed wipes the SADE database + blob storage and fills them with a
// small, believable demo dataset - for screenshots and demos, never for a
// real deployment.
//
//	go run ./cmd/seed            # wipe + seed (asks for no confirmation)
//	go run ./cmd/seed -links     # print /preview links for the done jobs
//
// Seeding generates its media locally with ffmpeg's synthetic sources
// (mandelbrot, gradients, sine), then uploads each file through the real
// job.Service.Create path, so every original is a genuine, ffprobe-verified
// asset. The jobs meant to end up "done" are left pending for the app's own
// worker to watermark on its next start; the rest are pinned to their demo
// status (failed / processing / pending-far-in-the-future) so the worker
// never touches them. Recipients of the non-final jobs use example.com, so a
// later run with a real SMTP transport cannot email a stranger.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"sade/config"
	"sade/internals/app"
	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/payment"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/ffmpeg"
	"sade/internals/storage"
	"sade/internals/token"
)

type demo struct {
	file      string
	recipient string
	kind      string // watermark kind
	text      string // watermark text override; "" = template
	status    string // final demo status
	errMsg    string
	attempts  int
	age       time.Duration // created_at = now - age
	paid      bool
	gen       []string // ffmpeg args producing the file (output path appended)
}

var (
	vid = func(src string) []string {
		return []string{"-f", "lavfi", "-i", src, "-f", "lavfi", "-i", "sine=frequency=220:sample_rate=44100", "-t", "8",
			"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"}
	}
	img = func(src string) []string { return []string{"-f", "lavfi", "-i", src, "-frames:v", "1"} }
	aud = func(expr string) []string { return []string{"-f", "lavfi", "-i", expr, "-t", "20"} }
)

var demos = []demo{
	{file: "brand-film-final-v3.mp4", recipient: "andreea.munteanu@studionord.ro", kind: job.WatermarkBoth, status: job.StatusDone, age: 14 * time.Minute, attempts: 1,
		gen: vid("mandelbrot=size=1280x720:rate=25:maxiter=4096")},
	{file: "drone-coastline-constanta.mp4", recipient: "mihai.dobre@bluewave.media", kind: job.WatermarkLogo, status: job.StatusDone, age: 3 * time.Hour, attempts: 1, paid: true,
		gen: vid("gradients=size=1280x720:rate=25:speed=0.03:n=5:seed=7")},
	{file: "podcast-ep12-master.wav", recipient: "irina.stan@vocea.fm", kind: job.WatermarkBoth, status: job.StatusDone, age: 9 * time.Hour, attempts: 1,
		gen: aud("aevalsrc=0.25*sin(2*PI*330*t)*(0.6+0.4*sin(2*PI*0.5*t))|0.25*sin(2*PI*440*t)*(0.6+0.4*sin(2*PI*0.3*t)):s=44100")},
	{file: "campaign-poster-a2.png", recipient: "radu.ionescu@agentiaverde.ro", kind: job.WatermarkText, text: "CONFIDENTIAL · Agentia Verde", status: job.StatusDone, age: 26 * time.Hour, attempts: 1, paid: true,
		gen: img("mandelbrot=size=1600x1000:start_scale=1.2:maxiter=2048")},
	{file: "headshot-elena-retouched.jpg", recipient: "elena.vasile@portofoliu.ro", kind: job.WatermarkBoth, status: job.StatusDone, age: 2*24*time.Hour + 5*time.Hour, attempts: 1,
		gen: img("gradients=size=1200x1500:n=4:seed=21")},
	{file: "jingle-radio-30s.mp3", recipient: "contact@radiocluj.ro", kind: job.WatermarkLogo, status: job.StatusDone, age: 4 * 24 * time.Hour, attempts: 2,
		gen: aud("aevalsrc=0.3*sin(2*PI*(262+131*floor(mod(t*2\\,4)))*t):s=44100")},
	{file: "product-teaser-60s.mp4", recipient: "cristina.pop@lumenstudio.ro", kind: job.WatermarkText, text: "Preview · Lumen Studio", status: job.StatusDone, age: 6 * 24 * time.Hour, attempts: 1,
		gen: vid("gradients=size=1280x720:rate=25:speed=0.05:n=3:seed=3")},
	{file: "label-mockup-v2.webp", recipient: "office@cramadealu.ro", kind: job.WatermarkLogo, status: job.StatusDone, age: 9 * 24 * time.Hour, attempts: 1,
		gen: img("mandelbrot=size=1400x900:start_x=-0.743643887037151:start_y=0.13182590420533:start_scale=0.005:maxiter=4096")},
	{file: "wedding-highlights-4k.mp4", recipient: "ana.georgescu@example.com", kind: job.WatermarkBoth, status: job.StatusProcessing, age: 2 * time.Minute, attempts: 1,
		gen: vid("mandelbrot=size=1280x720:rate=25:start_scale=2:maxiter=2048")},
	{file: "interview-raw-take4.mov", recipient: "bogdan.marin@example.com", kind: job.WatermarkBoth, status: job.StatusPending, age: 40 * time.Second,
		gen: vid("gradients=size=1280x720:rate=25:speed=0.02:n=6:seed=11")},
	{file: "concert-live-sala-palatului.mp4", recipient: "tickets@example.com", kind: job.WatermarkText, text: "Live · Sala Palatului", status: job.StatusFailed, age: 30 * time.Hour, attempts: 4,
		errMsg: "watermark: ffmpeg exited with status 1: Invalid data found when processing input",
		gen:    vid("gradients=size=1280x720:rate=25:speed=0.04:n=2:seed=5")},
	{file: "voiceover-spot-tv.flac", recipient: "studio@example.com", kind: job.WatermarkLogo, status: job.StatusFailed, age: 3*24*time.Hour + 2*time.Hour, attempts: 4,
		errMsg: "watermark: context deadline exceeded",
		gen:    aud("sine=frequency=523:sample_rate=44100")},
}

const operatorEmail = "ciipriian5521@gmail.com"

func main() {
	links := flag.Bool("links", false, "print /preview links for done jobs instead of seeding")
	flag.Parse()
	if err := run(*links); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(linksOnly bool) error {
	ctx := context.Background()
	cfg, err := config.Load("config.yaml", ".env")
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db := database.New(cfg.Database, log)
	if err := db.Connect(ctx); err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(app.Models()...); err != nil {
		return err
	}
	signer, err := token.New(cfg.Auth.HMACSecret)
	if err != nil {
		return err
	}
	if linksOnly {
		return printLinks(db, cfg, signer)
	}

	// Wipe: every table, then every stored blob.
	if _, err := db.Exec(`TRUNCATE users, magic_tokens, sessions, jobs, assets, payments CASCADE`); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	for _, dir := range []string{"originals", "previews"} {
		if err := os.RemoveAll(filepath.Join(cfg.Storage.LocalPath, dir)); err != nil {
			return err
		}
	}
	fmt.Println("wiped database and storage")

	store, err := storage.New(cfg.Storage, log)
	if err != nil {
		return err
	}
	engine, err := ffmpeg.New(cfg.FFmpeg, log)
	if err != nil {
		return err
	}
	users := user.NewService(user.NewRepo(db), log)
	jobRepo := job.NewRepo(db)
	jobs := job.NewService(jobRepo, asset.NewRepo(db), store, engine, cfg.Upload, log)
	payments := payment.NewRepo(db)

	op, err := users.EnsureByEmail(operatorEmail)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET created_at = $1 WHERE id = $2`, time.Now().Add(-21*24*time.Hour), op.ID); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "sade-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	now := time.Now()
	for _, d := range demos {
		path := filepath.Join(tmp, d.file)
		args := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, d.gen...)
		if out, err := exec.CommandContext(ctx, cfg.FFmpeg.BinPath, append(args, path)...).CombinedOutput(); err != nil {
			return fmt.Errorf("generate %s: %v: %s", d.file, err, out)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		res, err := jobs.Create(ctx, op.ID, job.NewUpload{
			RecipientEmail: d.recipient, WatermarkKind: d.kind, WatermarkText: d.text,
			Filename: d.file, File: f,
		})
		f.Close()
		if err != nil {
			return fmt.Errorf("create %s: %w", d.file, err)
		}

		created := now.Add(-d.age)
		// Done jobs stay pending (ready now) for the worker; everything else is
		// pinned: next_attempt_at a year out keeps the worker's claim away.
		status, next := job.StatusPending, time.Time{}
		if d.status != job.StatusDone {
			status, next = d.status, now.AddDate(1, 0, 0)
		}
		if _, err := db.Exec(`UPDATE jobs SET status=$1, error=$2, attempts=$3, next_attempt_at=$4, created_at=$5, updated_at=$6 WHERE id=$7`,
			status, d.errMsg, max(d.attempts-1, 0), next, created, created.Add(40*time.Second), res.ID); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE assets SET created_at=$1 WHERE job_id=$2`, created, res.ID); err != nil {
			return err
		}
		if d.paid {
			if _, err := payments.Create(&payment.Payment{
				JobID: res.ID, Provider: payment.ProviderStripe, ProviderRef: "pi_demo_" + res.ID[:8],
				Status: payment.StatusPaid, AmountCents: cfg.Payment.PriceCents, Currency: cfg.Payment.Currency,
			}); err != nil {
				return err
			}
		}
		fmt.Printf("  %-10s %-34s -> %s\n", d.status, d.file, d.recipient)
	}
	fmt.Printf("seeded %d jobs for %s; start the app so the worker watermarks the done ones\n", len(demos), operatorEmail)
	return nil
}

func printLinks(db *database.Database, cfg *config.Config, signer *token.Signer) error {
	rows, err := db.Query(`SELECT a.id, a.filename, j.recipient_email FROM assets a JOIN jobs j ON j.id = a.job_id
		WHERE a.kind = $1 ORDER BY j.created_at DESC`, asset.KindPreview)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, rcpt string
		if err := rows.Scan(&id, &name, &rcpt); err != nil {
			return err
		}
		tok, err := signer.Sign("preview", id, cfg.Auth.ShareTokenTTL.Std())
		if err != nil {
			return err
		}
		fmt.Printf("%s (%s)\n  %s/preview/%s\n", name, rcpt, cfg.App.PublicURL, tok)
	}
	return rows.Err()
}
