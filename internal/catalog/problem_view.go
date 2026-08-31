package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProblemView is everything the session page needs to render one problem.
type ProblemView struct {
	ProblemID int64
	Name      string
	Grade     string
	BoardYear string
	Angle     int16
	Holds     []HoldPlacement
}

// HoldPlacement is one hold used by a problem, with its grid position, render
// role, and hold-type tags.
type HoldPlacement struct {
	Seq         int
	MoveType    string
	GridRef     string
	PrimaryType string
	Modifiers   []string
	Col         int
	Row         int
	Role        string
}

// moveTypeRole maps a problem_moves.move_type code to a render role. The
// observed codes are s/e/l/r/p/m/f/o (see the plan's Current State Analysis).
func moveTypeRole(moveType string) string {
	switch moveType {
	case "s":
		return "start"
	case "e":
		return "finish"
	case "f":
		return "foot"
	default:
		return "hand"
	}
}

// ProblemDetail loads one problem_configuration and its ordered holds.
func ProblemDetail(ctx context.Context, pool *pgxpool.Pool, configurationID int64) (*ProblemView, error) {
	var (
		view      ProblemView
		holdsetup int16
	)
	err := pool.QueryRow(ctx, `
		SELECT p.id, p.name, pc.grade, pc.angle, p.holdsetup
		FROM problem_configurations pc
		JOIN problems p ON p.id = pc.problem_id
		WHERE pc.id = $1
	`, configurationID).Scan(&view.ProblemID, &view.Name, &view.Grade, &view.Angle, &holdsetup)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("catalog: problem detail %d: %w", configurationID, ErrConfigurationNotFound)
		}
		return nil, fmt.Errorf("catalog: query problem detail: %w", err)
	}
	view.BoardYear = BoardYears[int(holdsetup)]

	rows, err := pool.Query(ctx, `
		SELECT pm.seq, pm.move_type, pm.grid_ref, h.primary_type, h.modifiers
		FROM problem_moves pm
		LEFT JOIN holds h ON h.holdsetup = pm.holdsetup AND h.grid_ref = pm.grid_ref
		WHERE pm.problem_id = $1 AND pm.holdsetup = $2
		ORDER BY pm.seq
	`, view.ProblemID, holdsetup)
	if err != nil {
		return nil, fmt.Errorf("catalog: query problem holds: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			h           HoldPlacement
			primaryType *string
			modifiers   []string
		)
		if err := rows.Scan(&h.Seq, &h.MoveType, &h.GridRef, &primaryType, &modifiers); err != nil {
			return nil, fmt.Errorf("catalog: scan hold placement: %w", err)
		}
		if primaryType != nil {
			h.PrimaryType = *primaryType
		}
		h.Modifiers = modifiers
		h.Role = moveTypeRole(h.MoveType)

		if col, row, perr := ParseGridRef(h.GridRef); perr == nil {
			h.Col = colToIndex(col)
			h.Row = row
		}

		view.Holds = append(view.Holds, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate hold placements: %w", err)
	}

	return &view, nil
}

// ErrConfigurationNotFound is returned by ProblemDetail when no
// problem_configuration has the given id.
var ErrConfigurationNotFound = errors.New("catalog: problem configuration not found")

// colToIndex turns a single-letter column ("A".."K") into a 0-based index.
// Multi-letter columns (not used on any MoonBoard) fall back to 0.
func colToIndex(col string) int {
	if len(col) != 1 || col[0] < 'A' || col[0] > 'Z' {
		return 0
	}
	return int(col[0] - 'A')
}
