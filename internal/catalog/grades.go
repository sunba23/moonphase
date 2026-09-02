package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GradeLadder returns the ascending list of distinct non-blank grades offered
// on (holdsetup, angle). Font-scale grades sort lexically in difficulty order
// (6B < 6B+ < 6C < 6C+ < 7A), which the repo already relies on. The window
// logic steps by real catalog grades, so a board that skips a grade never
// produces an off-ladder step.
func GradeLadder(ctx context.Context, pool *pgxpool.Pool, holdsetup, angle int16) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT grade
		FROM problem_configurations
		WHERE holdsetup = $1 AND angle = $2 AND grade <> ''
		ORDER BY grade
	`, holdsetup, angle)
	if err != nil {
		return nil, fmt.Errorf("catalog: query grade ladder: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, fmt.Errorf("catalog: scan grade: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate grade ladder: %w", err)
	}

	return out, nil
}
