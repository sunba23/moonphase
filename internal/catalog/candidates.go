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
// already carrying its dominant hold type from problem_hold_types.
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

// NextPickCandidates runs a single index-friendly statement: a
// problem_configurations range scan on
// idx_problem_configurations_holdsetup_angle_grade joined flat to the
// problem_hold_types PK. No aggregation at request time.
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
		SELECT pc.problem_id, pc.id, pc.grade, pc.is_benchmark, pc.repeats,
		       COALESCE(pht.dominant, '')
		FROM problem_configurations pc
		JOIN problem_hold_types pht ON pht.problem_id = pc.problem_id
		WHERE pc.holdsetup = $1 AND pc.angle = $2
		  AND pc.grade <> '' AND pc.grade >= $3 AND pc.grade <= $4
		  AND (cardinality($5::bigint[]) = 0 OR pc.problem_id <> ALL($5))
		  AND ($6 = '' OR COALESCE(pht.dominant, '') <> $6)
		LIMIT $7
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
