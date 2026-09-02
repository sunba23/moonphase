package recommender

import (
	"errors"
	"testing"
)

func TestScoreNext(t *testing.T) {
	rollZero := func(int) int { return 0 }

	t.Run("empty -> ErrNoCandidates", func(t *testing.T) {
		if _, err := scoreNext(nil, ScoreState{}, rollZero); !errors.Is(err, ErrNoCandidates) {
			t.Fatalf("err = %v, want ErrNoCandidates", err)
		}
	})

	t.Run("grade fit alone picks the preferred index", func(t *testing.T) {
		cands := []ScoreCandidate{
			{ConfigurationID: 1, GradeIndex: 2, Dominant: "jug"},
			{ConfigurationID: 2, GradeIndex: 3, Dominant: "jug"},
		}
		st := ScoreState{PreferredIndex: 3, PrevDominant: "jug"}
		got, err := scoreNext(cands, st, rollZero)
		if err != nil || cands[got].ConfigurationID != 2 {
			t.Fatalf("got idx %d (cfg %d), want cfg 2", got, cands[got].ConfigurationID)
		}
	})

	t.Run("balance overrides a one-step grade miss", func(t *testing.T) {
		// on-grade candidate is crimp, matching a 3-long crimp streak;
		// one step off is a fresh sloper.
		cands := []ScoreCandidate{
			{ConfigurationID: 10, GradeIndex: 3, Dominant: "crimp"},
			{ConfigurationID: 11, GradeIndex: 2, Dominant: "sloper"},
		}
		st := ScoreState{
			PreferredIndex:        3,
			RecentDominants:       []string{"crimp", "crimp", "crimp"},
			SessionDominantCounts: map[string]int{"crimp": 3},
			PrevDominant:          "crimp",
		}
		got, err := scoreNext(cands, st, rollZero)
		if err != nil || cands[got].ConfigurationID != 11 {
			t.Fatalf("got cfg %d, want the fresh sloper (11)", cands[got].ConfigurationID)
		}
	})

	t.Run("DropBalance flips back to the on-grade pick", func(t *testing.T) {
		cands := []ScoreCandidate{
			{ConfigurationID: 10, GradeIndex: 3, Dominant: "crimp"},
			{ConfigurationID: 11, GradeIndex: 2, Dominant: "sloper"},
		}
		st := ScoreState{
			PreferredIndex:        3,
			RecentDominants:       []string{"crimp", "crimp", "crimp"},
			SessionDominantCounts: map[string]int{"crimp": 3},
			PrevDominant:          "crimp",
			DropBalance:           true,
		}
		got, err := scoreNext(cands, st, rollZero)
		if err != nil || cands[got].ConfigurationID != 10 {
			t.Fatalf("got cfg %d, want on-grade crimp (10)", cands[got].ConfigurationID)
		}
	})

	t.Run("exact tie is deterministic via roll", func(t *testing.T) {
		cands := []ScoreCandidate{
			{ConfigurationID: 1, GradeIndex: 2, Dominant: "jug"},
			{ConfigurationID: 2, GradeIndex: 2, Dominant: "jug"},
		}
		st := ScoreState{PreferredIndex: 2, PrevDominant: "jug"}
		got, err := scoreNext(cands, st, func(int) int { return 1 })
		if err != nil || got != 1 {
			t.Fatalf("got idx %d, want 1 from roll", got)
		}
	})
}
