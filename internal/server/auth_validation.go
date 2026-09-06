package server

import "strings"

const (
	// minPasswordLen is our own floor, checked before we call GoTrue so a
	// too-short password fails fast with clear wording. GoTrue owns any
	// stricter policy (complexity, breach checks); its message is forwarded.
	minPasswordLen = 8
	// maxPasswordBytes is the bcrypt input ceiling GoTrue rejects above.
	maxPasswordBytes = 72
)

// validateSignupCredentials runs the cheap local checks before the GoTrue call.
// On failure it returns a short, user-facing message and ok=false.
func validateSignupCredentials(email, password string) (msg string, ok bool) {
	if strings.TrimSpace(email) == "" {
		return "Enter your email address.", false
	}
	if len(password) < minPasswordLen {
		return "Use a password with at least 8 characters.", false
	}
	if len(password) > maxPasswordBytes {
		return "Use a password with 72 characters or fewer.", false
	}
	return "", true
}
