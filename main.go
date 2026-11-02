package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Every timestamp column is a Postgres TIMESTAMP (no time zone), and the
	// driver writes a time.Time's wall clock as-is. On a machine whose local
	// zone isn't UTC that stored local time, which then read back as UTC:
	// expiry checks ran hours late and the UI showed shifted times. Running
	// the process in UTC keeps what is written and what is read the same.
	time.Local = time.UTC
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// run owns the process lifecycle: it wires signal handling, builds the App,
// and guarantees App.Close runs (which os.Exit in main would skip).
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := NewApp(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()

	return app.Run(ctx)
}
