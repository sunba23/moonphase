package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
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
		renderAppPage(w, r, "Switch your board", pages.UnsupportedBoardContent(pages.UnsupportedBoardModel{BoardName: name}), http.StatusOK)
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

	pick, poolSize, err := s.rec.FirstPick(ctx, prof.Holdsetup, prof.Angle)
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

	s.logFirstPick(started.ID, 0, pick, poolSize)
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

	shown, err := s.sessions.ShownProblems(ctx, sessionID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load shown problems failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(shown) == 0 {
		s.logger.Error().Str("session", sessionID).Msg("session: active session has no shown problems")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	current := shown[len(shown)-1]

	view, err := catalog.ProblemDetail(ctx, s.pool, current.ConfigurationID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load problem detail failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ladder, err := catalog.GradeLadder(ctx, s.pool, sess.Holdsetup, sess.Angle)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load grade ladder failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderSessionPage(w, r, sessionID, pages.SessionCard(pages.SessionCardModel{
		SessionID: sessionID, Seq: current.Seq, Problem: *view,
		Panel: buildSessionPanel(ladder, shown),
	}), http.StatusOK)
}

// renderSessionPage renders the live session full page: the shared header with
// an End-session control above the given content. Falls back to a header-less
// render when no profile is in context (should not happen in the gated tier).
func renderSessionPage(w http.ResponseWriter, r *http.Request, sessionID string, content templ.Component, status int) {
	nav, ok := navFromContext(r)
	if !ok {
		renderPage(w, r, content, status)
		return
	}
	nav.EndSessionID = sessionID
	writeAppPage(w, r, "Session", nav, content, status)
}

// handleResult (POST /session/{sessionID}/result) records the current
// problem's RPE + completion, scores the next pick, and swaps in the new card
// fragment. Server-side validation only; every 4xx writes nothing.
func (s *sessionPages) handleResult(w http.ResponseWriter, r *http.Request) {
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
	if sess.Status != session.StatusActive {
		http.Error(w, "session not active", http.StatusConflict)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	seq, err := strconv.Atoi(r.FormValue("seq"))
	if err != nil {
		http.Error(w, "bad seq", http.StatusUnprocessableEntity)
		return
	}
	rpe, err := strconv.Atoi(r.FormValue("rpe"))
	if err != nil || rpe < 1 || rpe > 10 {
		http.Error(w, "bad rpe", http.StatusUnprocessableEntity)
		return
	}
	completion := r.FormValue("completion")
	if !session.ValidRatedCompletion(completion) {
		http.Error(w, "bad completion", http.StatusUnprocessableEntity)
		return
	}

	shown, err := s.sessions.ShownProblems(ctx, sessionID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load shown problems failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(shown) == 0 {
		http.NotFound(w, r)
		return
	}
	last := shown[len(shown)-1]
	if seq != last.Seq || last.Completion != nil {
		http.Error(w, "stale result", http.StatusConflict)
		return
	}

	states := toShownStates(shown)

	pick, diag, err := s.rec.PickNext(ctx, recommender.PickNextInput{
		Holdsetup:       sess.Holdsetup,
		Angle:           sess.Angle,
		SessionMaxGrade: sess.MaxGrade,
		Shown:           states,
		CurrentResult:   recommender.Result{RPE: rpe, Completion: recommender.Completion(completion)},
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("session: pick next failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rpe16 := int16(rpe) //nolint:gosec // rpe is validated to 1..10 above
	if err := s.sessions.AdvanceSession(ctx, sessionID, seq, rpe16, completion, session.SessionProblem{
		Seq: seq + 1, ProblemID: pick.ProblemID, ConfigurationID: pick.ConfigurationID,
	}); err != nil {
		if errors.Is(err, session.ErrStaleResult) {
			http.Error(w, "stale result", http.StatusConflict)
			return
		}
		s.logger.Error().Err(err).Msg("session: advance failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.logAdaptivePick(sessionID, seq+1, last, rpe, completion, sess.MaxGrade, diag, pick)
	s.renderNextCard(w, r, sess, sessionID, seq+1, pick.ConfigurationID)
}

// toShownStates maps a session's seq-ordered shown list onto the recommender's
// input. A stored completion of "skipped" flags the entry as inert.
func toShownStates(shown []session.ShownProblem) []recommender.ShownState {
	states := make([]recommender.ShownState, len(shown))
	for i, sp := range shown {
		states[i] = recommender.ShownState{
			ProblemID: sp.ProblemID,
			Grade:     sp.Grade,
			Dominant:  sp.Dominant,
			Skipped:   sp.Completion != nil && *sp.Completion == session.CompletionSkipped,
		}
	}
	return states
}

// renderNextCard re-renders the #session-card fragment for the newly inserted
// problem at newSeq. Shared tail of handleResult and handleSkip.
func (s *sessionPages) renderNextCard(w http.ResponseWriter, r *http.Request, sess *session.Session, sessionID string, newSeq int, configurationID int64) {
	ctx := r.Context()

	view, err := catalog.ProblemDetail(ctx, s.pool, configurationID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load next problem detail failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Reload the shown list so it carries the resolved problem and the new
	// pick; both feed the Session-balance panel.
	shownAfter, err := s.sessions.ShownProblems(ctx, sessionID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: reload shown problems failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ladder, err := catalog.GradeLadder(ctx, s.pool, sess.Holdsetup, sess.Angle)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load grade ladder failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, pages.SessionCard(pages.SessionCardModel{
		SessionID: sessionID, Seq: newSeq, Problem: *view,
		Panel: buildSessionPanel(ladder, shownAfter),
	}), http.StatusOK)
}

// handleSkip (POST /session/{sessionID}/skip) records the current problem as
// skipped — no RPE, inert for the recommender — and swaps in another pick as if
// the skip had not happened. The skipped problem is still excluded from the rest
// of the session.
func (s *sessionPages) handleSkip(w http.ResponseWriter, r *http.Request) {
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
	if sess.Status != session.StatusActive {
		http.Error(w, "session not active", http.StatusConflict)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	seq, err := strconv.Atoi(r.FormValue("seq"))
	if err != nil {
		http.Error(w, "bad seq", http.StatusUnprocessableEntity)
		return
	}

	shown, err := s.sessions.ShownProblems(ctx, sessionID)
	if err != nil {
		s.logger.Error().Err(err).Msg("session: load shown problems failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(shown) == 0 {
		http.NotFound(w, r)
		return
	}
	last := shown[len(shown)-1]
	if seq != last.Seq || last.Completion != nil {
		http.Error(w, "stale result", http.StatusConflict)
		return
	}

	// The problem being skipped is still completion IS NULL in the DB; flag it
	// here so the recommender treats it as inert.
	states := toShownStates(shown)
	states[len(states)-1].Skipped = true

	shownIDs := make([]int64, len(shown))
	var anchor *session.ShownProblem
	for i := range shown {
		shownIDs[i] = shown[i].ProblemID
		if shown[i].RPE != nil {
			anchor = &shown[i]
		}
	}

	var (
		pick     recommender.Pick
		diag     recommender.PickDiag
		poolSize int
		anchored bool
	)
	if anchor == nil {
		// Nothing rated yet — another minimum-grade pick (FR-011).
		pick, poolSize, err = s.rec.FirstPickExcluding(ctx, sess.Holdsetup, sess.Angle, shownIDs)
		if err != nil {
			s.logger.Error().Err(err).Msg("session: skip first-pick failed")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		anchored = true
		pick, diag, err = s.rec.PickNext(ctx, recommender.PickNextInput{
			Holdsetup:       sess.Holdsetup,
			Angle:           sess.Angle,
			SessionMaxGrade: sess.MaxGrade,
			Shown:           states,
			CurrentResult:   recommender.Result{RPE: int(*anchor.RPE), Completion: recommender.Completion(*anchor.Completion)},
		})
		if err != nil {
			s.logger.Error().Err(err).Msg("session: skip pick next failed")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	if err := s.sessions.SkipProblem(ctx, sessionID, seq, session.SessionProblem{
		Seq: seq + 1, ProblemID: pick.ProblemID, ConfigurationID: pick.ConfigurationID,
	}); err != nil {
		if errors.Is(err, session.ErrStaleResult) {
			http.Error(w, "stale result", http.StatusConflict)
			return
		}
		s.logger.Error().Err(err).Msg("session: skip advance failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if anchored {
		s.logAdaptivePick(sessionID, seq+1, *anchor, int(*anchor.RPE), *anchor.Completion, sess.MaxGrade, diag, pick)
	} else {
		s.logFirstPick(sessionID, seq+1, pick, poolSize)
	}
	s.renderNextCard(w, r, sess, sessionID, seq+1, pick.ConfigurationID)
}

// handleEnd (POST /session/{sessionID}/end) ends the active session and
// redirects to the hub. Ownership failure is an identical 404.
func (s *sessionPages) handleEnd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	sessionID := chi.URLParam(r, "sessionID")
	if err := s.sessions.EndSession(ctx, sessionID, userID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logger.Error().Err(err).Msg("session: end failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}
