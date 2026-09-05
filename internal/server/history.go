package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/templates/pages"
)

// historyPages wires the read-only past-sessions surface (FR-013, FR-014) to
// the session store and the catalog problem-detail query. Mirrors sessionPages.
type historyPages struct {
	pool     *pgxpool.Pool
	sessions *session.Store
	logger   *zerolog.Logger
}

func newHistoryPages(pool *pgxpool.Pool, sessions *session.Store, logger *zerolog.Logger) *historyPages {
	return &historyPages{pool: pool, sessions: sessions, logger: logger}
}

// historyDateFormat is the pre-formatted started-at shown in the list and
// detail headers — the template does no date work.
const historyDateFormat = "2006-01-02"

// handleList (GET /sessions) renders the caller's ended sessions, newest
// first. An empty list renders the empty state.
func (h *historyPages) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	summaries, err := h.sessions.ListForUser(ctx, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("history: list for user failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows := make([]pages.HistorySessionRow, len(summaries))
	for i, s := range summaries {
		name, _ := catalog.BoardName(s.Holdsetup)
		rows[i] = pages.HistorySessionRow{
			ID:           s.ID,
			BoardName:    name,
			StartedAt:    s.StartedAt.Format(historyDateFormat),
			Angle:        s.Angle,
			ClimbedCount: s.ClimbedCount,
		}
	}

	renderAppPage(w, r, "Past sessions", pages.HistoryListContent(pages.HistoryListModel{Sessions: rows}), http.StatusOK)
}

// ownedEndedSession resolves the {sessionID} route param to the caller's ended
// session. A missing id, a non-owner, and a still-active session all collapse
// to an identical 404 — the id's existence is never confirmed to a non-owner.
func (h *historyPages) ownedEndedSession(w http.ResponseWriter, r *http.Request) (*session.Session, bool) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return nil, false
	}

	sess, err := h.sessions.Get(ctx, chi.URLParam(r, "sessionID"))
	if err != nil || sess.UserID != userID || sess.Status != session.StatusEnded {
		http.NotFound(w, r)
		return nil, false
	}
	return sess, true
}

// handleDetail (GET /sessions/{sessionID}) renders one ended session's climbed
// problems in climb order. An empty list renders the detail empty state.
func (h *historyPages) handleDetail(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownedEndedSession(w, r)
	if !ok {
		return
	}

	climbed, err := h.sessions.ClimbedProblems(r.Context(), sess.ID)
	if err != nil {
		h.logger.Error().Err(err).Msg("history: climbed problems failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows := make([]pages.HistoryProblemRow, len(climbed))
	for i, p := range climbed {
		rows[i] = pages.HistoryProblemRow{
			Seq:        p.Seq,
			Name:       p.Name,
			Grade:      p.Grade,
			Completion: p.Completion,
			RPE:        p.RPE,
		}
	}

	name, _ := catalog.BoardName(sess.Holdsetup)
	renderAppPage(w, r, "Session", pages.HistoryDetailContent(pages.HistoryDetailModel{
		SessionID: sess.ID,
		BoardName: name,
		StartedAt: sess.StartedAt.Format(historyDateFormat),
		Angle:     sess.Angle,
		Problems:  rows,
	}), http.StatusOK)
}

// handleProblem (GET /sessions/{sessionID}/problem/{seq}) renders the
// read-only MoonBoard layout for one climbed problem plus the recorded result.
// A non-numeric seq, an unrated seq, and an out-of-range seq all 404.
func (h *historyPages) handleProblem(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownedEndedSession(w, r)
	if !ok {
		return
	}

	seq, err := strconv.Atoi(chi.URLParam(r, "seq"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ref, err := h.sessions.ClimbedProblemAt(r.Context(), sess.ID, seq)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error().Err(err).Msg("history: climbed problem at failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	view, err := catalog.ProblemDetail(r.Context(), h.pool, ref.ConfigurationID)
	if err != nil {
		h.logger.Error().Err(err).Msg("history: problem detail failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderAppPage(w, r, "Problem", pages.HistoryProblemContent(pages.HistoryProblemModel{
		SessionID:  sess.ID,
		Seq:        seq,
		RPE:        ref.RPE,
		Completion: ref.Completion,
		Problem:    *view,
	}), http.StatusOK)
}
