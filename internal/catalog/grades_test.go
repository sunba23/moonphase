package catalog_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/testdb"
)

func seedConfigGrade(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, holdsetup, angle int, grade string) {
	t.Helper()
	var problemID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, $2, 'seed', '') RETURNING id
	`, ext, holdsetup).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade)
		VALUES ($1, $2, $3, $4, $5)
	`, problemID, holdsetup, ext, angle, grade); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
}

func TestGradeLadder(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedConfigGrade(ctx, t, pool, 1, 1, 40, "6C")
	seedConfigGrade(ctx, t, pool, 2, 1, 40, "6B")
	seedConfigGrade(ctx, t, pool, 3, 1, 40, "6B+")
	seedConfigGrade(ctx, t, pool, 4, 1, 40, "6B") // duplicate grade
	seedConfigGrade(ctx, t, pool, 5, 1, 40, "")   // blank grade excluded
	seedConfigGrade(ctx, t, pool, 6, 1, 25, "7A") // different angle excluded

	ladder, err := catalog.GradeLadder(ctx, pool, 1, 40)
	if err != nil {
		t.Fatalf("GradeLadder: %v", err)
	}
	want := []string{"6B", "6B+", "6C"}
	if len(ladder) != len(want) {
		t.Fatalf("ladder = %v, want %v", ladder, want)
	}
	for i := range want {
		if ladder[i] != want[i] {
			t.Fatalf("ladder = %v, want %v", ladder, want)
		}
	}
}
