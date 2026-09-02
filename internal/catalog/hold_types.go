package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// execQuerier is the minimal write surface RecomputeHoldTypes needs. Both
// *pgxpool.Pool and pgx.Tx satisfy it, so the tagging path, the ingest path,
// and a CLI can all call the same helper.
type execQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// holdTypesAggregateSQL rebuilds problem_hold_types rows from problem_moves +
// holds. It mirrors the 0010 backfill exactly. $1 is a bigint[] of problem
// ids; an empty array rebuilds every row.
const holdTypesAggregateSQL = `
	INSERT INTO problem_hold_types
		(problem_id, crimp, sloper, pinch, jug, pocket, unknown, total_scored, dominant)
	SELECT
		pm.problem_id,
		count(*) FILTER (WHERE h.primary_type = 'crimp'),
		count(*) FILTER (WHERE h.primary_type = 'sloper'),
		count(*) FILTER (WHERE h.primary_type = 'pinch'),
		count(*) FILTER (WHERE h.primary_type = 'jug'),
		count(*) FILTER (WHERE h.primary_type = 'pocket'),
		count(*) FILTER (WHERE h.primary_type IS NULL OR h.primary_type NOT IN ('crimp', 'sloper', 'pinch', 'jug', 'pocket')),
		count(*),
		(
			SELECT t.name
			FROM (VALUES
				('crimp',  count(*) FILTER (WHERE h.primary_type = 'crimp')),
				('jug',    count(*) FILTER (WHERE h.primary_type = 'jug')),
				('pinch',  count(*) FILTER (WHERE h.primary_type = 'pinch')),
				('pocket', count(*) FILTER (WHERE h.primary_type = 'pocket')),
				('sloper', count(*) FILTER (WHERE h.primary_type = 'sloper'))
			) AS t(name, n)
			WHERE t.n > 0
			ORDER BY t.n DESC, t.name ASC
			LIMIT 1
		)
	FROM problem_moves pm
	JOIN holds h ON h.holdsetup = pm.holdsetup AND h.grid_ref = pm.grid_ref
	WHERE pm.move_type <> 'f'
	  AND (cardinality($1::bigint[]) = 0 OR pm.problem_id = ANY($1))
	GROUP BY pm.problem_id
`

// RecomputeHoldTypes rebuilds the problem_hold_types rows for the named
// problems (or every row when none are passed) as one DELETE + one
// INSERT...SELECT, reusing the 0010 aggregate. db may be a pool or a
// transaction, so callers keep composition data current wherever
// holds.primary_type or problem_moves change.
func RecomputeHoldTypes(ctx context.Context, db execQuerier, problemIDs ...int64) error {
	ids := problemIDs
	if ids == nil {
		ids = []int64{}
	}

	if len(ids) == 0 {
		if _, err := db.Exec(ctx, `DELETE FROM problem_hold_types`); err != nil {
			return fmt.Errorf("catalog: clear problem_hold_types: %w", err)
		}
	} else {
		if _, err := db.Exec(ctx, `DELETE FROM problem_hold_types WHERE problem_id = ANY($1)`, ids); err != nil {
			return fmt.Errorf("catalog: clear problem_hold_types rows: %w", err)
		}
	}

	if _, err := db.Exec(ctx, holdTypesAggregateSQL, ids); err != nil {
		return fmt.Errorf("catalog: rebuild problem_hold_types: %w", err)
	}

	return nil
}

// problemIDsForGridRefs returns every problem id whose moves reference any of
// the given grid refs on holdsetup. Used by ApplyTags to scope a recompute to
// just the problems a retag pass can affect.
func problemIDsForGridRefs(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, holdsetup int, gridRefs []string,
) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT problem_id
		FROM problem_moves
		WHERE holdsetup = $1 AND grid_ref = ANY($2)
	`, holdsetup, gridRefs)
	if err != nil {
		return nil, fmt.Errorf("catalog: query problems for grid refs: %w", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("catalog: scan problem id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: iterate problem ids: %w", err)
	}

	return out, nil
}

// poolQuerier is the read+write surface RecomputeHoldTypesForBoard needs.
type poolQuerier interface {
	execQuerier
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// RecomputeHoldTypesForBoard rebuilds every problem_hold_types row for the
// problems on holdsetup. The manual full-rebuild escape hatch behind the
// `catalog holds recompute-composition` CLI.
func RecomputeHoldTypesForBoard(ctx context.Context, db poolQuerier, holdsetup int) (int, error) {
	rows, err := db.Query(ctx, `SELECT id FROM problems WHERE holdsetup = $1`, holdsetup)
	if err != nil {
		return 0, fmt.Errorf("catalog: query board problems: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("catalog: scan board problem id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("catalog: iterate board problem ids: %w", err)
	}

	if len(ids) == 0 {
		return 0, nil
	}
	if err := RecomputeHoldTypes(ctx, db, ids...); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// DominantHoldType returns the plurality hold type in counts, breaking ties
// alphabetically. Zero counts are ignored; an all-zero (or empty) map returns
// "". Kept pure for reuse in tests and session-state assembly.
func DominantHoldType(counts map[string]int) string {
	best := ""
	bestN := 0
	for _, name := range AllowedHoldTypes {
		n := counts[name]
		if n <= 0 {
			continue
		}
		if n > bestN || (n == bestN && (best == "" || name < best)) {
			best = name
			bestN = n
		}
	}
	return best
}
