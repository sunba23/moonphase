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
