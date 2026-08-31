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
