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

// seedPickBulk inserts n problems + configs on (1, 40) at grade, each with a
// problem_hold_types row carrying dominant, in one statement. For tests that
// need a pool larger than the NextPickCandidates sample LIMIT.
func seedPickBulk(ctx context.Context, t *testing.T, pool *pgxpool.Pool, extBase, n int, grade, dominant string) {
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

func ladderIndexTest(ladder []string, g string) int { return indexOf(ladder, g) }

// TestPickNextRampEscapesDenseFloor pins the ramp fix at the PickNext level: an
// easy send must step grade up even when the current (lower) grade has far more
// problems than the target grade. The 6C rows carry a dominant absent from the
// shown history so every 6C candidate outscores every 6B+ one.
func TestPickNextRampEscapesDenseFloor(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedPickBulk(ctx, t, pool, 1, 800, "6B+", "jug")
	seedPickBulk(ctx, t, pool, 1000, 30, "6C", "pocket")

	rec := New(pool)
	in := PickNextInput{
		Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
		Shown: []ShownState{
			{ProblemID: 900001, Grade: "6B", Dominant: "crimp"},
			{ProblemID: 900002, Grade: "6B+", Dominant: "sloper"},
			{ProblemID: 900003, Grade: "6B+", Dominant: "jug"},
		},
		CurrentResult: Result{RPE: 3, Completion: CompletionSent},
	}

	for i := 0; i < 5; i++ {
		pick, _, err := rec.PickNext(ctx, in)
		if err != nil {
			t.Fatalf("PickNext: %v", err)
		}
		if pick.Grade != "6C" {
			t.Fatalf("iter %d: easy send from a dense 6B+ floor picked %q, want 6C — ramp is stuck", i, pick.Grade)
		}
	}
}

// TestPickNextIgnoresSkipped pins that a skipped shown problem is inert: it does
// not anchor the grade window, it does not suppress its own hold type, and it is
// still excluded from the result.
func TestPickNextIgnoresSkipped(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	pAnchor, _ := seedPick(ctx, t, pool, 1, "6B", "crimp")
	pSkip, _ := seedPick(ctx, t, pool, 2, "6B+", "sloper")
	seedPick(ctx, t, pool, 3, "7A", "sloper")
	seedPickBulk(ctx, t, pool, 100, 20, "6B+", "sloper")

	rec := New(pool)
	in := PickNextInput{
		Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
		Shown: []ShownState{
			{ProblemID: pAnchor, Grade: "6B", Dominant: "crimp"},
			{ProblemID: pSkip, Grade: "6B+", Dominant: "sloper", Skipped: true},
		},
		CurrentResult: Result{RPE: 3, Completion: CompletionSent},
	}

	for i := 0; i < 8; i++ {
		pick, _, err := rec.PickNext(ctx, in)
		if err != nil {
			t.Fatalf("iter %d: PickNext: %v", i, err)
		}
		// Anchored at the rated 6B (stepUp -> 6B+), never at the skipped 6B+/7A.
		if pick.Grade != "6B+" {
			t.Fatalf("iter %d: grade %q, want 6B+ (skipped entry must not move the anchor)", i, pick.Grade)
		}
		// The skipped problem is not handed back.
		if pick.ProblemID == pSkip {
			t.Fatalf("iter %d: returned the skipped problem", i)
		}
	}
}

func TestFirstPickExcluding(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	ext := 0
	next := func() int { ext++; return ext }
	a, _ := seedPick(ctx, t, pool, next(), "6B", "crimp")
	b, _ := seedPick(ctx, t, pool, next(), "6B", "sloper")
	c, _ := seedPick(ctx, t, pool, next(), "6B", "jug")

	rec := New(pool)
	for i := 0; i < 20; i++ {
		pick, poolSize, err := rec.FirstPickExcluding(ctx, 1, 40, []int64{a, b})
		if err != nil {
			t.Fatalf("iter %d: FirstPickExcluding: %v", i, err)
		}
		if poolSize != 1 {
			t.Fatalf("iter %d: poolSize = %d, want 1 (only one non-excluded problem)", i, poolSize)
		}
		if pick.ProblemID != c {
			t.Fatalf("iter %d: picked %d, want the only non-excluded problem %d", i, pick.ProblemID, c)
		}
	}
}

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

// TestPickNextDiagFields pins the decision-log diagnostics widened for the
// rec_pick log: Band, PreferredGrade, TieSetSize, and pick.Dominant.
func TestPickNextDiagFields(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	ext := 0
	next := func() int { ext++; return ext }
	for _, g := range []string{"6B", "6B+", "6C"} {
		for _, d := range []string{"crimp", "sloper", "jug"} {
			seedPick(ctx, t, pool, next(), g, d)
		}
	}
	rec := New(pool)

	cases := []struct {
		name      string
		rpe       int
		comp      Completion
		fromGrade string
		wantBand  string
		wantPref  string
	}{
		{"easy send steps up", 3, CompletionSent, "6B", "step_up", "6B+"},
		{"mid send holds", 6, CompletionSent, "6B+", "hold", "6B+"},
		{"hard send backs off", 9, CompletionSent, "6C", "back_off", "6B+"},
		{"failed backs off", 5, CompletionFailed, "6C", "back_off", "6B+"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diag, err := rec.PickNext(ctx, PickNextInput{
				Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
				Shown:         []ShownState{{ProblemID: 999000, Grade: c.fromGrade, Dominant: "jug"}},
				CurrentResult: Result{RPE: c.rpe, Completion: c.comp},
			})
			if err != nil {
				t.Fatalf("PickNext: %v", err)
			}
			if diag.Band != c.wantBand {
				t.Fatalf("Band = %q, want %q", diag.Band, c.wantBand)
			}
			if diag.PreferredGrade != c.wantPref {
				t.Fatalf("PreferredGrade = %q, want %q", diag.PreferredGrade, c.wantPref)
			}
			if diag.FallbackTier == 0 && diag.TieSetSize < 1 {
				t.Fatalf("TieSetSize = %d at tier 0, want >= 1", diag.TieSetSize)
			}
		})
	}

	t.Run("tier-0 pick carries a dominant", func(t *testing.T) {
		pick, diag, err := rec.PickNext(ctx, PickNextInput{
			Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
			Shown:         []ShownState{{ProblemID: 999000, Grade: "6B", Dominant: "jug"}},
			CurrentResult: Result{RPE: 3, Completion: CompletionSent},
		})
		if err != nil {
			t.Fatalf("PickNext: %v", err)
		}
		if diag.FallbackTier == 0 && pick.Dominant == "" {
			t.Fatalf("tier-0 pick has empty Dominant")
		}
	})
}
