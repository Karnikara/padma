// Package migrations embeds padma's goose SQL migrations so they ship inside the
// binary and run via `app migrate`.
package migrations

import "embed"

// FS holds every *.sql migration, applied in filename order by goose.
//
//go:embed *.sql
var FS embed.FS
