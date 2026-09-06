package server

import (
	"github.com/sunba23/moonphase/internal/recommender"
	"github.com/sunba23/moonphase/internal/session"
)

// The rec_pick log line records how the recommender chose each problem a
// session serves. One line per pick, keyed by session_id, so a session's
// decision trail is `grep '"session_id":"…"' | grep rec_pick`. Field names are
// a stable query contract once shipped — do not rename casually.

// logAdaptivePick emits the decision line for a scored PickNext pick: the input
// (previous problem + result + ceiling) and the decision (band, grade window,
// preferred grade, streak exclusion, fallback tier, tie-set size, chosen pick).
func (s *sessionPages) logAdaptivePick(
	sessionID string, seq int, prev session.ShownProblem,
	rpe int, completion, ceiling string,
	diag recommender.PickDiag, pick recommender.Pick,
) {
	s.logger.Info().
		Str("kind", "adaptive").
		Str("session_id", sessionID).
		Int("seq", seq).
		Int64("prev_problem_id", prev.ProblemID).
		Str("prev_grade", prev.Grade).
		Str("prev_dominant", prev.Dominant).
		Int("rpe", rpe).
		Str("completion", completion).
		Str("band", diag.Band).
		Str("ceiling", ceiling).
		Str("grade_lo", diag.GradeLo).
		Str("grade_hi", diag.GradeHi).
		Str("pref_grade", diag.PreferredGrade).
		Str("excluded_dominant", diag.ExcludedDominant).
		Int("fallback_tier", diag.FallbackTier).
		Int("tie_set_size", diag.TieSetSize).
		Int64("pick_problem_id", pick.ProblemID).
		Str("pick_grade", pick.Grade).
		Str("pick_dominant", pick.Dominant).
		Msg("rec_pick")
}

// logFirstPick emits the minimal decision line for a FirstPick-style pick — the
// session's opening problem, or a skip taken before anything was rated. The
// only "reason" is minimum grade (FR-011), so the line just anchors the pick
// in the session's grep trail.
func (s *sessionPages) logFirstPick(sessionID string, seq int, pick recommender.Pick, poolSize int) {
	s.logger.Info().
		Str("kind", "first_pick").
		Str("session_id", sessionID).
		Int("seq", seq).
		Int64("pick_problem_id", pick.ProblemID).
		Str("pick_grade", pick.Grade).
		Int("pool_size", poolSize).
		Msg("rec_pick")
}
