package recommender

// Completion is the per-problem completion status, mirrored from
// internal/session so the recommender core stays import-free of that package.
type Completion string

const (
	CompletionSent   Completion = "sent"
	CompletionFailed Completion = "failed"
	CompletionBailed Completion = "bailed"
)

// Result is the previous problem's outcome: an RPE 1–10 plus a completion
// status.
type Result struct {
	RPE        int
	Completion Completion
}

// band is the direction the next grade may move, derived from a Result.
type band int

const (
	bandBackOff band = iota // same-or-easier
	bandHold                // same grade
	bandStepUp              // same or one harder
)

// classify maps a Result to a band. The locked 4-band rule:
//   - failed / bailed, OR RPE >= 8 (on any status) -> backOff
//   - RPE 5–7 and sent                             -> hold
//   - RPE <= 4 and sent                            -> stepUp
func classify(r Result) band {
	if r.Completion != CompletionSent {
		return bandBackOff
	}
	if r.RPE >= 8 {
		return bandBackOff
	}
	if r.RPE <= 4 {
		return bandStepUp
	}
	return bandHold
}

// indexOf returns the position of grade on ladder, or -1.
func indexOf(ladder []string, grade string) int {
	for i, g := range ladder {
		if g == grade {
			return i
		}
	}
	return -1
}

// oneEasier returns the ladder grade one step below cur, clamped at the bottom.
func oneEasier(ladder []string, cur string) string {
	i := indexOf(ladder, cur)
	if i <= 0 {
		if len(ladder) == 0 {
			return cur
		}
		return ladder[0]
	}
	return ladder[i-1]
}

// oneHarder returns the ladder grade one step above cur, clamped at the top.
func oneHarder(ladder []string, cur string) string {
	i := indexOf(ladder, cur)
	if i < 0 || i >= len(ladder)-1 {
		if i < 0 && len(ladder) > 0 {
			return ladder[len(ladder)-1]
		}
		return cur
	}
	return ladder[i+1]
}

// gradeWindow returns the inclusive [lo, hi] band of catalog grades the next
// pick may have, before the session ceiling clamp. When cur is off-ladder it
// returns a wide safe window and ok = false, so the caller can log the
// anomaly.
//
// Invariant: for backOff and hold, hi is never above cur.
func gradeWindow(ladder []string, cur string, b band) (lo, hi string, ok bool) {
	if len(ladder) == 0 {
		return cur, cur, false
	}
	if indexOf(ladder, cur) < 0 {
		return ladder[0], ladder[len(ladder)-1], false
	}

	switch b {
	case bandBackOff:
		return oneEasier(ladder, cur), cur, true
	case bandStepUp:
		return cur, oneHarder(ladder, cur), true
	default: // bandHold
		return cur, cur, true
	}
}

// preferredIndex is the ladder index the scorer aims at inside the window.
// backOff aims one easier, stepUp aims one harder, hold stays put — all
// clamped to the ladder.
func preferredIndex(ladder []string, cur string, b band) int {
	i := indexOf(ladder, cur)
	if i < 0 {
		return 0
	}
	switch b {
	case bandBackOff:
		if i > 0 {
			return i - 1
		}
		return 0
	case bandStepUp:
		if i < len(ladder)-1 {
			return i + 1
		}
		return i
	default:
		return i
	}
}

// minGradeOnLadder returns whichever of a, b sits lower on the ladder. An
// off-ladder argument loses (the on-ladder one wins); both off-ladder returns
// a.
func minGradeOnLadder(ladder []string, a, b string) string {
	ia, ib := indexOf(ladder, a), indexOf(ladder, b)
	switch {
	case ia < 0 && ib < 0:
		return a
	case ia < 0:
		return b
	case ib < 0:
		return a
	case ib < ia:
		return b
	default:
		return a
	}
}
