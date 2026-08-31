// Package testdb spins up an ephemeral Postgres with the real migration
// schema applied, for integration tests. Docker must be available; tests that
// call New will fail (not skip) without it, per the change's test plan.
package testdb

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sunba23/moonphase/migrations"
)

// authUsersShim stands in for the Supabase-managed auth.users table so the
// profiles / sessions foreign keys resolve against a bare Postgres image.
const authUsersShim = `
CREATE SCHEMA IF NOT EXISTS auth;
CREATE TABLE IF NOT EXISTS auth.users (id uuid PRIMARY KEY);
`

// New starts postgres:17-alpine, applies the auth.users shim plus every
// migration, and returns a connected pool. The container and pool are torn
// down via t.Cleanup.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("moonphase_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("testdb: start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("testdb: terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testdb: connection string: %v", err)
	}

	applySchema(ctx, t, dsn)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("testdb: create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func applySchema(ctx context.Context, t testing.TB, dsn string) {
	t.Helper()

	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("testdb: parse dsn: %v", err)
	}
	db := stdlib.OpenDB(*connConfig)
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, authUsersShim); err != nil {
		t.Fatalf("testdb: create auth.users shim: %v", err)
	}

	m, err := migrations.NewMigrator(db)
	if err != nil {
		t.Fatalf("testdb: build migrator: %v", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("testdb: apply migrations: %v", err)
	}
}
