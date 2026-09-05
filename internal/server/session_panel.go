package server

import (
	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/templates/pages"
)

// buildSessionPanel assembles the collapsed "Session balance" panel from a
// session's seq-ordered shown problems (last entry = the current, unrated
// problem) and the board's grade ladder. It returns nil until at least one
// problem has been climbed.
//
// Both axes are reconstructed here, not read back from the recommender: the
// tally counts each climbed problem's dominant hold type; the rationale
// compares the current pick to the climbed problems before it. Untagged
// dominants are dropped, mirroring the engine excluding untagged holds from its
// balance math.
func buildSessionPanel(ladder []string, shown []session.ShownProblem) *pages.SessionPanel {
	if len(shown) < 2 {
		return nil
	}
	current := shown[len(shown)-1]

	climbed := make([]session.ShownProblem, 0, len(shown)-1)
	for _, sp := range shown[:len(shown)-1] {
		if sp.RPE != nil {
			climbed = append(climbed, sp)
		}
	}
	if len(climbed) == 0 {
		return nil
	}

	counts := make(map[string]int, len(catalog.AllowedHoldTypes))
	for _, sp := range climbed {
		if isAllowedHoldType(sp.Dominant) {
			counts[sp.Dominant]++
		}
	}

	bars := make([]pages.HoldTypeBar, 0, len(catalog.AllowedHoldTypes))
	maxCount := 1
	for _, t := range catalog.AllowedHoldTypes {
		n := counts[t]
		if n > maxCount {
			maxCount = n
		}
		bars = append(bars, pages.HoldTypeBar{Type: t, Count: n})
	}

	return &pages.SessionPanel{
		Bars:     bars,
		MaxCount: maxCount,
		Climbed:  len(climbed),
		GradeTag: gradeTag(ladder, climbed[len(climbed)-1].Grade, current.Grade),
		HoldTag:  holdTag(climbed, current.Dominant),
	}
}

// gradeTag names the current pick's grade move relative to the last climbed
// problem, by position on the ladder. Either grade off the ladder -> "".
func gradeTag(ladder []string, prevGrade, curGrade string) string {
	pi, ci := gradeIndex(ladder, prevGrade), gradeIndex(ladder, curGrade)
	if pi < 0 || ci < 0 {
		return ""
	}
	switch {
	case ci > pi:
		return "Harder"
	case ci < pi:
		return "Easier"
	default:
		return "Holding"
	}
}

// holdTag names the current pick's hold-type shift. A 3-long same-dominant
// streak that the pick moved off reads "Off <type>" (the injury-avoidance
// mechanism made visible); otherwise "Onto <type>" on a change, "More <type>"
// on a repeat. An untagged current pick -> "".
func holdTag(climbed []session.ShownProblem, cur string) string {
	if !isAllowedHoldType(cur) {
		return ""
	}
	if len(climbed) >= 3 {
		s := climbed[len(climbed)-1].Dominant
		if isAllowedHoldType(s) &&
			climbed[len(climbed)-2].Dominant == s &&
			climbed[len(climbed)-3].Dominant == s &&
			cur != s {
			return "Off " + s
		}
	}
	if last := climbed[len(climbed)-1].Dominant; cur != last {
		return "Onto " + cur
	}
	return "More " + cur
}

func gradeIndex(ladder []string, g string) int {
	for i, l := range ladder {
		if l == g {
			return i
		}
	}
	return -1
}

func isAllowedHoldType(s string) bool {
	for _, t := range catalog.AllowedHoldTypes {
		if s == t {
			return true
		}
	}
	return false
}
