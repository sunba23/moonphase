package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BoardEdition is one selectable MoonBoard set, as offered by the onboarding
// form's board dropdown.
type BoardEdition struct {
	Holdsetup int16
	Name      string
}

// DistinctGrades returns every non-blank grade in use across the catalog,
// Font-scale-sorted (the scale's lexical order already matches numeric
// order, per shape-notes.md).
func DistinctGrades(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT grade
		FROM problem_configurations
		WHERE grade <> ''
		ORDER BY grade
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog: query distinct grades: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var grade string
		if err := rows.Scan(&grade); err != nil {
			return nil, fmt.Errorf("catalog: scan grade: %w", err)
		}
		out = append(out, grade)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate distinct grades: %w", err)
	}

	return out, nil
}

// BoardEditions returns every supported MoonBoard set.
func BoardEditions(ctx context.Context, pool *pgxpool.Pool) ([]BoardEdition, error) {
	rows, err := pool.Query(ctx, `
		SELECT holdsetup, name
		FROM board_editions
		ORDER BY holdsetup
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog: query board editions: %w", err)
	}
	defer rows.Close()

	var out []BoardEdition
	for rows.Next() {
		var be BoardEdition
		if err := rows.Scan(&be.Holdsetup, &be.Name); err != nil {
			return nil, fmt.Errorf("catalog: scan board edition: %w", err)
		}
		out = append(out, be)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate board editions: %w", err)
	}

	return out, nil
}

// DistinctAngles returns every angle in use across the catalog.
func DistinctAngles(ctx context.Context, pool *pgxpool.Pool) ([]int16, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT angle
		FROM problem_configurations
		ORDER BY angle
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog: query distinct angles: %w", err)
	}
	defer rows.Close()

	var out []int16
	for rows.Next() {
		var angle int16
		if err := rows.Scan(&angle); err != nil {
			return nil, fmt.Errorf("catalog: scan angle: %w", err)
		}
		out = append(out, angle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate distinct angles: %w", err)
	}

	return out, nil
}
