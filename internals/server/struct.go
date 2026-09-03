package server

import (
	"log/slog"
	"net/http"

	"sade/config"
)

// Server is a thin lifecycle wrapper around *http.Server: bind synchronously
// (so a taken port fails fast), serve in the background, shut down within a
// bounded timeout. It owns no routes or middleware - the handler passed to
// New is the fully assembled router.
type Server struct {
	cfg     config.ServerConfig
	log     *slog.Logger
	httpSrv *http.Server
	errCh   chan error // buffered(1); receives the Serve result once it returns
}
