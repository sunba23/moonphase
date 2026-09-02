package catalog_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/testdb"
)

// seedTaggedHold inserts (or leaves) a hold with a given primary_type.
func seedTaggedHold(ctx context.Context, t *testing.T, pool *pgxpool.Pool, gridRef, primaryType string) {
	t.Helper()
	var pt any
	if primaryType == "" {
		pt = nil
	} else {
		pt = primaryType
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO holds (holdsetup, grid_ref, primary_type, modifiers, is_tagged)
		VALUES (1, $1, $2, ARRAY[]::text[], $3)
		ON CONFLICT (holdsetup, grid_ref) DO UPDATE SET primary_type = EXCLUDED.primary_type, is_tagged = EXCLUDED.is_tagged
	`, gridRef, pt, primaryType != ""); err != nil {
		t.Fatalf("seed hold %s: %v", gridRef, err)
	}
}

// seedProblemMoves inserts a problem plus one move per (gridRef, moveType).
func seedProblemMoves(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, moves [][2]string) int64 {
	t.Helper()
	var problemID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, 'seed', '') RETURNING id
	`, ext).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	for i, m := range moves {
		if _, err := pool.Exec(ctx, `
			INSERT INTO problem_moves (problem_id, holdsetup, seq, move_type, grid_ref)
			VALUES ($1, 1, $2, $3, $4)
		`, problemID, i, m[1], m[0]); err != nil {
			t.Fatalf("seed move: %v", err)
		}
	}
	return problemID
}

func readComposition(ctx context.Context, t *testing.T, pool *pgxpool.Pool, problemID int64) (crimp, sloper, jug, unknown, total int, dominant string) {
	t.Helper()
	var dom *string
	if err := pool.QueryRow(ctx, `
		SELECT crimp, sloper, jug, unknown, total_scored, dominant
		FROM problem_hold_types WHERE problem_id = $1
	`, problemID).Scan(&crimp, &sloper, &jug, &unknown, &total, &dom); err != nil {
		t.Fatalf("read composition: %v", err)
	}
	if dom != nil {
		dominant = *dom
	}
	return
}

func TestRecomputeHoldTypes(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedTaggedHold(ctx, t, pool, "A1", "crimp")
	seedTaggedHold(ctx, t, pool, "A2", "crimp")
	seedTaggedHold(ctx, t, pool, "A3", "crimp")
	seedTaggedHold(ctx, t, pool, "B1", "jug")
	seedTaggedHold(ctx, t, pool, "F1", "crimp") // foot hold, excluded by move_type

	problemID := seedProblemMoves(ctx, t, pool, 1, [][2]string{
		{"A1", "l"}, {"A2", "r"}, {"A3", "m"}, {"B1", "p"}, {"F1", "f"},
	})

	if err := catalog.RecomputeHoldTypes(ctx, pool, problemID); err != nil {
		t.Fatalf("RecomputeHoldTypes: %v", err)
	}

	crimp, _, jug, unknown, total, dominant := readComposition(ctx, t, pool, problemID)
	if crimp != 3 || jug != 1 || unknown != 0 || total != 4 || dominant != "crimp" {
		t.Fatalf("composition = crimp %d jug %d unknown %d total %d dominant %q, want 3/1/0/4/crimp", crimp, jug, unknown, total, dominant)
	}

	// Retag one crimp -> sloper, recompute just this problem.
	seedTaggedHold(ctx, t, pool, "A1", "sloper")
	if err := catalog.RecomputeHoldTypes(ctx, pool, problemID); err != nil {
		t.Fatalf("RecomputeHoldTypes after retag: %v", err)
	}
	crimp, sloper, _, _, _, dominant := readComposition(ctx, t, pool, problemID)
	if crimp != 2 || sloper != 1 || dominant != "crimp" {
		t.Fatalf("after retag = crimp %d sloper %d dominant %q, want 2/1/crimp", crimp, sloper, dominant)
	}
}

func TestApplyTagsRecomputesComposition(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	// Untagged holds first, then a problem using them.
	seedTaggedHold(ctx, t, pool, "C1", "")
	seedTaggedHold(ctx, t, pool, "C2", "")
	problemID := seedProblemMoves(ctx, t, pool, 1, [][2]string{{"C1", "l"}, {"C2", "r"}})

	if err := catalog.RecomputeHoldTypes(ctx, pool, problemID); err != nil {
		t.Fatalf("initial recompute: %v", err)
	}
	if _, _, _, unknown, _, _ := readComposition(ctx, t, pool, problemID); unknown != 2 {
		t.Fatalf("pre-tag unknown = %d, want 2", unknown)
	}

	store := catalog.NewHoldStore(pool)
	if err := store.ApplyTags(ctx, 1, []catalog.HoldRow{
		{GridRef: "C1", PrimaryType: "pinch"},
		{GridRef: "C2", PrimaryType: "pinch"},
	}); err != nil {
		t.Fatalf("ApplyTags: %v", err)
	}

	var pinch, unknown int
	if err := pool.QueryRow(ctx, `SELECT pinch, unknown FROM problem_hold_types WHERE problem_id = $1`, problemID).Scan(&pinch, &unknown); err != nil {
		t.Fatalf("read after ApplyTags: %v", err)
	}
	if pinch != 2 || unknown != 0 {
		t.Fatalf("after ApplyTags = pinch %d unknown %d, want 2/0", pinch, unknown)
	}
}

func TestDominantHoldType(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{"plurality", map[string]int{"crimp": 3, "jug": 1}, "crimp"},
		{"alphabetical tie-break", map[string]int{"jug": 2, "crimp": 2}, "crimp"},
		{"all zero", map[string]int{"crimp": 0}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := catalog.DominantHoldType(tt.counts); got != tt.want {
				t.Fatalf("DominantHoldType(%v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}
