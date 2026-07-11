package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/karnikara/padma/migrations"
)

// Migrate applies all pending goose migrations against the DSN. It opens its own
// database/sql handle (goose requires one) using the pgx stdlib driver, separate
// from the pgxpool used at runtime.
func Migrate(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("db: migrate open: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("db: migrate dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}
