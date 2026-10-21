package migrations

import "embed"

// Files are shared by the CLI migration runner and the isolated test database.
//
//go:embed *.sql
var Files embed.FS
