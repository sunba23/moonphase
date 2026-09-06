package server

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/templates/layout"
)

// navFromContext builds the shared-header model from the profile OnboardingGate
// stashed in context. The second result is false when no profile is present —
// which should not happen inside the gated tier.
func navFromContext(r *http.Request) (layout.NavModel, bool) {
	prof, ok := profileFromContext(r.Context())
	if !ok {
		return layout.NavModel{}, false
	}

	boardName, ok := catalog.BoardName(prof.Holdsetup)
	if !ok {
		boardName = "Unknown board"
	}

	return layout.NavModel{
		BoardName: boardName,
		Angle:     prof.Angle,
		MaxGrade:  prof.MaxGrade,
	}, true
}

// writeAppPage renders the authenticated full-page shell with the given nav.
func writeAppPage(w http.ResponseWriter, r *http.Request, title string, nav layout.NavModel, content templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	// r.Context() IS propagated into the render — the footer template reads the
	// version off it. contextcheck trips over templ's generated closures once
	// any component in the shell consumes the context.
	_ = layout.AppPage(title, nav, content).Render(r.Context(), w) //nolint:contextcheck // false positive: context is propagated, see comment above
}

// renderAppPage renders an authenticated full page: the shared header (built
// from the profile OnboardingGate stashed in context) above the given content.
// If no profile is in context — which should not happen inside the gated
// tier — it falls back to a header-less render.
func renderAppPage(w http.ResponseWriter, r *http.Request, title string, content templ.Component, status int) {
	nav, ok := navFromContext(r)
	if !ok {
		renderPage(w, r, content, status)
		return
	}
	writeAppPage(w, r, title, nav, content, status)
}
