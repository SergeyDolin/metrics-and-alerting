package storage

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func RunMigrations(db *sql.DB) error {
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}
