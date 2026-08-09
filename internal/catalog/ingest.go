package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// batchSize is how many problems are written per transaction on the fast
// path. A per-problem transaction (one Begin/Commit and ~12-20 sequential
// round trips each) measured under 1 problem/sec against Supabase's pooled
// connection; grouping problems into pipelined batches cuts that to ~4 round
// trips per batch regardless of size.
const batchSize = 200

// sqlTime converts a parsed SourceTime to a *time.Time pgx can bind to a
// TIMESTAMPTZ column -- pgx's codec system matches time.Time exactly, not
// the SourceTime named type.
func sqlTime(t *SourceTime) *time.Time {
	if t == nil {
		return nil
	}

	tt := t.Time()
	return &tt
}

// KnownBoards maps the 4 seeded board_editions.holdsetup codes to their
// display names, mirroring migrations/0001_board_editions.up.sql.
var KnownBoards = map[int]string{
	1:  "2016",
	15: "Masters 2017",
	17: "Masters 2019",
	21: "2024",
}

// ProblemError records a single problem that failed to ingest. Ingestion of
// one export file continues past these -- one bad problem shouldn't abort
// the other ~90K in the same file.
type ProblemError struct {
	ExternalID int
	Err        error
}

func (e ProblemError) Error() string {
	return fmt.Sprintf("problem %d: %v", e.ExternalID, e.Err)
}

// Summary reports what one IngestFile call did.
type Summary struct {
	ProblemsSeen    int
	Ingested        int
	SkippedInactive int
	SkippedDeleted  int
	ConfigsWritten  int
	MovesWritten    int
	NewHoldsFound   int
	Errors          []ProblemError
}

type Ingester struct {
	pool *pgxpool.Pool
}

func NewIngester(pool *pgxpool.Pool) *Ingester {
	return &Ingester{pool: pool}
}

// parsedConfig is a Configuration with its angle fields pre-parsed.
type parsedConfig struct {
	cfg          Configuration
	angle        int
	primaryAngle *int
}

// preparedProblem is a Problem with its moves and configs already parsed and
// validated in Go, ready to be written to the DB -- either via the batched
// fast path or, on that batch's failure, the sequential per-problem fallback.
type preparedProblem struct {
	problem Problem
	moves   []Move
	configs []parsedConfig
}

// prepareProblem parses a problem's moves and configs. Pure Go, no DB --
// failures here are per-problem and never reach SQL.
func prepareProblem(p Problem) (preparedProblem, error) {
	moves, err := ParseMoves(p.Moves)
	if err != nil {
		return preparedProblem{}, fmt.Errorf("parse moves: %w", err)
	}

	configs := make([]parsedConfig, 0, len(p.Configurations))
	for _, cfg := range p.Configurations {
		angle, err := ParseAngle(cfg.Configuration)
		if err != nil {
			return preparedProblem{}, fmt.Errorf("parse config %d angle: %w", cfg.APIID, err)
		}

		var primaryAngle *int
		if cfg.PrimaryAngle != "" {
			pa, err := ParseAngle(cfg.PrimaryAngle)
			if err != nil {
				return preparedProblem{}, fmt.Errorf("parse config %d primary angle: %w", cfg.APIID, err)
			}
			primaryAngle = &pa
		}

		configs = append(configs, parsedConfig{cfg: cfg, angle: angle, primaryAngle: primaryAngle})
	}

	return preparedProblem{problem: p, moves: moves, configs: configs}, nil
}

// IngestFile stream-decodes one board-edition export file and writes
// qualifying (active, non-deleted) problems in pipelined batches of
// batchSize. An unrecognized holdsetup is fatal for the whole file -- it
// signals a file/DB mismatch -- while a single problem's parse or write
// failure is collected in the returned Summary and does not abort the rest
// of the file.
func (i *Ingester) IngestFile(ctx context.Context, path string, dryRun bool) (Summary, error) {
	f, err := os.Open(path) //nolint:gosec // path is an operator-supplied CLI flag, not untrusted input
	if err != nil {
		return Summary{}, fmt.Errorf("catalog: open export file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var summary Summary
	buf := make([]preparedProblem, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		i.ingestBatch(ctx, buf, dryRun, &summary)
		buf = buf[:0]
	}

	onHoldsetup := func(holdsetup int) error {
		if _, ok := KnownBoards[holdsetup]; !ok {
			return fmt.Errorf("catalog: unrecognized holdsetup %d in %s (expected one of 1, 15, 17, 21)", holdsetup, path)
		}
		return nil
	}

	onProblem := func(p Problem) error {
		summary.ProblemsSeen++

		if !p.Active {
			summary.SkippedInactive++
			return nil
		}
		if p.DateDeleted != nil {
			summary.SkippedDeleted++
			return nil
		}

		pp, err := prepareProblem(p)
		if err != nil {
			// Captured in summary.Errors, not swallowed -- one bad problem must
			// not abort the rest of the file.
			summary.Errors = append(summary.Errors, ProblemError{ExternalID: p.ID, Err: err})
			return nil //nolint:nilerr
		}

		buf = append(buf, pp)
		if len(buf) >= batchSize {
			flush()
		}

		return nil
	}

	if err := decodeExportStream(f, onHoldsetup, onProblem); err != nil {
		return summary, err
	}

	flush()

	return summary, nil
}

// ingestBatch writes a batch of already-parsed problems. It always tries the
// pipelined fast path first; if that whole-batch transaction fails for any
// reason, it falls back to writing each problem in its own transaction so a
// single bad problem in the batch doesn't take the other batchSize-1 down
// with it. Never returns an error -- all outcomes land in summary.
func (i *Ingester) ingestBatch(ctx context.Context, batch []preparedProblem, dryRun bool, summary *Summary) {
	if dryRun {
		for _, pp := range batch {
			summary.Ingested++
			summary.ConfigsWritten += len(pp.configs)
			summary.MovesWritten += len(pp.moves)
		}
		return
	}

	if newHolds, err := i.ingestBatchFast(ctx, batch); err == nil {
		summary.Ingested += len(batch)
		summary.NewHoldsFound += newHolds
		for _, pp := range batch {
			summary.ConfigsWritten += len(pp.configs)
			summary.MovesWritten += len(pp.moves)
		}
		return
	}

	for _, pp := range batch {
		configsWritten, movesWritten, newHolds, err := i.ingestProblemSequential(ctx, pp)
		if err != nil {
			summary.Errors = append(summary.Errors, ProblemError{ExternalID: pp.problem.ID, Err: err})
			continue
		}

		summary.Ingested++
		summary.ConfigsWritten += configsWritten
		summary.MovesWritten += movesWritten
		summary.NewHoldsFound += newHolds
	}
}

// ingestBatchFast writes an entire batch in one transaction using two
// pipelined round trips: one to upsert every problem and collect their ids,
// one to write every problem's configs, the batch's deduped hold discovery,
// and every problem's moves -- holds are queued before any move so the FK is
// satisfied despite the whole batch being pipelined out of order.
func (i *Ingester) ingestBatchFast(ctx context.Context, batch []preparedProblem) (newHolds int, err error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids, err := batchUpsertProblems(ctx, tx, batch)
	if err != nil {
		return 0, fmt.Errorf("batch upsert problems: %w", err)
	}

	newHolds, err = batchWriteChildren(ctx, tx, batch, ids)
	if err != nil {
		return 0, fmt.Errorf("batch write children: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return newHolds, nil
}

const upsertProblemSQL = `
	INSERT INTO problems (
		external_id, holdsetup, name, setter, setby_id, climb_method,
		holdsets, coordinates, beta_videos, moves_raw, date_inserted, date_updated
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	ON CONFLICT (holdsetup, external_id) DO UPDATE SET
		name          = EXCLUDED.name,
		setter        = EXCLUDED.setter,
		setby_id      = EXCLUDED.setby_id,
		climb_method  = EXCLUDED.climb_method,
		holdsets      = EXCLUDED.holdsets,
		coordinates   = EXCLUDED.coordinates,
		beta_videos   = EXCLUDED.beta_videos,
		moves_raw     = EXCLUDED.moves_raw,
		date_inserted = EXCLUDED.date_inserted,
		date_updated  = EXCLUDED.date_updated
	RETURNING id
`

const insertConfigSQL = `
	INSERT INTO problem_configurations (
		problem_id, holdsetup, api_id, angle, primary_angle, grade, user_grade,
		user_rating, is_benchmark, is_competition_problem, is_primary, repeats,
		comment, date_updated
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
`

const insertMoveSQL = `
	INSERT INTO problem_moves (problem_id, holdsetup, seq, move_type, grid_ref)
	VALUES ($1,$2,$3,$4,$5)
`

// batchUpsertProblems queues one upsert-problem statement per problem in the
// batch and sends them all in a single pipelined round trip, returning their
// ids in the same order as batch.
func batchUpsertProblems(ctx context.Context, tx pgx.Tx, batch []preparedProblem) (ids []int64, err error) {
	b := &pgx.Batch{}
	for _, pp := range batch {
		p := pp.problem
		b.Queue(upsertProblemSQL,
			p.ID, p.Holdsetup, p.Name, p.Setter, p.SetbyID, p.ClimbMethod,
			p.Holdsets, p.Coordinates, p.BetaVideos, p.Moves, sqlTime(p.DateInserted), sqlTime(p.DateUpdated),
		)
	}

	br := tx.SendBatch(ctx, b)
	defer func() {
		if cerr := br.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close upsert-problems batch: %w", cerr)
		}
	}()

	ids = make([]int64, len(batch))
	for idx := range batch {
		var id int64
		if err := br.QueryRow().Scan(&id); err != nil {
			return nil, fmt.Errorf("upsert problem %d: %w", batch[idx].problem.ID, err)
		}
		ids[idx] = id
	}

	return ids, nil
}

// batchWriteChildren queues every problem's config replace, the batch's
// deduped hold discovery, and every problem's move replace into one
// pipelined round trip, in that order -- holds are queued strictly before
// any move so the FK is satisfied even though every statement is sent
// without waiting for individual results.
func batchWriteChildren(ctx context.Context, tx pgx.Tx, batch []preparedProblem, ids []int64) (newHolds int, err error) {
	b := &pgx.Batch{}

	configStmts := 0
	for idx, pp := range batch {
		problemID := ids[idx]

		b.Queue(`DELETE FROM problem_configurations WHERE problem_id = $1`, problemID)
		configStmts++

		for _, c := range pp.configs {
			b.Queue(insertConfigSQL,
				problemID, pp.problem.Holdsetup, c.cfg.APIID, c.angle, c.primaryAngle, c.cfg.Grade, c.cfg.UserGrade,
				c.cfg.UserRating, c.cfg.IsBenchmark, c.cfg.IsCompetitionProblem, c.cfg.IsPrimary, c.cfg.Repeats,
				c.cfg.Comment, sqlTime(c.cfg.DateUpdated),
			)
			configStmts++
		}
	}

	type holdKey struct {
		holdsetup int
		gridRef   string
	}

	seenHolds := make(map[holdKey]struct{})
	holdKeys := make([]holdKey, 0, len(batch)*4)
	for _, pp := range batch {
		for _, m := range pp.moves {
			k := holdKey{pp.problem.Holdsetup, m.GridRef}
			if _, ok := seenHolds[k]; ok {
				continue
			}
			seenHolds[k] = struct{}{}
			holdKeys = append(holdKeys, k)
		}
	}
	for _, k := range holdKeys {
		b.Queue(`INSERT INTO holds (holdsetup, grid_ref) VALUES ($1, $2) ON CONFLICT (holdsetup, grid_ref) DO NOTHING`,
			k.holdsetup, k.gridRef)
	}

	moveStmts := 0
	for idx, pp := range batch {
		problemID := ids[idx]

		b.Queue(`DELETE FROM problem_moves WHERE problem_id = $1`, problemID)
		moveStmts++

		for _, m := range pp.moves {
			b.Queue(insertMoveSQL, problemID, pp.problem.Holdsetup, m.Seq, m.Type, m.GridRef)
			moveStmts++
		}
	}

	br := tx.SendBatch(ctx, b)
	defer func() {
		if cerr := br.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close write-children batch: %w", cerr)
		}
	}()

	for j := 0; j < configStmts; j++ {
		if _, err := br.Exec(); err != nil {
			return 0, fmt.Errorf("config write (stmt %d/%d): %w", j+1, configStmts, err)
		}
	}

	for j := 0; j < len(holdKeys); j++ {
		tag, err := br.Exec()
		if err != nil {
			return newHolds, fmt.Errorf("hold write (stmt %d/%d): %w", j+1, len(holdKeys), err)
		}
		newHolds += int(tag.RowsAffected())
	}

	for j := 0; j < moveStmts; j++ {
		if _, err := br.Exec(); err != nil {
			return newHolds, fmt.Errorf("move write (stmt %d/%d): %w", j+1, moveStmts, err)
		}
	}

	return newHolds, nil
}

// ingestProblemSequential writes one already-parsed problem in its own
// transaction. Used as the fallback when ingestBatchFast fails, so the
// failure is isolated to just the problem(s) that actually caused it instead
// of the whole batch.
func (i *Ingester) ingestProblemSequential(ctx context.Context, pp preparedProblem) (configsWritten, movesWritten, newHolds int, err error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	problemID, err := upsertProblem(ctx, tx, pp.problem)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("upsert problem: %w", err)
	}

	if err := replaceConfigurations(ctx, tx, problemID, pp.problem.Holdsetup, pp.configs); err != nil {
		return 0, 0, 0, fmt.Errorf("replace configurations: %w", err)
	}

	newHolds, err = discoverHolds(ctx, tx, pp.problem.Holdsetup, pp.moves)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("discover holds: %w", err)
	}

	if err := replaceMoves(ctx, tx, problemID, pp.problem.Holdsetup, pp.moves); err != nil {
		return 0, 0, 0, fmt.Errorf("replace moves: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("commit tx: %w", err)
	}

	return len(pp.configs), len(pp.moves), newHolds, nil
}

func upsertProblem(ctx context.Context, tx pgx.Tx, p Problem) (int64, error) {
	var id int64

	err := tx.QueryRow(ctx, upsertProblemSQL,
		p.ID, p.Holdsetup, p.Name, p.Setter, p.SetbyID, p.ClimbMethod,
		p.Holdsets, p.Coordinates, p.BetaVideos, p.Moves, sqlTime(p.DateInserted), sqlTime(p.DateUpdated),
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func replaceConfigurations(ctx context.Context, tx pgx.Tx, problemID int64, holdsetup int, configs []parsedConfig) error {
	if _, err := tx.Exec(ctx, `DELETE FROM problem_configurations WHERE problem_id = $1`, problemID); err != nil {
		return err
	}

	for _, c := range configs {
		_, err := tx.Exec(ctx, insertConfigSQL,
			problemID, holdsetup, c.cfg.APIID, c.angle, c.primaryAngle, c.cfg.Grade, c.cfg.UserGrade,
			c.cfg.UserRating, c.cfg.IsBenchmark, c.cfg.IsCompetitionProblem, c.cfg.IsPrimary, c.cfg.Repeats,
			c.cfg.Comment, sqlTime(c.cfg.DateUpdated),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// discoverHolds inserts every distinct (holdsetup, grid_ref) pair touched by
// moves ahead of the problem_moves FK insert, returning how many were newly
// discovered (vs. already known from a prior ingestion).
func discoverHolds(ctx context.Context, tx pgx.Tx, holdsetup int, moves []Move) (int, error) {
	seen := make(map[string]struct{}, len(moves))
	newHolds := 0

	for _, m := range moves {
		if _, ok := seen[m.GridRef]; ok {
			continue
		}
		seen[m.GridRef] = struct{}{}

		tag, err := tx.Exec(ctx, `
			INSERT INTO holds (holdsetup, grid_ref) VALUES ($1, $2)
			ON CONFLICT (holdsetup, grid_ref) DO NOTHING
		`, holdsetup, m.GridRef)
		if err != nil {
			return newHolds, err
		}

		newHolds += int(tag.RowsAffected())
	}

	return newHolds, nil
}

func replaceMoves(ctx context.Context, tx pgx.Tx, problemID int64, holdsetup int, moves []Move) error {
	if _, err := tx.Exec(ctx, `DELETE FROM problem_moves WHERE problem_id = $1`, problemID); err != nil {
		return err
	}

	for _, m := range moves {
		if _, err := tx.Exec(ctx, insertMoveSQL, problemID, holdsetup, m.Seq, m.Type, m.GridRef); err != nil {
			return err
		}
	}

	return nil
}

// decodeExportStream walks the top-level {holdsetup, count, problems: [...]}
// export object token-by-token so the ~90-260K-element problems array never
// needs to be materialized in memory as a single slice. onHoldsetup is
// invoked as soon as the holdsetup key is decoded (before any problem is
// read, since it precedes "problems" in the source shape) so a caller can
// bail out fatally on a file/DB mismatch before writing anything.
func decodeExportStream(r io.Reader, onHoldsetup func(int) error, onProblem func(Problem) error) error {
	dec := json.NewDecoder(r)

	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("catalog: read opening token: %w", err)
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("catalog: read key token: %w", err)
		}

		key, _ := keyTok.(string)

		switch key {
		case "holdsetup":
			var holdsetup int
			if err := dec.Decode(&holdsetup); err != nil {
				return fmt.Errorf("catalog: decode holdsetup: %w", err)
			}
			if err := onHoldsetup(holdsetup); err != nil {
				return err
			}
		case "problems":
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("catalog: read problems array start: %w", err)
			}
			for dec.More() {
				var p Problem
				if err := dec.Decode(&p); err != nil {
					return fmt.Errorf("catalog: decode problem: %w", err)
				}
				if err := onProblem(p); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("catalog: read problems array end: %w", err)
			}
		default:
			var discard json.RawMessage
			if err := dec.Decode(&discard); err != nil {
				return fmt.Errorf("catalog: skip field %q: %w", key, err)
			}
		}
	}

	return nil
}
