package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HoldStore is the DB-backed half of the hold-tagging workflow: read the
// auto-discovered inventory, write back a hand-filled pass, and report
// per-board coverage.
type HoldStore struct {
	pool *pgxpool.Pool
}

func NewHoldStore(pool *pgxpool.Pool) *HoldStore {
	return &HoldStore{pool: pool}
}

// Inventory returns every hold auto-discovered for holdsetup, already-tagged
// rows pre-filled -- so re-running WriteInventoryCSV against this after a
// future re-ingest only appends new blank rows, preserving prior tagging
// work.
func (s *HoldStore) Inventory(ctx context.Context, holdsetup int) ([]HoldRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT grid_ref, primary_type, modifiers
		FROM holds
		WHERE holdsetup = $1
	`, holdsetup)
	if err != nil {
		return nil, fmt.Errorf("catalog: query hold inventory: %w", err)
	}
	defer rows.Close()

	var out []HoldRow
	for rows.Next() {
		var (
			gridRef     string
			primaryType *string
			modifiers   []string
		)
		if err := rows.Scan(&gridRef, &primaryType, &modifiers); err != nil {
			return nil, fmt.Errorf("catalog: scan hold row: %w", err)
		}

		row := HoldRow{GridRef: gridRef, Modifiers: modifiers}
		if primaryType != nil {
			row.PrimaryType = *primaryType
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate hold inventory: %w", err)
	}

	return out, nil
}

// validateHoldRows checks every non-blank-PrimaryType row's PrimaryType
// against ValidateHoldType, collecting all errors rather than stopping at
// the first one -- so ApplyTags can report every bad row in a single pass
// instead of forcing the operator through one-error-at-a-time re-runs. Rows
// with a blank PrimaryType are left untouched -- a partially-filled load is
// the expected mid-pass case, not an error. Pure and DB-independent so it's
// directly unit-testable.
func validateHoldRows(rows []HoldRow) error {
	var errs []error
	for _, r := range rows {
		if r.PrimaryType == "" {
			continue
		}
		if err := ValidateHoldType(r.PrimaryType); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.GridRef, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("catalog: invalid hold tags, applied nothing: %w", errors.Join(errs...))
	}
	return nil
}

// ApplyTags validates every row via validateHoldRows up front and applies
// nothing if any row is invalid, before ever opening a transaction.
func (s *HoldStore) ApplyTags(ctx context.Context, holdsetup int, rows []HoldRow) error {
	if err := validateHoldRows(rows); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("catalog: begin apply-tags tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var unmatched []string
	var tagged []string
	for _, r := range rows {
		if r.PrimaryType == "" {
			continue
		}
		tagged = append(tagged, r.GridRef)

		modifiers := r.Modifiers
		if modifiers == nil {
			// pgx encodes a nil []string as SQL NULL, not '{}', which
			// violates modifiers' NOT NULL constraint for holds with no
			// modifiers tagged.
			modifiers = []string{}
		}

		tag, err := tx.Exec(ctx, `
			UPDATE holds
			SET primary_type = $1, modifiers = $2, is_tagged = true, tagged_at = now()
			WHERE holdsetup = $3 AND grid_ref = $4
		`, r.PrimaryType, modifiers, holdsetup, r.GridRef)
		if err != nil {
			return fmt.Errorf("catalog: update hold %s: %w", r.GridRef, err)
		}
		if tag.RowsAffected() == 0 {
			unmatched = append(unmatched, r.GridRef)
		}
	}

	if len(unmatched) > 0 {
		return fmt.Errorf("catalog: no matching hold for grid_ref(s) %s on holdsetup %d (typo, or board not yet ingested); applied nothing", strings.Join(unmatched, ", "), holdsetup)
	}

	// Keep problem_hold_types current for every problem whose moves reference a
	// retagged grid ref, in the same transaction as the tag write.
	if len(tagged) > 0 {
		ids, err := problemIDsForGridRefs(ctx, tx, holdsetup, tagged)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := RecomputeHoldTypes(ctx, tx, ids...); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("catalog: commit apply-tags tx: %w", err)
	}

	return nil
}

// BoardStatus reports tagging coverage for one board.
type BoardStatus struct {
	Holdsetup int
	Tagged    int
	Total     int
}

// Status reports per-board tagging coverage, optionally filtered to a single
// board.
func (s *HoldStore) Status(ctx context.Context, holdsetup *int) ([]BoardStatus, error) {
	query := `
		SELECT holdsetup, count(*) FILTER (WHERE is_tagged), count(*)
		FROM holds
	`

	args := []any{}
	if holdsetup != nil {
		query += " WHERE holdsetup = $1"
		args = append(args, *holdsetup)
	}
	query += " GROUP BY holdsetup ORDER BY holdsetup"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog: query hold status: %w", err)
	}
	defer rows.Close()

	var out []BoardStatus
	for rows.Next() {
		var bs BoardStatus
		if err := rows.Scan(&bs.Holdsetup, &bs.Tagged, &bs.Total); err != nil {
			return nil, fmt.Errorf("catalog: scan hold status: %w", err)
		}
		out = append(out, bs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate hold status: %w", err)
	}

	return out, nil
}
