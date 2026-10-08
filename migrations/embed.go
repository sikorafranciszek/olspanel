// Package migrations embeds goose SQL migrations.
package migrations

import "embed"

// FS contains all *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
