package recommender

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/testdb"
)

func TestPickFrom(t *testing.T) {
	bench := Candidate{ProblemID: 1, ConfigurationID: 11, Grade: "6B", IsBenchmark: true}
	repeated := Candidate{ProblemID: 2, ConfigurationID: 22, Grade: "6B", Repeats: minQualityRepeats}
	obscure := Candidate{ProblemID: 3, ConfigurationID: 33, Grade: "6B", Repeats: 1}

	tests := []struct {
		name      string
		cands     []Candidate
		roll      func(n int) int
		wantCfg   int64
		wantErrIs error
	}{
		{
			name:    "all qualify — picks within quality pool",
			cands:   []Candidate{bench, repeated},
			roll:    func(int) int { return 1 },
			wantCfg: 22,
		},
		{
			name:    "none qualify — falls back to full set",
			cands:   []Candidate{obscure},
			roll:    func(int) int { return 0 },
			wantCfg: 33,
		},
		{
			name:      "empty — ErrNoCandidates",
			cands:     nil,
			roll:      func(int) int { return 0 },
			wantErrIs: ErrNoCandidates,
		},
		{
			name:    "deterministic pick with stubbed roll",
			cands:   []Candidate{bench, repeated, obscure},
			roll:    func(int) int { return 0 },
			wantCfg: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickFrom(tt.cands, tt.roll)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tt.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.ConfigurationID != tt.wantCfg {
				t.Fatalf("ConfigurationID = %d, want %d", got.ConfigurationID, tt.wantCfg)
			}
		})
	}
}

func seedGradedProblem(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, grade string) {
	t.Helper()
	var problemID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, 'seed', '') RETURNING id
	`, ext).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark)
		VALUES ($1, 1, $2, 40, $3, true)
	`, problemID, ext, grade); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
}

func TestFirstPick_PicksLowestSeededGrade(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	seedGradedProblem(ctx, t, pool, 1, "6C")
	seedGradedProblem(ctx, t, pool, 2, "6B")
	seedGradedProblem(ctx, t, pool, 3, "7A")

	pick, err := New(pool).FirstPick(ctx, 1, 40)
	if err != nil {
		t.Fatalf("FirstPick: %v", err)
	}
	if pick.Grade != "6B" {
		t.Fatalf("FirstPick grade = %q, want 6B", pick.Grade)
	}
}
