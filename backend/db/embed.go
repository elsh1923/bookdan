// Package db embeds the SQL so the migrate command ships as a single binary.
package db

import "embed"

// Migrations are applied in order by goose. Each file is one transaction.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// DevSeed is demo data. cmd/migrate refuses to apply it when APP_ENV=production.
//
//go:embed seed/dev_seed.sql
var DevSeed string
