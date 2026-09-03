// Package app ties the feature packages together. For now it only owns the
// model registry; the HTTP router, middleware and the repo/service/handler
// wiring move here as those land.
package app

import (
	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/magic_token"
	"sade/internals/app/payment"
	"sade/internals/app/session"
	"sade/internals/app/user"
)

// Models is every persisted model, in dependency order (parents first). It is
// the single list passed to database.Migrate, so adding a domain means adding
// one line here.
func Models() []any {
	return []any{
		user.User{},
		magic_token.MagicToken{},
		session.Session{},
		job.Job{},
		asset.Asset{},
		payment.Payment{},
	}
}
