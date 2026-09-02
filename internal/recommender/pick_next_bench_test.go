package recommender

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/testdb"
)

// benchSeedGrade bulk-inserts n problems + configs on (1, 40) at grade, each
// with a problem_hold_types row cycling through the dominant list, in one
// statement — fast enough to stand up a catalog-scale pool per benchmark.
func benchSeedGrade(ctx context.Context, b *testing.B, pool *pgxpool.Pool, extBase, n int, grade string) {
	b.Helper()
	if _, err := pool.Exec(ctx, `
		WITH new_problems AS (
			INSERT INTO problems (external_id, holdsetup, name, moves_raw)
			SELECT g, 1, 'bench', '' FROM generate_series($1::int, $1::int + $2::int - 1) g
			RETURNING id, external_id
		),
		new_configs AS (
			INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
			SELECT id, 1, external_id, 40, $3, true, 20 FROM new_problems
			RETURNING problem_id, api_id
		)
		INSERT INTO problem_hold_types (problem_id, total_scored, dominant)
		SELECT problem_id, 4,
		       (ARRAY['crimp','sloper','pinch','jug','pocket',NULL])[1 + (api_id % 6)]
		FROM new_configs
	`, extBase, n, grade); err != nil {
		b.Fatalf("bench seed %s: %v", grade, err)
	}
}

// BenchmarkPickNext records a baseline for the ~3 s p95 next-pick guardrail
// (test-plan Risk #7), against a catalog-scale pool: ~5000 configs per grade
// across three grades, so the ORDER BY random() sample + top-N heapsort runs
// against a realistic window.
func BenchmarkPickNext(b *testing.B) {
	ctx := context.Background()
	pool := testdb.New(b)

	const perGrade = 5000
	benchSeedGrade(ctx, b, pool, 1, perGrade, "6B")
	benchSeedGrade(ctx, b, pool, 1+perGrade, perGrade, "6B+")
	benchSeedGrade(ctx, b, pool, 1+2*perGrade, perGrade, "6C")

	in := PickNextInput{
		Holdsetup: 1, Angle: 40, SessionMaxGrade: "7A",
		Shown: []ShownState{
			{ProblemID: 900001, Grade: "6B", Dominant: "crimp"},
			{ProblemID: 900002, Grade: "6B", Dominant: "sloper"},
			{ProblemID: 900003, Grade: "6B+", Dominant: "jug"},
		},
		CurrentResult: Result{RPE: 5, Completion: CompletionSent},
	}
	rec := New(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := rec.PickNext(ctx, in); err != nil {
			b.Fatalf("PickNext: %v", err)
		}
	}
	b.StopTimer()

	nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	if nsPerOp > 3e9 {
		b.Fatalf("PickNext %.0f ns/op exceeds the 3 s guardrail", nsPerOp)
	}
}

// BenchmarkFirstPick records a baseline for the ~10 s first-pick guardrail
// (test-plan Risk #7). FirstPick / MinGradeCandidates are not changing, but the
// guardrail has never had one.
func BenchmarkFirstPick(b *testing.B) {
	ctx := context.Background()
	pool := testdb.New(b)

	benchSeedGrade(ctx, b, pool, 1, 1000, "6A")     // the minimum grade
	benchSeedGrade(ctx, b, pool, 1001, 4000, "6B")  // higher grades — ignored by FirstPick
	benchSeedGrade(ctx, b, pool, 5001, 4000, "6B+") //

	rec := New(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rec.FirstPick(ctx, 1, 40); err != nil {
			b.Fatalf("FirstPick: %v", err)
		}
	}
	b.StopTimer()

	nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	if nsPerOp > 10e9 {
		b.Fatalf("FirstPick %.0f ns/op exceeds the 10 s guardrail", nsPerOp)
	}
}
