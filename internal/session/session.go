// Package session stores the Main Session container and the ordered list of
// problems it has shown. This slice (S-03) records only the first pick
// (seq 0); S-04 adds the adaptive loop and per-problem results.
package session

import (
	"errors"
	"time"
)

// Session is one Main Session. Holdsetup, Angle, and MaxGrade are snapshotted
// from the user's profile at creation so a mid-session profile edit can't move
// the recommender's inputs.
type Session struct {
	ID        string
	UserID    string
	Holdsetup int16
	Angle     int16
	MaxGrade  string
	Status    string
	StartedAt time.Time
}

// SessionProblem is one row of the ordered list of problems a session has
// shown. Seq 0 is the first pick.
type SessionProblem struct {
	Seq             int
	ProblemID       int64
	ConfigurationID int64
}

// Session status values. StatusActive is set at creation; StatusEnded is set
// by the explicit End-session action (FR-010). The partial unique index frees
// the active slot as soon as status moves off 'active'.
const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

// Completion status values for a per-problem result (FR-008).
const (
	CompletionSent   = "sent"
	CompletionFailed = "failed"
	CompletionBailed = "bailed"
)

// ValidCompletion reports whether s is one of the three completion statuses.
func ValidCompletion(s string) bool {
	switch s {
	case CompletionSent, CompletionFailed, CompletionBailed:
		return true
	default:
		return false
	}
}

var (
	// ErrNoActiveSession is returned by ActiveForUser when the user has no
	// open session.
	ErrNoActiveSession = errors.New("session: no active session")
	// ErrNotFound is returned by Get / FirstProblem when the row is absent.
	ErrNotFound = errors.New("session: not found")
	// ErrActiveExists is returned by StartSession when the one-active-session
	// partial unique index rejects a concurrent second start.
	ErrActiveExists = errors.New("session: an active session already exists")
	// ErrStaleResult is returned by AdvanceSession when the guarded UPDATE
	// affects no row — a duplicate or out-of-order result submit.
	ErrStaleResult = errors.New("session: stale result")
	// ErrSessionNotActive is returned when an operation needs an active
	// session but the session is already ended.
	ErrSessionNotActive = errors.New("session: not active")
)
