package auth

import "context"

type contextKey int

const userIDContextKey contextKey = iota

// WithUserID returns a new context carrying the resolved user id.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext returns the user id stored by the auth middleware, if any.
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}
