package migrations

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxv5 "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// NewMigrator builds a golang-migrate instance over the embedded SQL files,
// bound to an already-open database/sql handle. Shared by cmd/migrate and
// internal/testdb so both apply the schema exactly the same way. The caller
// owns db's lifecycle; migrate.Close on the returned value does not close it.
func NewMigrator(db *sql.DB) (*migrate.Migrate, error) {
	dbDriver, err := pgxv5.WithInstance(db, &pgxv5.Config{})
	if err != nil {
		return nil, fmt.Errorf("create pgx migrate driver: %w", err)
	}

	src, err := iofs.New(FS, ".")
	if err != nil {
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", dbDriver)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return m, nil
}
