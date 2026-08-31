package catalog_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/testdb"
)

func seedProblemWithMoves(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, grade string, moves [][2]string) int64 {
	t.Helper()
	var problemID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, 'Test Problem', '') RETURNING id
	`, ext).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	var configID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
		VALUES ($1, 1, $2, 40, $3, true, 20) RETURNING id
	`, problemID, ext, grade).Scan(&configID); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
	for i, m := range moves {
		gridRef, moveType := m[0], m[1]
		if _, err := pool.Exec(ctx, `
			INSERT INTO holds (holdsetup, grid_ref, primary_type, modifiers, is_tagged)
			VALUES (1, $1, 'crimp', ARRAY['sharp']::text[], true)
			ON CONFLICT DO NOTHING
		`, gridRef); err != nil {
			t.Fatalf("seed hold: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO problem_moves (problem_id, holdsetup, seq, move_type, grid_ref)
			VALUES ($1, 1, $2, $3, $4)
		`, problemID, i, moveType, gridRef); err != nil {
			t.Fatalf("seed move: %v", err)
		}
	}
	_ = configID
	return configID
}

func TestMinGradeCandidates(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedProblemWithMoves(ctx, t, pool, 1, "6C", nil)
	seedProblemWithMoves(ctx, t, pool, 2, "6B", nil)
	seedProblemWithMoves(ctx, t, pool, 3, "6B", nil)

	cands, err := catalog.MinGradeCandidates(ctx, pool, 1, 40)
	if err != nil {
		t.Fatalf("MinGradeCandidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2 (both at 6B)", len(cands))
	}
	for _, c := range cands {
		if c.Grade != "6B" {
			t.Errorf("candidate grade = %q, want 6B", c.Grade)
		}
	}
}

func TestProblemDetail(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	configID := seedProblemWithMoves(ctx, t, pool, 7, "6B", [][2]string{
		{"C5", "s"}, {"F10", "l"}, {"H18", "e"},
	})

	view, err := catalog.ProblemDetail(ctx, pool, configID)
	if err != nil {
		t.Fatalf("ProblemDetail: %v", err)
	}
	if view.Name != "Test Problem" || view.Grade != "6B" || view.BoardYear != "2016" || view.Angle != 40 {
		t.Fatalf("view header = %+v", view)
	}
	if len(view.Holds) != 3 {
		t.Fatalf("got %d holds, want 3", len(view.Holds))
	}
	if view.Holds[0].Role != "start" || view.Holds[2].Role != "finish" || view.Holds[1].Role != "hand" {
		t.Errorf("roles = %q/%q/%q", view.Holds[0].Role, view.Holds[1].Role, view.Holds[2].Role)
	}
	if view.Holds[0].GridRef != "C5" || view.Holds[0].Col != 2 || view.Holds[0].Row != 5 {
		t.Errorf("hold[0] pos = %+v", view.Holds[0])
	}
	if view.Holds[0].PrimaryType != "crimp" {
		t.Errorf("hold[0] primary type = %q, want crimp", view.Holds[0].PrimaryType)
	}
}
