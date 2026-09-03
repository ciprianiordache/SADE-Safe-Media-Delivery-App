// Package server runs the SADE HTTP server. It is a small lifecycle wrapper
// around net/http: Start binds the listen socket synchronously (a taken port
// or missing permission is returned, not fatal), serves in the background,
// and Shutdown stops it within config.ServerConfig.ShutdownTimeout. Signal
// handling belongs to the caller (main.go, via signal.NotifyContext).
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"sade/config"
)

const defaultShutdownTimeout = 15 * time.Second

// New builds a Server for cfg. handler is the fully assembled router; New
// adds nothing to it.
func New(cfg config.ServerConfig, log *slog.Logger, handler http.Handler) *Server {
	return &Server{
		cfg: cfg,
		log: log,
		httpSrv: &http.Server{
			Addr:           net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
			Handler:        handler,
			ReadTimeout:    cfg.ReadTimeout.Std(),
			WriteTimeout:   cfg.WriteTimeout.Std(),
			IdleTimeout:    cfg.IdleTimeout.Std(),
			MaxHeaderBytes: cfg.MaxHeaderBytes,
			ErrorLog:       slog.NewLogLogger(log.Handler(), slog.LevelError),
		},
	}
}

// Addr is the address the server is bound to. Before Start it is the
// configured host:port; after Start with port 0 it is the OS-assigned one.
func (s *Server) Addr() string { return s.httpSrv.Addr }

// Start binds the listen socket synchronously and then serves in the
// background. A bind failure is returned directly. Once it returns nil the
// server is accepting connections; observe an unexpected later stop with Err,
// and stop it with Shutdown.
func (s *Server) Start() error {
	if s.errCh != nil {
		return errors.New("server: already started")
	}

	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("server: listen on %s: %w", s.httpSrv.Addr, err)
	}
	s.httpSrv.Addr = ln.Addr().String()

	scheme := "http"
	if s.cfg.TLS.Enabled {
		tlsCfg, terr := s.tlsConfig()
		if terr != nil {
			_ = ln.Close()
			return terr
		}
		s.httpSrv.TLSConfig = tlsCfg
		ln = tls.NewListener(ln, tlsCfg)
		scheme = "https"
	}

	s.errCh = make(chan error, 1)
	go func() {
		serveErr := s.httpSrv.Serve(ln)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			s.log.Error("http server stopped unexpectedly", "error", serveErr)
			s.errCh <- serveErr
			return
		}
		s.errCh <- nil
	}()

	s.log.Info("http server listening", "addr", s.httpSrv.Addr, "scheme", scheme)
	return nil
}

func (s *Server) tlsConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(s.cfg.TLS.CertFile, s.cfg.TLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("server: load TLS keypair: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

// Err receives a value if the server stops on its own rather than via
// Shutdown: non-nil on a serve error, nil on an orderly stop. Valid only
// after Start, and only one of Err or Shutdown should consume it.
func (s *Server) Err() <-chan error { return s.errCh }

// Shutdown gracefully stops the server, waiting up to
// ServerConfig.ShutdownTimeout for in-flight requests to drain. It is safe to
// call if the server already stopped, and safe to call more than once.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.errCh == nil {
		return nil
	}

	timeout := s.cfg.ShutdownTimeout.Std()
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.log.Info("http server shutting down")
	shutdownErr := s.httpSrv.Shutdown(ctx)

	// Shutdown returns only after Serve has returned, so the goroutine has
	// already sent; a non-blocking read avoids hanging if Err consumed it.
	var serveErr error
	select {
	case serveErr = <-s.errCh:
	default:
	}
	s.errCh = nil

	if shutdownErr != nil {
		return fmt.Errorf("server: graceful shutdown: %w", shutdownErr)
	}
	if serveErr != nil {
		return serveErr
	}
	s.log.Info("http server stopped")
	return nil
}

// Run starts s and blocks until ctx is cancelled or the server stops on its
// own, then shuts it down. The simple entry point for main.go.
func Run(ctx context.Context, s *Server) error {
	if err := s.Start(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-s.Err():
		if err != nil {
			return err
		}
		return s.Shutdown(context.Background())
	}
}
