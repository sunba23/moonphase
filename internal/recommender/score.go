package recommender

import "math"

// Scoring weights. Tunable by hand — no learned model (PRD Non-Goal). wRecent
// + wSession can reach ~1.0, so a fully-loaded hold-type balance penalty can
// override a one-step grade miss. That is deliberate per FR-012: the right
// pick after a hard crimp send is not the next-grade-up crimp problem.
const (
	wGrade   = 1.00 // per ladder step of |GradeIndex - PreferredIndex|
	wRecent  = 0.70 // * share of last 3 shown problems with this dominant
	wSession = 0.30 // * share of all session scored-hold dominants
	wNovel   = 0.20 // bonus if Dominant != "" and Dominant != PrevDominant
	wQuality = 0.15 // bonus if Quality

	// scoreEpsilon groups near-equal scores into one tie set, resolved by roll.
	scoreEpsilon = 1e-9
)

// ScoreCandidate is one problem the next pick may choose, already carrying its
// ladder position and dominant hold type.
type ScoreCandidate struct {
	ProblemID       int64
	ConfigurationID int64
	GradeIndex      int
	Dominant        string
	Quality         bool
}

// ScoreState is the session-so-far context the scorer weighs each candidate
// against.
type ScoreState struct {
	PreferredIndex        int
	RecentDominants       []string       // last up-to-3 shown dominants
	SessionDominantCounts map[string]int // all session scored-hold dominants
	PrevDominant          string
	DropBalance           bool // fallback tier 1: zero the balance terms
}

func recentShare(recent []string, d string) float64 {
	if d == "" || len(recent) == 0 {
		return 0
	}
	n := 0
	for _, r := range recent {
		if r == d {
			n++
		}
	}
	return float64(n) / float64(len(recent))
}

func sessionShare(counts map[string]int, d string) float64 {
	if d == "" || len(counts) == 0 {
		return 0
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return 0
	}
	return float64(counts[d]) / float64(total)
}

func scoreCandidate(c ScoreCandidate, st ScoreState) float64 {
	s := -wGrade * math.Abs(float64(c.GradeIndex-st.PreferredIndex))

	if !st.DropBalance {
		s -= wRecent * recentShare(st.RecentDominants, c.Dominant)
		s -= wSession * sessionShare(st.SessionDominantCounts, c.Dominant)
	}

	if c.Dominant != "" && c.Dominant != st.PrevDominant {
		s += wNovel
	}
	if c.Quality {
		s += wQuality
	}

	return s
}

// scoreNext scores every candidate on grade fit + hold-type balance + variety
// + quality and returns the index of the argmax. Candidates within
// scoreEpsilon of the max form the tie set, resolved by roll. Empty cands ->
// (-1, ErrNoCandidates). Pure and deterministic given roll.
func scoreNext(cands []ScoreCandidate, st ScoreState, roll func(n int) int) (int, error) {
	if len(cands) == 0 {
		return -1, ErrNoCandidates
	}

	best := math.Inf(-1)
	for _, c := range cands {
		if s := scoreCandidate(c, st); s > best {
			best = s
		}
	}

	var tie []int
	for i, c := range cands {
		if scoreCandidate(c, st) >= best-scoreEpsilon {
			tie = append(tie, i)
		}
	}

	return tie[roll(len(tie))], nil
}
