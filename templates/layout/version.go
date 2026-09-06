package layout

import "context"

// ctxKey is the private key type for the version value carried on the request
// context. The footer template reads it; nothing outside this package needs it.
type ctxKey struct{}

var versionKey ctxKey

// WithVersion returns a copy of ctx that carries full — the complete commit SHA
// the binary was built from, or "dev" for a local run.
func WithVersion(ctx context.Context, full string) context.Context {
	return context.WithValue(ctx, versionKey, full)
}

// versionFromContext returns the version stored by WithVersion, or "dev" when
// nothing is stored (an unset value must never render as an empty footer).
func versionFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(versionKey).(string); ok && v != "" {
		return v
	}
	return "dev"
}

// shortVersion is what the footer shows: "dev" stays "dev"; a real SHA is cut
// to its first 7 characters.
func shortVersion(full string) string {
	if full == "dev" {
		return "dev"
	}
	if len(full) > 7 {
		return full[:7]
	}
	return full
}

// commitURL is the GitHub commit page for full, or "" when there is no commit
// to link ("dev", or a too-short value).
func commitURL(full string) string {
	if full == "dev" || len(full) < 7 {
		return ""
	}
	return "https://github.com/sunba23/moonphase/commit/" + full
}
