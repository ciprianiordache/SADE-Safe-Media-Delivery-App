package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
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
