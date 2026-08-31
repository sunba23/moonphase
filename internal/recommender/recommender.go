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
