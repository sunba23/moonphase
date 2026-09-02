package session_test

import (
	"context"
	"testing"

	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

func TestShownProblems(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	userID := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B+")
	p2, c2 := seedProblem(ctx, t, pool, "6C")

	// dominant hold type only for the middle problem
	if _, err := pool.Exec(ctx, `INSERT INTO problem_hold_types (problem_id, total_scored, dominant) VALUES ($1, 4, 'crimp')`, p1); err != nil {
		t.Fatalf("seed hold types: %v", err)
	}

	started, err := store.StartSession(ctx, session.Session{
		UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	for _, r := range []struct {
		seq      int
		pid, cid int64
	}{{1, p1, c1}, {2, p2, c2}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO session_problems (session_id, seq, problem_id, problem_configuration_id)
			VALUES ($1, $2, $3, $4)
		`, started.ID, r.seq, r.pid, r.cid); err != nil {
			t.Fatalf("insert seq %d: %v", r.seq, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE session_problems SET rpe = 6, completion = 'sent', climbed_at = now()
		WHERE session_id = $1 AND seq = 1
	`, started.ID); err != nil {
		t.Fatalf("rate seq 1: %v", err)
	}

	shown, err := store.ShownProblems(ctx, started.ID)
	if err != nil {
		t.Fatalf("ShownProblems: %v", err)
	}
	if len(shown) != 3 {
		t.Fatalf("got %d shown, want 3", len(shown))
	}
	if shown[0].RPE != nil || shown[2].RPE != nil {
		t.Fatalf("unrated rows have non-nil RPE")
	}
	if shown[1].RPE == nil || *shown[1].RPE != 6 {
		t.Fatalf("rated row RPE = %v, want 6", shown[1].RPE)
	}
	if shown[1].Dominant != "crimp" {
		t.Fatalf("shown[1].Dominant = %q, want crimp", shown[1].Dominant)
	}
	if shown[0].Grade != "6B" || shown[2].Grade != "6C" {
		t.Fatalf("grades = %q..%q", shown[0].Grade, shown[2].Grade)
	}
	_ = p2
}
