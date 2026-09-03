package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"sade/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() config.ServerConfig {
	return config.ServerConfig{
		Host:            "127.0.0.1",
		Port:            0, // OS-assigned
		ReadTimeout:     config.Duration(5 * time.Second),
		WriteTimeout:    config.Duration(5 * time.Second),
		IdleTimeout:     config.Duration(30 * time.Second),
		ShutdownTimeout: config.Duration(2 * time.Second),
		MaxHeaderBytes:  1 << 20,
	}
}

func TestStartServeShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	s := New(testConfig(), testLogger(), mux)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	resp, err := http.Get("http://" + s.Addr() + "/ping")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "pong" {
		t.Fatalf("got %d %q, want 200 \"pong\"", resp.StatusCode, body)
	}

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, err := http.Get("http://" + s.Addr() + "/ping"); err == nil {
		t.Fatal("server still accepting connections after Shutdown")
	}
}

func TestStartPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	cfg := testConfig()
	cfg.Port = port
	s := New(cfg, testLogger(), http.NewServeMux())

	if err := s.Start(); err == nil {
		_ = s.Shutdown(context.Background())
		t.Fatal("expected Start to fail on an occupied port")
	}
}

func TestShutdownBeforeStartIsNoop(t *testing.T) {
	s := New(testConfig(), testLogger(), http.NewServeMux())
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown before Start: %v", err)
	}
}

func TestDoubleStartFails(t *testing.T) {
	s := New(testConfig(), testLogger(), http.NewServeMux())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	if err := s.Start(); err == nil {
		t.Fatal("expected second Start to fail")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	s := New(testConfig(), testLogger(), http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- Run(ctx, s) }()

	// Give Run a moment to bind, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
