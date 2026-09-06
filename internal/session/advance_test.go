package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

func TestAdvanceSession(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	userID := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B+")

	started, err := store.StartSession(ctx, session.Session{
		UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := store.AdvanceSession(ctx, started.ID, 0, 6, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("AdvanceSession: %v", err)
	}

	shown, err := store.ShownProblems(ctx, started.ID)
	if err != nil {
		t.Fatalf("ShownProblems: %v", err)
	}
	if len(shown) != 2 || shown[0].RPE == nil || *shown[0].RPE != 6 || shown[1].RPE != nil {
		t.Fatalf("after advance shown = %+v", shown)
	}

	// Re-submitting the same seq is stale and inserts nothing more.
	err = store.AdvanceSession(ctx, started.ID, 0, 4, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1})
	if !errors.Is(err, session.ErrStaleResult) {
		t.Fatalf("stale advance err = %v, want ErrStaleResult", err)
	}
	shown, _ = store.ShownProblems(ctx, started.ID)
	if len(shown) != 2 {
		t.Fatalf("stale advance added a row: %d", len(shown))
	}
}

func TestSkipProblem(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	userID := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B")
	p2, c2 := seedProblem(ctx, t, pool, "6B+")

	started, err := store.StartSession(ctx, session.Session{
		UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := store.SkipProblem(ctx, started.ID, 0,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("SkipProblem: %v", err)
	}

	// Seq 0 is recorded as skipped: completion set, no RPE, climbed_at stamped.
	var completion *string
	var rpe *int16
	var climbedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT completion, rpe, climbed_at FROM session_problems WHERE session_id = $1 AND seq = 0
	`, started.ID).Scan(&completion, &rpe, &climbedAt); err != nil {
		t.Fatalf("read seq 0: %v", err)
	}
	if completion == nil || *completion != session.CompletionSkipped {
		t.Fatalf("seq 0 completion = %v, want skipped", completion)
	}
	if rpe != nil {
		t.Fatalf("seq 0 rpe = %v, want nil", rpe)
	}
	if climbedAt == nil {
		t.Fatalf("seq 0 climbed_at is nil, want set")
	}

	shown, err := store.ShownProblems(ctx, started.ID)
	if err != nil {
		t.Fatalf("ShownProblems: %v", err)
	}
	if len(shown) != 2 || shown[1].Seq != 1 {
		t.Fatalf("after skip shown = %+v", shown)
	}

	// Re-submitting the skipped seq is stale and inserts nothing more.
	err = store.SkipProblem(ctx, started.ID, 0,
		session.SessionProblem{Seq: 1, ProblemID: p2, ConfigurationID: c2})
	if !errors.Is(err, session.ErrStaleResult) {
		t.Fatalf("stale skip err = %v, want ErrStaleResult", err)
	}
	shown, _ = store.ShownProblems(ctx, started.ID)
	if len(shown) != 2 {
		t.Fatalf("stale skip added a row: %d", len(shown))
	}

	// A skipped-only session has no climbed problems.
	if err := store.EndSession(ctx, started.ID, userID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	climbed, err := store.ClimbedProblems(ctx, started.ID)
	if err != nil {
		t.Fatalf("ClimbedProblems: %v", err)
	}
	if len(climbed) != 0 {
		t.Fatalf("skipped problems leaked into history: %+v", climbed)
	}
}

func TestEndSession(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	owner := seedUser(ctx, t, pool)
	other := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")

	started, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := store.EndSession(ctx, started.ID, other); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("non-owner EndSession err = %v, want ErrNotFound", err)
	}
	if err := store.EndSession(ctx, started.ID, owner); err != nil {
		t.Fatalf("owner EndSession: %v", err)
	}
	if err := store.EndSession(ctx, started.ID, owner); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("second EndSession err = %v, want ErrNotFound", err)
	}

	// The freed active slot lets the same user start again.
	if _, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0}); err != nil {
		t.Fatalf("restart after end: %v", err)
	}
}
