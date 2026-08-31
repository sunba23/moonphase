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

// StatusActive is the only session status this slice writes. S-04 extends the
// value set (ended, etc.).
const StatusActive = "active"

var (
	// ErrNoActiveSession is returned by ActiveForUser when the user has no
	// open session.
	ErrNoActiveSession = errors.New("session: no active session")
	// ErrNotFound is returned by Get / FirstProblem when the row is absent.
	ErrNotFound = errors.New("session: not found")
	// ErrActiveExists is returned by StartSession when the one-active-session
	// partial unique index rejects a concurrent second start.
	ErrActiveExists = errors.New("session: an active session already exists")
)
