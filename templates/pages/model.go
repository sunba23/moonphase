package pages

import "github.com/sunba23/moonphase/internal/catalog"

// AuthFormModel carries per-request state for the signup/signin forms.
type AuthFormModel struct {
	Error string
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
