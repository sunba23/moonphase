package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

func TestListForUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	owner := seedUser(ctx, t, pool)
	other := seedUser(ctx, t, pool)

	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B+")
	p2, c2 := seedProblem(ctx, t, pool, "6C")

	// Owner's first ended session: two rated problems + a trailing unrated row.
	s1, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start s1: %v", err)
	}
	if err := store.AdvanceSession(ctx, s1.ID, 0, 5, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("advance s1 seq 0: %v", err)
	}
	if err := store.AdvanceSession(ctx, s1.ID, 1, 7, session.CompletionFailed,
		session.SessionProblem{Seq: 2, ProblemID: p2, ConfigurationID: c2}); err != nil {
		t.Fatalf("advance s1 seq 1: %v", err)
	}
	if err := store.EndSession(ctx, s1.ID, owner); err != nil {
		t.Fatalf("end s1: %v", err)
	}

	// Owner's second ended session: ended right after start, no rated problems.
	s2, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start s2: %v", err)
	}
	if err := store.EndSession(ctx, s2.ID, owner); err != nil {
		t.Fatalf("end s2: %v", err)
	}

	// Owner's still-active session — must be excluded from the list.
	if _, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0}); err != nil {
		t.Fatalf("start active: %v", err)
	}

	// Another user's ended session — must never appear for owner.
	sOther, err := store.StartSession(ctx, session.Session{
		UserID: other, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start other: %v", err)
	}
	if err := store.EndSession(ctx, sOther.ID, other); err != nil {
		t.Fatalf("end other: %v", err)
	}

	got, err := store.ListForUser(ctx, owner)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (ended only, owner only)", len(got))
	}
	if got[0].ID != s2.ID || got[1].ID != s1.ID {
		t.Fatalf("order = [%s %s], want newest-first [%s %s]", got[0].ID, got[1].ID, s2.ID, s1.ID)
	}
	if got[0].ClimbedCount != 0 {
		t.Fatalf("s2 ClimbedCount = %d, want 0", got[0].ClimbedCount)
	}
	if got[1].ClimbedCount != 2 {
		t.Fatalf("s1 ClimbedCount = %d, want 2 (trailing unrated row excluded)", got[1].ClimbedCount)
	}
	if got[1].Holdsetup != 1 || got[1].Angle != 40 {
		t.Fatalf("s1 snapshot = %d/%d, want 1/40", got[1].Holdsetup, got[1].Angle)
	}

	fresh := seedUser(ctx, t, pool)
	empty, err := store.ListForUser(ctx, fresh)
	if err != nil {
		t.Fatalf("ListForUser(fresh): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("fresh user list = %v, want non-nil empty slice", empty)
	}
}

func TestClimbedProblems(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	owner := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B+")
	p2, c2 := seedProblem(ctx, t, pool, "6C")

	s1, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := store.AdvanceSession(ctx, s1.ID, 0, 4, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("advance seq 0: %v", err)
	}
	if err := store.AdvanceSession(ctx, s1.ID, 1, 8, session.CompletionFailed,
		session.SessionProblem{Seq: 2, ProblemID: p2, ConfigurationID: c2}); err != nil {
		t.Fatalf("advance seq 1: %v", err)
	}
	if err := store.EndSession(ctx, s1.ID, owner); err != nil {
		t.Fatalf("end: %v", err)
	}

	got, err := store.ClimbedProblems(ctx, s1.ID)
	if err != nil {
		t.Fatalf("ClimbedProblems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d climbed, want 2 (trailing unrated row excluded)", len(got))
	}
	if got[0].Seq != 0 || got[1].Seq != 1 {
		t.Fatalf("seq order = [%d %d], want [0 1]", got[0].Seq, got[1].Seq)
	}
	if got[0].RPE != 4 || got[0].Completion != session.CompletionSent {
		t.Fatalf("row 0 = RPE %d %q, want 4 sent", got[0].RPE, got[0].Completion)
	}
	if got[1].RPE != 8 || got[1].Completion != session.CompletionFailed {
		t.Fatalf("row 1 = RPE %d %q, want 8 failed", got[1].RPE, got[1].Completion)
	}
	if got[0].Grade != "6B" || got[1].Grade != "6B+" {
		t.Fatalf("grades = %q %q, want 6B 6B+", got[0].Grade, got[1].Grade)
	}
	if got[0].Name == "" {
		t.Fatalf("climbed problem name is empty")
	}

	s2, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start s2: %v", err)
	}
	if err := store.EndSession(ctx, s2.ID, owner); err != nil {
		t.Fatalf("end s2: %v", err)
	}
	none, err := store.ClimbedProblems(ctx, s2.ID)
	if err != nil {
		t.Fatalf("ClimbedProblems(s2): %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("s2 climbed = %v, want non-nil empty slice", none)
	}
}

func TestClimbedProblemAt(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	owner := seedUser(ctx, t, pool)
	p0, c0 := seedProblem(ctx, t, pool, "6B")
	p1, c1 := seedProblem(ctx, t, pool, "6B+")

	s1, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Rates seq 0, inserts an unrated trailing seq 1.
	if err := store.AdvanceSession(ctx, s1.ID, 0, 6, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("advance: %v", err)
	}

	ref, err := store.ClimbedProblemAt(ctx, s1.ID, 0)
	if err != nil {
		t.Fatalf("ClimbedProblemAt(0): %v", err)
	}
	if ref.ConfigurationID != c0 || ref.RPE != 6 || ref.Completion != session.CompletionSent {
		t.Fatalf("ref = %+v, want config %d / RPE 6 / sent", ref, c0)
	}

	if _, err := store.ClimbedProblemAt(ctx, s1.ID, 1); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("trailing unrated seq err = %v, want ErrNotFound", err)
	}
	if _, err := store.ClimbedProblemAt(ctx, s1.ID, 99); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("out-of-range seq err = %v, want ErrNotFound", err)
	}
	if _, err := store.ClimbedProblemAt(ctx, "00000000-0000-0000-0000-000000000000", 0); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("wrong session id err = %v, want ErrNotFound", err)
	}
}
