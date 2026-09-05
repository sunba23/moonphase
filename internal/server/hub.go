package server

import (
	"net/http"

	"github.com/sunba23/moonphase/templates/pages"
)

// handleHub renders the post-login hub: the Main Session action plus the
// secondary links. The training context lives in the shared header
// (renderAppPage builds it from the gate-loaded profile), so the hub body
// needs no model.
func handleHub(w http.ResponseWriter, r *http.Request) {
	renderAppPage(w, r, "MoonPhase", pages.HubContent(), http.StatusOK)
}
