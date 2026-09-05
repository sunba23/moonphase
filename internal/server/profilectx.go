package server

import (
	"context"

	"github.com/sunba23/moonphase/internal/profile"
)

// profileCtxKey is the unexported key under which OnboardingGate stashes the
// profile it already loaded, so renderAppPage can build the shared header
// without a second query. Mirrors auth.WithUserID / auth.UserIDFromContext.
type profileCtxKey struct{}

// withProfile returns a context carrying the gate-loaded profile.
func withProfile(ctx context.Context, p *profile.Profile) context.Context {
	return context.WithValue(ctx, profileCtxKey{}, p)
}

// profileFromContext returns the profile stashed by OnboardingGate, if any.
// Only the OnboardingGate tier populates it.
func profileFromContext(ctx context.Context) (*profile.Profile, bool) {
	p, ok := ctx.Value(profileCtxKey{}).(*profile.Profile)
	return p, ok
}
