// Package storage is the blob store for job files: uploaded originals and
// watermarked previews. Only the local-disk backend exists today; New
// returns an explicit error for driver "s3" until that lands. Object
// metadata (size, MIME, checksum) is the database's job - this package only
// moves bytes.
package storage

import (
	"fmt"
	"log/slog"

	"sade/config"
)

// New builds the Storage backend named by cfg.Driver.
func New(cfg config.StorageConfig, log *slog.Logger) (Storage, error) {
	switch cfg.Driver {
	case "local":
		return newLocal(cfg.LocalPath, log)
	case "s3":
		return nil, fmt.Errorf("storage: s3 backend is not implemented yet")
	default:
		return nil, fmt.Errorf("storage: unknown driver %q (want local or s3)", cfg.Driver)
	}
}
