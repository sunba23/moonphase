package pages

import "github.com/sunba23/moonphase/internal/catalog"

// AuthFormModel carries per-request state for the signup/signin forms.
type AuthFormModel struct {
	Error string
}

// HubModel is the climber's training context shown on the hub.
type HubModel struct {
	BoardName string
	Angle     int16
	MaxGrade  string
}

// SessionCardModel backs the swappable #session-card fragment: one problem
// plus the result form that advances the loop.
type SessionCardModel struct {
	SessionID string
	Seq       int
	Problem   catalog.ProblemView
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
