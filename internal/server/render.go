package server

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/templates/layout"
)

// renderAppPage renders an authenticated full page: the shared header (built
// from the profile OnboardingGate stashed in context) above the given content.
// If no profile is in context — which should not happen inside the gated
// tier — it falls back to a header-less render.
func renderAppPage(w http.ResponseWriter, r *http.Request, title string, content templ.Component, status int) {
	prof, ok := profileFromContext(r.Context())
	if !ok {
		renderPage(w, r, content, status)
		return
	}

	boardName, ok := catalog.BoardName(prof.Holdsetup)
	if !ok {
		boardName = "Unknown board"
	}

	nav := layout.NavModel{
		BoardName: boardName,
		Angle:     prof.Angle,
		MaxGrade:  prof.MaxGrade,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = layout.AppPage(title, nav, content).Render(r.Context(), w)
}
