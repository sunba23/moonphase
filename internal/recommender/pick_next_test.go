package recommender

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/testdb"
)

func seedPick(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, grade, dominant string) (problemID, configID int64) {
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

func ladderIndexTest(ladder []string, g string) int { return indexOf(ladder, g) }

func TestPickNext(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	// A pool across 6B / 6B+ / 6C with mixed dominants.
	ext := 0
	next := func() int { ext++; return ext }
	seedPick(ctx, t, pool, next(), "6B", "crimp")
	seedPick(ctx, t, pool, next(), "6B", "sloper")
	seedPick(ctx, t, pool, next(), "6B", "jug")
	seedPick(ctx, t, pool, next(), "6B+", "crimp")
	seedPick(ctx, t, pool, next(), "6B+", "sloper")
	seedPick(ctx, t, pool, next(), "6B+", "jug")
	p6C, _ := seedPick(ctx, t, pool, next(), "6C", "crimp")
	seedPick(ctx, t, pool, next(), "6C", "sloper")

	ladder := []string{"6B", "6B+", "6C"}
	rec := New(pool)

	t.Run("hard result never harder", func(t *testing.T) {
		pick, _, err := rec.PickNext(ctx, PickNextInput{
			Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
			Shown:         []ShownState{{ProblemID: p6C, Grade: "6C", Dominant: "sloper"}},
			CurrentResult: Result{RPE: 9, Completion: CompletionSent},
		})
		if err != nil {
			t.Fatalf("PickNext: %v", err)
		}
		if ladderIndexTest(ladder, pick.Grade) > ladderIndexTest(ladder, "6C") {
			t.Fatalf("hard result picked %q, harder than 6C", pick.Grade)
		}
	})

	t.Run("easy send may step up, ceiling holds", func(t *testing.T) {
		up, _, err := rec.PickNext(ctx, PickNextInput{
			Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
			Shown:         []ShownState{{ProblemID: 999999, Grade: "6B", Dominant: "jug"}},
			CurrentResult: Result{RPE: 3, Completion: CompletionSent},
		})
		if err != nil {
			t.Fatalf("PickNext up: %v", err)
		}
		if ladderIndexTest(ladder, up.Grade) > ladderIndexTest(ladder, "6B+") {
			t.Fatalf("step-up picked %q, above the +1 window", up.Grade)
		}

		capped, _, err := rec.PickNext(ctx, PickNextInput{
			Holdsetup: 1, Angle: 40, SessionMaxGrade: "6B",
			Shown:         []ShownState{{ProblemID: 999999, Grade: "6B", Dominant: "jug"}},
			CurrentResult: Result{RPE: 3, Completion: CompletionSent},
		})
		if err != nil {
			t.Fatalf("PickNext capped: %v", err)
		}
		if capped.Grade != "6B" {
			t.Fatalf("ceiling 6B picked %q", capped.Grade)
		}
	})

	t.Run("crimp streak switches type or logs a fallback", func(t *testing.T) {
		pick, diag, err := rec.PickNext(ctx, PickNextInput{
			Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
			Shown: []ShownState{
				{ProblemID: 900001, Grade: "6B", Dominant: "crimp"},
				{ProblemID: 900002, Grade: "6B", Dominant: "crimp"},
				{ProblemID: 900003, Grade: "6B", Dominant: "crimp"},
			},
			CurrentResult: Result{RPE: 5, Completion: CompletionSent},
		})
		if err != nil {
			t.Fatalf("PickNext streak: %v", err)
		}
		if pick.Grade == "" {
			t.Fatalf("streak returned empty pick")
		}
		// Either the dominant changed, or a fallback tier fired.
		var dom string
		_ = pool.QueryRow(ctx, `SELECT COALESCE(dominant,'') FROM problem_hold_types WHERE problem_id = $1`, pick.ProblemID).Scan(&dom)
		if dom == "crimp" && diag.FallbackTier == 0 {
			t.Fatalf("crimp streak: still crimp at tier 0")
		}
	})
}
