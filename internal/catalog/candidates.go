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
