package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProblemCandidate is one problem_configuration eligible to be a session's
// first pick. Field layout matches recommender.Candidate for a direct
// struct conversion.
type ProblemCandidate struct {
	ProblemID       int64
	ConfigurationID int64
	Grade           string
	IsBenchmark     bool
	Repeats         int
}

// NextCandidate is one problem_configuration eligible to be the next pick,
// already carrying its dominant hold type from problem_hold_types. The slice
// order is not meaningful — NextPickCandidates returns a random sample.
type NextCandidate struct {
	ProblemID       int64
	ConfigurationID int64
	Grade           string
	IsBenchmark     bool
	Repeats         int
	Dominant        string
}

// NextPickQuery parameterises the flat next-pick candidate scan. The caller
// pre-clamps GradeMax to session.max_grade.
type NextPickQuery struct {
	Holdsetup         int16
	Angle             int16
	GradeMin          string
	GradeMax          string
	ExcludeProblemIDs []int64
	ExcludeDominant   string
	Limit             int
}

// NextPickCandidates returns a bounded random sample of the problem_configurations
// in the grade window, each carrying its dominant hold type.
//
// The sample is taken from problem_configurations ALONE (an
// idx_problem_configurations_holdsetup_angle_grade range scan + a bounded
// top-N heapsort on ORDER BY random()); problem_hold_types is PK-joined only
// for the <=Limit sampled rows. Sampling before the join is what keeps the
// join a cheap nested loop — ORDER BY random() over the joined query makes the
// planner Seq-Scan every problem_hold_types row. The random sample is what
// lets the loop ramp: without it the LIMIT takes the lexicographically-lowest
// grade's rows and a dense grade can never be climbed past.
//
// ExcludeDominant is applied after the join, so a set ExcludeDominant can
// return fewer than Limit rows (only used on a hold-type streak, tier 0; the
// caller's tier 1 retries without it). No aggregation at request time.
func NextPickCandidates(ctx context.Context, pool *pgxpool.Pool, q NextPickQuery) ([]NextCandidate, error) {
	exclude := q.ExcludeProblemIDs
	if exclude == nil {
		exclude = []int64{}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 500
	}

	rows, err := pool.Query(ctx, `
		WITH sampled AS (
			SELECT pc.problem_id, pc.id, pc.grade, pc.is_benchmark, pc.repeats
			FROM problem_configurations pc
			WHERE pc.holdsetup = $1 AND pc.angle = $2
			  AND pc.grade <> '' AND pc.grade >= $3 AND pc.grade <= $4
			  AND (cardinality($5::bigint[]) = 0 OR pc.problem_id <> ALL($5))
			ORDER BY random()
			LIMIT $7
		)
		SELECT s.problem_id, s.id, s.grade, s.is_benchmark, s.repeats,
		       COALESCE(pht.dominant, '')
		FROM sampled s
		JOIN problem_hold_types pht ON pht.problem_id = s.problem_id
		WHERE ($6 = '' OR COALESCE(pht.dominant, '') <> $6)
	`, q.Holdsetup, q.Angle, q.GradeMin, q.GradeMax, exclude, q.ExcludeDominant, limit)
	if err != nil {
		return nil, fmt.Errorf("catalog: query next-pick candidates: %w", err)
	}
	defer rows.Close()

	var out []NextCandidate
	for rows.Next() {
		var c NextCandidate
		if err := rows.Scan(&c.ProblemID, &c.ConfigurationID, &c.Grade, &c.IsBenchmark, &c.Repeats, &c.Dominant); err != nil {
			return nil, fmt.Errorf("catalog: scan next candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate next candidates: %w", err)
	}

	return out, nil
}

// MinGradeCandidates returns every problem_configuration at the lowest
// non-blank grade offered on (holdsetup, angle). Backed by
// idx_problem_configurations_holdsetup_angle_grade.
func MinGradeCandidates(ctx context.Context, pool *pgxpool.Pool, holdsetup, angle int16) ([]ProblemCandidate, error) {
	rows, err := pool.Query(ctx, `
		WITH min_grade AS (
			SELECT grade
			FROM problem_configurations
			WHERE holdsetup = $1 AND angle = $2 AND grade <> ''
			ORDER BY grade
			LIMIT 1
		)
		SELECT pc.problem_id, pc.id, pc.grade, pc.is_benchmark, pc.repeats
		FROM problem_configurations pc
		JOIN min_grade ON pc.grade = min_grade.grade
		WHERE pc.holdsetup = $1 AND pc.angle = $2
	`, holdsetup, angle)
	if err != nil {
		return nil, fmt.Errorf("catalog: query min-grade candidates: %w", err)
	}
	defer rows.Close()

	var out []ProblemCandidate
	for rows.Next() {
		var c ProblemCandidate
		if err := rows.Scan(&c.ProblemID, &c.ConfigurationID, &c.Grade, &c.IsBenchmark, &c.Repeats); err != nil {
			return nil, fmt.Errorf("catalog: scan candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate candidates: %w", err)
	}

	return out, nil
}
