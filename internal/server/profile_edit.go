package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/templates/pages"
)

// profilePages wires the profile-edit form to the catalog read-queries and
// profile.Store.
type profilePages struct {
	pool   *pgxpool.Pool
	store  *profile.Store
	logger *zerolog.Logger
}

func newProfilePages(pool *pgxpool.Pool, store *profile.Store, logger *zerolog.Logger) *profilePages {
	return &profilePages{pool: pool, store: store, logger: logger}
}

func (p *profilePages) loadModel(ctx context.Context, current profile.Profile) (pages.ProfileModel, error) {
	grades, err := catalog.DistinctGrades(ctx, p.pool)
	if err != nil {
		return pages.ProfileModel{}, err
	}

	angles, err := catalog.DistinctAngles(ctx, p.pool)
	if err != nil {
		return pages.ProfileModel{}, err
	}

	return pages.ProfileModel{
		Grades: grades,
		Boards: catalog.SupportedBoards(),
		Angles: angles,

		CurrentGrade:     current.MaxGrade,
		CurrentHoldsetup: current.Holdsetup,
		CurrentAngle:     current.Angle,
	}, nil
}

func (p *profilePages) handlePage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	current, err := p.store.Get(r.Context(), userID)
	if err != nil {
		p.logger.Error().Err(err).Msg("profile: load profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	model, err := p.loadModel(r.Context(), *current)
	if err != nil {
		p.logger.Error().Err(err).Msg("profile: load catalog options failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, pages.ProfilePage(model), http.StatusOK)
}

func (p *profilePages) handleSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	current, err := p.store.Get(r.Context(), userID)
	if err != nil {
		p.logger.Error().Err(err).Msg("profile: load profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	model, err := p.loadModel(r.Context(), *current)
	if err != nil {
		p.logger.Error().Err(err).Msg("profile: load catalog options failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	maxGrade := r.FormValue("max_grade")
	if !gradeValid(model.Grades, maxGrade) {
		model.Error = "Invalid max grade"
		renderPage(w, r, pages.ProfilePage(model), http.StatusUnprocessableEntity)
		return
	}

	holdsetup, err := strconv.ParseInt(r.FormValue("holdsetup"), 10, 16)
	if err != nil || !boardValid(model.Boards, int16(holdsetup)) {
		model.Error = "Invalid board"
		renderPage(w, r, pages.ProfilePage(model), http.StatusUnprocessableEntity)
		return
	}

	angle, err := strconv.ParseInt(r.FormValue("angle"), 10, 16)
	if err != nil || !angleValid(model.Angles, int16(angle)) {
		model.Error = "Invalid angle"
		renderPage(w, r, pages.ProfilePage(model), http.StatusUnprocessableEntity)
		return
	}

	if err := p.store.Upsert(r.Context(), profile.Profile{
		UserID:    userID,
		MaxGrade:  maxGrade,
		Holdsetup: int16(holdsetup),
		Angle:     int16(angle),
	}); err != nil {
		p.logger.Error().Err(err).Msg("profile: upsert profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}
