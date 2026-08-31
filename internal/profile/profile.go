// Package profile stores each climber's onboarding-declared max grade and
// MoonBoard set/angle.
package profile

// Profile is one climber's onboarding-declared training context.
type Profile struct {
	UserID    string
	MaxGrade  string
	Holdsetup int16
	Angle     int16
}
