package pages

import "github.com/sunba23/moonphase/internal/catalog"

// AuthFormModel carries per-request state for the signup/signin forms. Email
// is echoed back after a failed submit so the user does not retype it; the
// password is never repopulated.
type AuthFormModel struct {
	Error string
	Email string
}

// SessionCardModel backs the swappable #session-card fragment: one problem
// plus the result form that advances the loop.
type SessionCardModel struct {
	SessionID string
	Seq       int
	Problem   catalog.ProblemView
	// Panel is the collapsed "Session balance" secondary surface. Nil before
	// anything is climbed (seq 0); the template renders nothing then.
	Panel *SessionPanel
}

// HoldTypeBar is one row of the session hold-type tally.
type HoldTypeBar struct {
	Type  string
	Count int
}

// SessionPanel backs the collapsed <details> panel below the board: the
// session-so-far hold-type tally plus a terse, non-prose rationale for the
// current pick. Built at render time from the session's shown problems — the
// recommender is not involved.
type SessionPanel struct {
	Bars     []HoldTypeBar // always the 5 allowed types, fixed order
	MaxCount int           // largest bar count, for width scaling; >= 1
	Climbed  int           // problems climbed so far this session
	GradeTag string        // "Easier" | "Holding" | "Harder"; "" when unknown
	HoldTag  string        // "Off crimp" | "Onto sloper" | "More jug"; "" when none
}

// SessionModel wraps the card for a full-page render.
type SessionModel struct {
	Card SessionCardModel
}

// UnsupportedBoardModel backs the "switch your board" page shown when a
// profile points at a board the app can't run a session on.
type UnsupportedBoardModel struct {
	BoardName string
}

// HistorySessionRow is one row of the past-sessions list (FR-013). StartedAt
// is pre-formatted in the handler so the template does no date work.
type HistorySessionRow struct {
	ID           string
	BoardName    string
	StartedAt    string
	Angle        int16
	ClimbedCount int
}

// HistoryListModel backs the read-only past-sessions list page.
type HistoryListModel struct {
	Sessions []HistorySessionRow
}

// HistoryProblemRow is one climbed problem in a past session's detail view
// (FR-014).
type HistoryProblemRow struct {
	Seq        int
	Name       string
	Grade      string
	Completion string
	RPE        int16
}

// HistoryDetailModel backs the read-only past-session detail page.
type HistoryDetailModel struct {
	SessionID string
	BoardName string
	StartedAt string
	Angle     int16
	Problems  []HistoryProblemRow
}

// HistoryProblemModel backs the read-only problem card reached from a past
// session's detail view — the live session card stripped to its read-only
// parts (no result form, no End button).
type HistoryProblemModel struct {
	SessionID  string
	Seq        int
	RPE        int16
	Completion string
	Problem    catalog.ProblemView
}

// OnboardingModel carries the catalog-derived dropdown options and
// per-request state for the onboarding form.
type OnboardingModel struct {
	Grades []string
	Boards []catalog.BoardEdition
	Angles []int16
	Error  string
}

// ProfileModel carries the catalog-derived dropdown options, the user's
// current values (for pre-selection), and per-request state for the
// profile-edit form.
type ProfileModel struct {
	Grades []string
	Boards []catalog.BoardEdition
	Angles []int16

	CurrentGrade     string
	CurrentHoldsetup int16
	CurrentAngle     int16

	Error string
}
