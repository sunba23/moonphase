package server

import (
	"net/http"

	"github.com/sunba23/moonphase/templates/layout"
)

// versionContext puts the running commit SHA (or "dev") on every request's
// context so the shared page footer can render it without threading a parameter
// through every handler and page component.
func versionContext(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(layout.WithVersion(r.Context(), version)))
		})
	}
}
