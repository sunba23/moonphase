package server

import (
	"testing"

	"github.com/sunba23/moonphase/internal/session"
)

var panelLadder = []string{"6A", "6A+", "6B", "6B+", "6C", "6C+", "7A", "7A+"}

func climbedProblem(grade, dominant string) session.ShownProblem {
	rpe := int16(5)
	done := session.CompletionSent
	return session.ShownProblem{Grade: grade, Dominant: dominant, RPE: &rpe, Completion: &done}
}

func currentProblem(grade, dominant string) session.ShownProblem {
	return session.ShownProblem{Grade: grade, Dominant: dominant}
}

func TestBuildSessionPanel_NilBeforeFirstClimb(t *testing.T) {
	if got := buildSessionPanel(panelLadder, nil); got != nil {
		t.Fatalf("nil shown: got %+v, want nil", got)
	}
	// Session start: seq 0 only, nothing climbed yet.
	only := []session.ShownProblem{currentProblem("6A", "crimp")}
	if got := buildSessionPanel(panelLadder, only); got != nil {
		t.Fatalf("one unrated problem: got %+v, want nil", got)
	}
}

func TestBuildSessionPanel_Tally(t *testing.T) {
	shown := []session.ShownProblem{
		climbedProblem("6A", "crimp"),
		climbedProblem("6A+", "crimp"),
		climbedProblem("6B", "sloper"),
		climbedProblem("6B", ""),        // untagged -> dropped from the tally
		climbedProblem("6B", "unknown"), // not an allowed type -> dropped
		currentProblem("6B", "jug"),
	}
	p := buildSessionPanel(panelLadder, shown)
	if p == nil {
		t.Fatal("got nil panel, want a panel")
	}
	if p.Climbed != 5 {
		t.Fatalf("Climbed = %d, want 5", p.Climbed)
	}
	want := map[string]int{"crimp": 2, "sloper": 1, "pinch": 0, "jug": 0, "pocket": 0}
	if len(p.Bars) != len(want) {
		t.Fatalf("Bars len = %d, want %d", len(p.Bars), len(want))
	}
	order := []string{"crimp", "sloper", "pinch", "jug", "pocket"}
	for i, b := range p.Bars {
		if b.Type != order[i] {
			t.Fatalf("Bars[%d].Type = %q, want %q", i, b.Type, order[i])
		}
		if b.Count != want[b.Type] {
			t.Fatalf("Bars[%d] (%s).Count = %d, want %d", i, b.Type, b.Count, want[b.Type])
		}
	}
	if p.MaxCount != 2 {
		t.Fatalf("MaxCount = %d, want 2", p.MaxCount)
	}
}

func TestBuildSessionPanel_GradeTag(t *testing.T) {
	tests := []struct {
		name             string
		prevGrade, curGr string
		want             string
	}{
		{"harder", "6B", "6B+", "Harder"},
		{"easier", "6B", "6A+", "Easier"},
		{"holding", "6B", "6B", "Holding"},
		{"current off ladder", "6B", "9Z", ""},
		{"prev off ladder", "9Z", "6B", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := []session.ShownProblem{
				climbedProblem(tt.prevGrade, "crimp"),
				currentProblem(tt.curGr, "sloper"),
			}
			p := buildSessionPanel(panelLadder, shown)
			if p == nil {
				t.Fatal("got nil panel")
			}
			if p.GradeTag != tt.want {
				t.Fatalf("GradeTag = %q, want %q", p.GradeTag, tt.want)
			}
		})
	}
}

func TestBuildSessionPanel_HoldTag(t *testing.T) {
	tests := []struct {
		name    string
		climbed []string // dominants of climbed problems, oldest first
		cur     string
		want    string
	}{
		{"streak broken", []string{"crimp", "crimp", "crimp"}, "sloper", "Off crimp"},
		{"streak not broken - still crimp", []string{"crimp", "crimp", "crimp"}, "crimp", "More crimp"},
		{"only two in a row is not a streak", []string{"jug", "crimp", "crimp"}, "sloper", "Onto sloper"},
		{"change without streak", []string{"crimp", "sloper"}, "jug", "Onto jug"},
		{"repeat without streak", []string{"crimp", "sloper"}, "sloper", "More sloper"},
		{"untagged current pick", []string{"crimp", "crimp", "crimp"}, "", ""},
		{"streak of untagged is not a streak", []string{"", "", ""}, "crimp", "Onto crimp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := make([]session.ShownProblem, 0, len(tt.climbed)+1)
			for _, d := range tt.climbed {
				shown = append(shown, climbedProblem("6B", d))
			}
			shown = append(shown, currentProblem("6B", tt.cur))
			p := buildSessionPanel(panelLadder, shown)
			if p == nil {
				t.Fatal("got nil panel")
			}
			if p.HoldTag != tt.want {
				t.Fatalf("HoldTag = %q, want %q", p.HoldTag, tt.want)
			}
		})
	}
}
