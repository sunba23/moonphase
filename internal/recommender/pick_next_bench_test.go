package recommender

import (
	"context"
	"fmt"
	"testing"

	"github.com/sunba23/moonphase/internal/testdb"
)

// BenchmarkPickNext records a baseline for the ~3 s p95 next-pick guardrail
// (test-plan Risk #7). It seeds a realistic-size candidate pool on (1, 40) —
// a few hundred configs across three grades, each with a problem_hold_types
// row — then times PickNext with a mid-session input.
func BenchmarkPickNext(b *testing.B) {
	ctx := context.Background()
	pool := testdb.New(b)

	grades := []string{"6B", "6B+", "6C"}
	doms := []string{"crimp", "sloper", "pinch", "jug", "pocket", ""}
	const perGrade = 200
	ext := 0
	for _, g := range grades {
		for i := 0; i < perGrade; i++ {
			ext++
			var pid int64
			if err := pool.QueryRow(ctx, `
				INSERT INTO problems (external_id, holdsetup, name, moves_raw)
				VALUES ($1, 1, $2, '') RETURNING id
			`, ext, fmt.Sprintf("bench %d", ext)).Scan(&pid); err != nil {
				b.Fatalf("seed problem: %v", err)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
				VALUES ($1, 1, $2, 40, $3, true, 20)
			`, pid, ext, g); err != nil {
				b.Fatalf("seed config: %v", err)
			}
			var dom any
			if d := doms[i%len(doms)]; d != "" {
				dom = d
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO problem_hold_types (problem_id, total_scored, dominant) VALUES ($1, 4, $2)
			`, pid, dom); err != nil {
				b.Fatalf("seed hold types: %v", err)
			}
		}
	}

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
