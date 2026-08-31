package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/profile"
)

// ProfileChecker is the minimal profile-lookup surface OnboardingGate needs,
// letting tests inject a fake without a live DB. profile.Store satisfies
// this structurally.
type ProfileChecker interface {
	Get(ctx context.Context, userID string) (*profile.Profile, error)
}

// OnboardingGate redirects an authenticated-but-not-yet-onboarded user to
// /onboarding. It must sit only in front of routes that require a complete
// profile — never in front of /onboarding itself, or a signed-in user could
// never reach the form that would complete their profile.
func OnboardingGate(pc ProfileChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
				return
			}

			if _, err := pc.Get(r.Context(), userID); err != nil {
				if errors.Is(err, profile.ErrNotFound) {
					http.Redirect(w, r, "/onboarding", http.StatusFound)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
