package migrations

import "embed"

// FS contains versioned SQL migration files.
//
//go:embed *.sql
var FS embed.FS
