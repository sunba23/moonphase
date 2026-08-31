package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by Get when the user has no profile row yet — the
// onboarding gate branches on this to redirect to /onboarding.
var ErrNotFound = errors.New("profile: not found")

// Store is the DB-backed read/write access to the profiles table.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Get returns the user's profile, or ErrNotFound if they haven't onboarded.
func (s *Store) Get(ctx context.Context, userID string) (*Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx, `
		SELECT id, max_grade, holdsetup, angle
		FROM profiles
		WHERE id = $1
	`, userID).Scan(&p.UserID, &p.MaxGrade, &p.Holdsetup, &p.Angle)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("profile: get: %w", err)
	}

	return &p, nil
}

// Upsert creates or updates the user's profile row.
func (s *Store) Upsert(ctx context.Context, p Profile) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles (id, max_grade, holdsetup, angle)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET max_grade = EXCLUDED.max_grade,
		    holdsetup = EXCLUDED.holdsetup,
		    angle = EXCLUDED.angle,
		    updated_at = now()
	`, p.UserID, p.MaxGrade, p.Holdsetup, p.Angle)
	if err != nil {
		return fmt.Errorf("profile: upsert: %w", err)
	}

	return nil
}
