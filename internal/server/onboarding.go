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

// onboardingPages wires the onboarding form to the catalog read-queries and
// profile.Store.
type onboardingPages struct {
	pool   *pgxpool.Pool
	store  *profile.Store
	logger *zerolog.Logger
}

func newOnboardingPages(pool *pgxpool.Pool, store *profile.Store, logger *zerolog.Logger) *onboardingPages {
	return &onboardingPages{pool: pool, store: store, logger: logger}
}

func (o *onboardingPages) loadModel(ctx context.Context) (pages.OnboardingModel, error) {
	grades, err := catalog.DistinctGrades(ctx, o.pool)
	if err != nil {
		return pages.OnboardingModel{}, err
	}

	angles, err := catalog.DistinctAngles(ctx, o.pool)
	if err != nil {
		return pages.OnboardingModel{}, err
	}

	// Only app-ready boards (image shipped + fully hold-tagged) — an
	// out-of-list holdsetup is rejected by boardValid's existing 422 path.
	return pages.OnboardingModel{Grades: grades, Boards: catalog.SupportedBoards(), Angles: angles}, nil
}

func (o *onboardingPages) handlePage(w http.ResponseWriter, r *http.Request) {
	model, err := o.loadModel(r.Context())
	if err != nil {
		o.logger.Error().Err(err).Msg("onboarding: load catalog options failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, pages.OnboardingPage(model), http.StatusOK)
}

func (o *onboardingPages) handleSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	model, err := o.loadModel(r.Context())
	if err != nil {
		o.logger.Error().Err(err).Msg("onboarding: load catalog options failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	maxGrade := r.FormValue("max_grade")
	if !gradeValid(model.Grades, maxGrade) {
		model.Error = "Invalid max grade"
		renderPage(w, r, pages.OnboardingPage(model), http.StatusUnprocessableEntity)
		return
	}

	holdsetup, err := strconv.ParseInt(r.FormValue("holdsetup"), 10, 16)
	if err != nil || !boardValid(model.Boards, int16(holdsetup)) {
		model.Error = "Invalid board"
		renderPage(w, r, pages.OnboardingPage(model), http.StatusUnprocessableEntity)
		return
	}

	angle, err := strconv.ParseInt(r.FormValue("angle"), 10, 16)
	if err != nil || !angleValid(model.Angles, int16(angle)) {
		model.Error = "Invalid angle"
		renderPage(w, r, pages.OnboardingPage(model), http.StatusUnprocessableEntity)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	if err := o.store.Upsert(r.Context(), profile.Profile{
		UserID:    userID,
		MaxGrade:  maxGrade,
		Holdsetup: int16(holdsetup),
		Angle:     int16(angle),
	}); err != nil {
		o.logger.Error().Err(err).Msg("onboarding: upsert profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}
