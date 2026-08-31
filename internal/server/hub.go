package server

import (
	"net/http"

	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/templates/pages"
)

// hubPages renders the post-login hub: training context + Main Session button.
type hubPages struct {
	store  *profile.Store
	logger *zerolog.Logger
}

func newHubPages(store *profile.Store, logger *zerolog.Logger) *hubPages {
	return &hubPages{store: store, logger: logger}
}

func (h *hubPages) handlePage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	current, err := h.store.Get(r.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("hub: load profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	boardName, ok := catalog.BoardName(current.Holdsetup)
	if !ok {
		boardName = "Unknown board"
	}

	renderPage(w, r, pages.HubPage(pages.HubModel{
		BoardName: boardName,
		Angle:     current.Angle,
		MaxGrade:  current.MaxGrade,
	}), http.StatusOK)
}
