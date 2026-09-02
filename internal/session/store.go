package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the DB-backed read/write access to the sessions and
// session_problems tables. Mirrors internal/profile.Store.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const sessionColumns = `id, user_id, holdsetup, angle, max_grade, status, started_at`

func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.Holdsetup, &s.Angle, &s.MaxGrade, &s.Status, &s.StartedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// ActiveForUser returns the user's open session, or ErrNoActiveSession.
func (s *Store) ActiveForUser(ctx context.Context, userID string) (*Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM sessions
		WHERE user_id = $1 AND status = 'active'
	`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoActiveSession
		}
		return nil, fmt.Errorf("session: active for user: %w", err)
	}
	return sess, nil
}

// Get returns a session by id, or ErrNotFound. Not user-scoped — ownership is
// the handler's job (a non-owner must get an identical 404, so the handler
// compares UserID itself).
func (s *Store) Get(ctx context.Context, sessionID string) (*Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM sessions
		WHERE id = $1
	`, sessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("session: get: %w", err)
	}
	return sess, nil
}

// StartSession inserts the sessions row and its seq-0 session_problems row in
// one transaction. A unique violation on sessions_one_active_per_user (a
// concurrent second start) surfaces as ErrActiveExists.
func (s *Store) StartSession(ctx context.Context, in Session, first SessionProblem) (*Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("session: begin start-session tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out := Session{
		UserID:    in.UserID,
		Holdsetup: in.Holdsetup,
		Angle:     in.Angle,
		MaxGrade:  in.MaxGrade,
		Status:    StatusActive,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (user_id, holdsetup, angle, max_grade, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, started_at
	`, in.UserID, in.Holdsetup, in.Angle, in.MaxGrade).Scan(&out.ID, &out.StartedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrActiveExists
		}
		return nil, fmt.Errorf("session: insert session: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO session_problems (session_id, seq, problem_id, problem_configuration_id)
		VALUES ($1, $2, $3, $4)
	`, out.ID, first.Seq, first.ProblemID, first.ConfigurationID); err != nil {
		return nil, fmt.Errorf("session: insert first problem: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("session: commit start-session tx: %w", err)
	}
	return &out, nil
}

// ShownProblem is one row of a session's ordered problem list, joined to its
// grade and dominant hold type. RPE / Completion are nil for the current
// unrated problem.
type ShownProblem struct {
	Seq             int
	ProblemID       int64
	ConfigurationID int64
	Grade           string
	Dominant        string
	RPE             *int16
	Completion      *string
}

// ShownProblems returns every problem a session has shown, seq-ordered, with
// grade and dominant hold type. The last entry is the current problem.
func (s *Store) ShownProblems(ctx context.Context, sessionID string) ([]ShownProblem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sp.seq, sp.problem_id, sp.problem_configuration_id,
		       pc.grade, COALESCE(pht.dominant, ''), sp.rpe, sp.completion
		FROM session_problems sp
		JOIN problem_configurations pc ON pc.id = sp.problem_configuration_id
		LEFT JOIN problem_hold_types pht ON pht.problem_id = sp.problem_id
		WHERE sp.session_id = $1
		ORDER BY sp.seq
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session: query shown problems: %w", err)
	}
	defer rows.Close()

	var out []ShownProblem
	for rows.Next() {
		var p ShownProblem
		if err := rows.Scan(&p.Seq, &p.ProblemID, &p.ConfigurationID, &p.Grade, &p.Dominant, &p.RPE, &p.Completion); err != nil {
			return nil, fmt.Errorf("session: scan shown problem: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session: iterate shown problems: %w", err)
	}

	return out, nil
}

// LatestProblem returns the highest-seq session_problems row, or ErrNotFound.
func (s *Store) LatestProblem(ctx context.Context, sessionID string) (*SessionProblem, error) {
	var sp SessionProblem
	err := s.pool.QueryRow(ctx, `
		SELECT seq, problem_id, problem_configuration_id
		FROM session_problems
		WHERE session_id = $1
		ORDER BY seq DESC
		LIMIT 1
	`, sessionID).Scan(&sp.Seq, &sp.ProblemID, &sp.ConfigurationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("session: latest problem: %w", err)
	}
	return &sp, nil
}

// AdvanceSession writes the current problem's result and inserts the next
// problem in one transaction. The guarded UPDATE is the idempotency key: it
// only touches the highest-seq, still-unrated row, so a duplicate or
// out-of-order submit affects 0 rows and returns ErrStaleResult with no
// second insert. Takes plain scalars + SessionProblem only — no recommender
// import.
func (s *Store) AdvanceSession(ctx context.Context, sessionID string, curSeq int, rpe int16, completion string, next SessionProblem) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("session: begin advance tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE session_problems
		SET rpe = $1, completion = $2, climbed_at = now()
		WHERE session_id = $3 AND seq = $4 AND rpe IS NULL
		  AND seq = (SELECT max(seq) FROM session_problems WHERE session_id = $3)
	`, rpe, completion, sessionID, curSeq)
	if err != nil {
		return fmt.Errorf("session: update result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleResult
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO session_problems (session_id, seq, problem_id, problem_configuration_id)
		VALUES ($1, $2, $3, $4)
	`, sessionID, curSeq+1, next.ProblemID, next.ConfigurationID); err != nil {
		return fmt.Errorf("session: insert next problem: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("session: commit advance tx: %w", err)
	}
	return nil
}

// EndSession flips an active session to 'ended' and stamps ended_at. A
// non-owner, a missing id, and an already-ended session are indistinguishable
// — all return ErrNotFound.
func (s *Store) EndSession(ctx context.Context, sessionID, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET status = 'ended', ended_at = now()
		WHERE id = $1 AND user_id = $2 AND status = 'active'
	`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("session: end session: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// FirstProblem returns the seq-0 session_problems row, or ErrNotFound.
func (s *Store) FirstProblem(ctx context.Context, sessionID string) (*SessionProblem, error) {
	var sp SessionProblem
	err := s.pool.QueryRow(ctx, `
		SELECT seq, problem_id, problem_configuration_id
		FROM session_problems
		WHERE session_id = $1 AND seq = 0
	`, sessionID).Scan(&sp.Seq, &sp.ProblemID, &sp.ConfigurationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("session: first problem: %w", err)
	}
	return &sp, nil
}
