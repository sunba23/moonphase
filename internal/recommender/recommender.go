// Package recommender owns the policy of which problem to recommend. This
// slice (S-03) implements only FirstPick (FR-011: session starts at the
// board+angle minimum grade). S-04 adds PickNext for the adaptive loop.
package recommender

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/catalog"
)

// Candidate is one problem the first pick may choose. Field layout matches
// catalog.ProblemCandidate for a direct struct conversion.
type Candidate struct {
	ProblemID       int64
	ConfigurationID int64
	Grade           string
	IsBenchmark     bool
	Repeats         int
}

// Pick is the recommender's output: one problem to show next.
type Pick struct {
	ProblemID       int64
	ConfigurationID int64
	Grade           string
}

// ErrNoCandidates means the catalog returned nothing for the board+angle —
// an ops/data problem, surfaced as a 500 by the handler.
var ErrNoCandidates = errors.New("recommender: no candidates")

// minQualityRepeats is the community-repeat count that lets a non-benchmark
// problem into the quality-filtered pool.
const minQualityRepeats = 5

// pickFrom applies the quality filter (benchmark OR well-repeated), falls
// back to the full set when the filter empties it, and rolls one pick. Pure
// and deterministic given roll.
func pickFrom(cands []Candidate, roll func(n int) int) (Pick, error) {
	quality := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.IsBenchmark || c.Repeats >= minQualityRepeats {
			quality = append(quality, c)
		}
	}

	pool := quality
	if len(pool) == 0 {
		pool = cands
	}
	if len(pool) == 0 {
		return Pick{}, ErrNoCandidates
	}

	c := pool[roll(len(pool))]
	return Pick{ProblemID: c.ProblemID, ConfigurationID: c.ConfigurationID, Grade: c.Grade}, nil
}

// Recommender answers first-pick queries against the catalog.
type Recommender struct {
	pool *pgxpool.Pool
	rng  *rand.Rand
}

func New(pool *pgxpool.Pool) *Recommender {
	return &Recommender{
		pool: pool,
		//nolint:gosec // non-crypto: shuffling equally-graded problems
		rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
	}
}

// ShownState is one already-shown problem, as PickNext needs it: id, grade,
// and dominant hold type. The caller builds this from session.ShownProblem so
// the recommender never imports internal/session.
type ShownState struct {
	ProblemID int64
	Grade     string
	Dominant  string
}

// PickNextInput is everything PickNext weighs. Shown is seq-ordered; the last
// entry is the just-climbed problem that CurrentResult describes.
type PickNextInput struct {
	Holdsetup       int16
	Angle           int16
	SessionMaxGrade string
	Shown           []ShownState
	CurrentResult   Result
}

// PickDiag reports how PickNext reached its answer, for handler-side logging.
// FallbackTier 0 means the primary query succeeded.
type PickDiag struct {
	FallbackTier     int
	GradeLo          string
	GradeHi          string
	ExcludedDominant string
}

const nextPickLimit = 500

// lastDominants returns the dominant hold types of the last n shown problems,
// oldest-first.
func lastDominants(shown []ShownState, n int) []string {
	start := len(shown) - n
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, len(shown)-start)
	for _, s := range shown[start:] {
		out = append(out, s.Dominant)
	}
	return out
}

// toScoreCandidates maps catalog rows onto the pure scorer's input, dropping
// any row whose grade is not on the ladder.
func toScoreCandidates(cands []catalog.NextCandidate, ladder []string) []ScoreCandidate {
	out := make([]ScoreCandidate, 0, len(cands))
	for _, c := range cands {
		gi := indexOf(ladder, c.Grade)
		if gi < 0 {
			continue
		}
		out = append(out, ScoreCandidate{
			ProblemID:       c.ProblemID,
			ConfigurationID: c.ConfigurationID,
			GradeIndex:      gi,
			Dominant:        c.Dominant,
			Quality:         c.IsBenchmark || c.Repeats >= minQualityRepeats,
		})
	}
	return out
}

// PickNext runs the adaptive loop's next-problem pick: build the RPE→grade
// window, clamp it to the session ceiling, score the composition-joined
// candidate pool, and walk the 4-tier empty-pool fallback. Deterministic
// given r.rng.
func (r *Recommender) PickNext(ctx context.Context, in PickNextInput) (Pick, PickDiag, error) {
	var diag PickDiag
	if len(in.Shown) == 0 {
		return Pick{}, diag, fmt.Errorf("recommender: pick next: no shown problems")
	}

	ladder, err := catalog.GradeLadder(ctx, r.pool, in.Holdsetup, in.Angle)
	if err != nil {
		return Pick{}, diag, fmt.Errorf("recommender: pick next: grade ladder: %w", err)
	}
	if len(ladder) == 0 {
		return Pick{}, diag, ErrNoCandidates
	}

	last := in.Shown[len(in.Shown)-1]
	b := classify(in.CurrentResult)
	lo, hi, _ := gradeWindow(ladder, last.Grade, b)
	hi = minGradeOnLadder(ladder, hi, in.SessionMaxGrade)
	// lo must never exceed hi after the ceiling clamp.
	if indexOf(ladder, lo) > indexOf(ladder, hi) {
		lo = hi
	}
	diag.GradeLo, diag.GradeHi = lo, hi

	shownIDs := make([]int64, 0, len(in.Shown))
	sessionCounts := make(map[string]int, 5)
	for _, s := range in.Shown {
		shownIDs = append(shownIDs, s.ProblemID)
		if s.Dominant != "" {
			sessionCounts[s.Dominant]++
		}
	}
	recent := lastDominants(in.Shown, 3)

	prefIdx := preferredIndex(ladder, last.Grade, b)
	if hiIdx := indexOf(ladder, hi); hiIdx >= 0 && prefIdx > hiIdx {
		prefIdx = hiIdx
	}
	if loIdx := indexOf(ladder, lo); loIdx >= 0 && prefIdx < loIdx {
		prefIdx = loIdx
	}

	st := ScoreState{
		PreferredIndex:        prefIdx,
		RecentDominants:       recent,
		SessionDominantCounts: sessionCounts,
		PrevDominant:          last.Dominant,
	}

	// Streak exclude: last 3 shown dominants all equal and non-empty.
	excludeDominant := ""
	if len(recent) == 3 && recent[0] != "" && recent[0] == recent[1] && recent[1] == recent[2] {
		excludeDominant = recent[0]
	}
	diag.ExcludedDominant = excludeDominant

	pick, ok, err := r.tieredPick(ctx, in, ladder, lo, hi, shownIDs, excludeDominant, st, &diag)
	if err != nil {
		return Pick{}, diag, err
	}
	if !ok {
		return Pick{}, diag, ErrNoCandidates
	}
	return pick, diag, nil
}

// tieredPick walks fallback tiers 0–4, setting diag.FallbackTier as it goes.
func (r *Recommender) tieredPick(
	ctx context.Context, in PickNextInput, ladder []string,
	lo, hi string, shownIDs []int64, excludeDominant string,
	st ScoreState, diag *PickDiag,
) (Pick, bool, error) {
	score := func(cands []catalog.NextCandidate, s ScoreState) (Pick, bool) {
		sc := toScoreCandidates(cands, ladder)
		if len(sc) == 0 {
			return Pick{}, false
		}
		idx, err := scoreNext(sc, s, r.rng.IntN)
		if err != nil {
			return Pick{}, false
		}
		c := sc[idx]
		return Pick{ProblemID: c.ProblemID, ConfigurationID: c.ConfigurationID, Grade: ladder[c.GradeIndex]}, true
	}

	// Tier 0 — full window, exclude shown + streak dominant.
	diag.FallbackTier = 0
	cands, err := catalog.NextPickCandidates(ctx, r.pool, catalog.NextPickQuery{
		Holdsetup: in.Holdsetup, Angle: in.Angle, GradeMin: lo, GradeMax: hi,
		ExcludeProblemIDs: shownIDs, ExcludeDominant: excludeDominant, Limit: nextPickLimit,
	})
	if err != nil {
		return Pick{}, false, fmt.Errorf("recommender: tier 0 candidates: %w", err)
	}
	if p, ok := score(cands, st); ok {
		return p, true, nil
	}

	// Tier 1 — same window + exclude shown, drop the hold-type penalty.
	diag.FallbackTier = 1
	cands, err = catalog.NextPickCandidates(ctx, r.pool, catalog.NextPickQuery{
		Holdsetup: in.Holdsetup, Angle: in.Angle, GradeMin: lo, GradeMax: hi,
		ExcludeProblemIDs: shownIDs, Limit: nextPickLimit,
	})
	if err != nil {
		return Pick{}, false, fmt.Errorf("recommender: tier 1 candidates: %w", err)
	}
	st1 := st
	st1.DropBalance = true
	if p, ok := score(cands, st1); ok {
		return p, true, nil
	}

	// Tier 2 — step lo down the ladder one entry at a time, keep hi.
	diag.FallbackTier = 2
	loIdx := indexOf(ladder, lo)
	for i := loIdx - 1; i >= 0; i-- {
		cands, err = catalog.NextPickCandidates(ctx, r.pool, catalog.NextPickQuery{
			Holdsetup: in.Holdsetup, Angle: in.Angle, GradeMin: ladder[i], GradeMax: hi,
			ExcludeProblemIDs: shownIDs, Limit: nextPickLimit,
		})
		if err != nil {
			return Pick{}, false, fmt.Errorf("recommender: tier 2 candidates: %w", err)
		}
		if p, ok := score(cands, st1); ok {
			return p, true, nil
		}
	}

	// Tier 3 — drop the exclude-shown filter; window [ladder[0], hi].
	diag.FallbackTier = 3
	cands, err = catalog.NextPickCandidates(ctx, r.pool, catalog.NextPickQuery{
		Holdsetup: in.Holdsetup, Angle: in.Angle, GradeMin: ladder[0], GradeMax: hi,
		Limit: nextPickLimit,
	})
	if err != nil {
		return Pick{}, false, fmt.Errorf("recommender: tier 3 candidates: %w", err)
	}
	if p, ok := score(cands, st1); ok {
		return p, true, nil
	}

	// Tier 4 — FirstPick-style: min-grade random, filtered to the ceiling.
	diag.FallbackTier = 4
	minCands, err := catalog.MinGradeCandidates(ctx, r.pool, in.Holdsetup, in.Angle)
	if err != nil {
		return Pick{}, false, fmt.Errorf("recommender: tier 4 candidates: %w", err)
	}
	mapped := make([]Candidate, 0, len(minCands))
	for _, c := range minCands {
		if in.SessionMaxGrade != "" && indexOf(ladder, c.Grade) > indexOf(ladder, in.SessionMaxGrade) {
			continue
		}
		mapped = append(mapped, Candidate(c))
	}
	p, err := pickFrom(mapped, r.rng.IntN)
	if err != nil {
		return Pick{}, false, err
	}
	return p, true, nil
}

// FirstPick returns a problem at the minimum grade available on
// (holdsetup, angle), quality-filtered where possible.
func (r *Recommender) FirstPick(ctx context.Context, holdsetup, angle int16) (Pick, error) {
	cands, err := catalog.MinGradeCandidates(ctx, r.pool, holdsetup, angle)
	if err != nil {
		return Pick{}, fmt.Errorf("recommender: first pick: %w", err)
	}

	mapped := make([]Candidate, len(cands))
	for i, c := range cands {
		mapped[i] = Candidate(c)
	}

	return pickFrom(mapped, r.rng.IntN)
}
