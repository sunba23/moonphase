package server

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/internal/recommender"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/templates/pages"
)

// sessionPages wires the Main Session start/resume + view handlers to the
// session store, recommender, and catalog problem-detail query.
type sessionPages struct {
	pool     *pgxpool.Pool
	profiles *profile.Store
	sessions *session.Store
	rec      *recommender.Recommender
	logger   *zerolog.Logger
}

func newSessionPages(pool *pgxpool.Pool, profiles *profile.Store, sessions *session.Store, rec *recommender.Recommender, logger *zerolog.Logger) *sessionPages {
	return &sessionPages{pool: pool, profiles: profiles, sessions: sessions, rec: rec, logger: logger}
}

// handleStart (POST /session) starts a Main Session, or resumes the caller's
// open one. One active session per user is enforced by a partial unique
// index; a concurrent second tap loses the race and is redirected to the
// winner.
func (s *sessionPages) handleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	prof, err := s.profiles.Get(ctx, userID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load profile failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if _, ok := catalog.BoardImageYear(prof.Holdsetup); !ok {
		name, _ := catalog.BoardName(prof.Holdsetup)
		renderPage(w, r, pages.UnsupportedBoardPage(pages.UnsupportedBoardModel{BoardName: name}), http.StatusOK)
		return
	}

	active, err := s.sessions.ActiveForUser(ctx, userID)
	switch {
	case err == nil:
		s.redirectToSession(w, active.ID)
		return
	case !errors.Is(err, session.ErrNoActiveSession):
		s.logger.Error().Err(err).Msg("session: active-for-user check failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	pick, err := s.rec.FirstPick(ctx, prof.Holdsetup, prof.Angle)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: first pick failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	started, err := s.sessions.StartSession(ctx, session.Session{
		UserID:    userID,
		Holdsetup: prof.Holdsetup,
		Angle:     prof.Angle,
		MaxGrade:  prof.MaxGrade,
	}, session.SessionProblem{Seq: 0, ProblemID: pick.ProblemID, ConfigurationID: pick.ConfigurationID})
	if err != nil {
		if errors.Is(err, session.ErrActiveExists) {
			resumed, rerr := s.sessions.ActiveForUser(ctx, userID)
			if rerr != nil {
				s.logger.Error().Err(rerr).Msg("session: re-read after ErrActiveExists failed")
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			s.redirectToSession(w, resumed.ID)
			return
		}
		s.logger.Error().Err(err).Msg("session: start failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.redirectToSession(w, started.ID)
}

func (s *sessionPages) redirectToSession(w http.ResponseWriter, id string) {
	w.Header().Set("HX-Redirect", "/session/"+id)
	w.WriteHeader(http.StatusOK)
}

// handleView (GET /session/{sessionID}) renders a session's first problem.
// A missing session and a session owned by someone else return an identical
// 404 — the id's existence is never confirmed to a non-owner.
func (s *sessionPages) handleView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	sessionID := chi.URLParam(r, "sessionID")
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil || sess.UserID != userID {
		http.NotFound(w, r)
		return
	}

	first, err := s.sessions.FirstProblem(ctx, sessionID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load first problem failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view, err := catalog.ProblemDetail(ctx, s.pool, first.ConfigurationID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load problem detail failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, pages.SessionPage(pages.SessionModel{SessionID: sessionID, Problem: *view}), http.StatusOK)
}
