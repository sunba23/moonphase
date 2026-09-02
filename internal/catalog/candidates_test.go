package catalog_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/testdb"
)

// seedNextProblem inserts a problem + config on (1, 40) at grade, plus a
// problem_hold_types row with the given dominant.
func seedNextProblem(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, grade, dominant string) (problemID, configID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, 'seed', '') RETURNING id
	`, ext).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
		VALUES ($1, 1, $2, 40, $3, true, 20) RETURNING id
	`, problemID, ext, grade).Scan(&configID); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	var dom any
	if dominant != "" {
		dom = dominant
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO problem_hold_types (problem_id, total_scored, dominant) VALUES ($1, 4, $2)
	`, problemID, dom); err != nil {
		t.Fatalf("seed hold types: %v", err)
	}
	return problemID, configID
}

// seedNextProblemsBulk inserts n problems + configs on (1, 40) at grade, each
// with a problem_hold_types row carrying dominant, in one statement — for tests
// that need a realistically lopsided pool larger than the sample LIMIT.
func seedNextProblemsBulk(ctx context.Context, t *testing.T, pool *pgxpool.Pool, extBase, n int, grade, dominant string) {
	t.Helper()
	var dom any
	if dominant != "" {
		dom = dominant
	}
	if _, err := pool.Exec(ctx, `
		WITH new_problems AS (
			INSERT INTO problems (external_id, holdsetup, name, moves_raw)
			SELECT g, 1, 'bulk', '' FROM generate_series($1::int, $1::int + $2::int - 1) g
			RETURNING id, external_id
		),
		new_configs AS (
			INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
			SELECT id, 1, external_id, 40, $3, true, 20 FROM new_problems
			RETURNING problem_id
		)
		INSERT INTO problem_hold_types (problem_id, total_scored, dominant)
		SELECT problem_id, 4, $4 FROM new_configs
	`, extBase, n, grade, dom); err != nil {
		t.Fatalf("seed bulk: %v", err)
	}
}

// TestNextPickCandidatesLopsidedWindow pins the ramp fix: a window whose lower
// grade has ~30x the population of the higher grade must still surface the
// higher grade. Before the random sample, the LIMIT took the lower grade's
// rows only and the loop could never ramp.
func TestNextPickCandidatesLopsidedWindow(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedNextProblemsBulk(ctx, t, pool, 1000, 600, "6B+", "jug")
	seedNextProblemsBulk(ctx, t, pool, 2000, 20, "6C", "pocket")

	got, err := catalog.NextPickCandidates(ctx, pool, catalog.NextPickQuery{
		Holdsetup: 1, Angle: 40, GradeMin: "6B+", GradeMax: "6C",
	})
	if err != nil {
		t.Fatalf("NextPickCandidates: %v", err)
	}

	has6C := false
	for _, c := range got {
		if c.Grade == "6C" {
			has6C = true
			break
		}
	}
	if !has6C {
		t.Fatalf("sample of %d rows from a [6B+ x600, 6C x20] window has no 6C row — the ramp fix is not working", len(got))
	}
}

func TestNextPickCandidates(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	p1, _ := seedNextProblem(ctx, t, pool, 1, "6B", "crimp")
	seedNextProblem(ctx, t, pool, 2, "6B", "sloper")
	seedNextProblem(ctx, t, pool, 3, "6B+", "jug")
	seedNextProblem(ctx, t, pool, 4, "6C", "jug") // out of window

	all, err := catalog.NextPickCandidates(ctx, pool, catalog.NextPickQuery{
		Holdsetup: 1, Angle: 40, GradeMin: "6B", GradeMax: "6B+",
	})
	if err != nil {
		t.Fatalf("NextPickCandidates: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("window [6B,6B+] returned %d, want 3", len(all))
	}

	excl, err := catalog.NextPickCandidates(ctx, pool, catalog.NextPickQuery{
		Holdsetup: 1, Angle: 40, GradeMin: "6B", GradeMax: "6B+",
		ExcludeProblemIDs: []int64{p1},
	})
	if err != nil {
		t.Fatalf("NextPickCandidates exclude: %v", err)
	}
	if len(excl) != 2 {
		t.Fatalf("exclude one returned %d, want 2", len(excl))
	}

	noCrimp, err := catalog.NextPickCandidates(ctx, pool, catalog.NextPickQuery{
		Holdsetup: 1, Angle: 40, GradeMin: "6B", GradeMax: "6B+",
		ExcludeDominant: "crimp",
	})
	if err != nil {
		t.Fatalf("NextPickCandidates exclude-dominant: %v", err)
	}
	for _, c := range noCrimp {
		if c.Dominant == "crimp" {
			t.Fatalf("exclude-dominant crimp still returned a crimp row")
		}
	}
	if len(noCrimp) != 2 {
		t.Fatalf("exclude-dominant returned %d, want 2", len(noCrimp))
	}
}
