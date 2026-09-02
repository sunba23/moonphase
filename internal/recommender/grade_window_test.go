package recommender

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		r    Result
		want band
	}{
		{"low rpe bailed -> backOff", Result{RPE: 3, Completion: CompletionBailed}, bandBackOff},
		{"high rpe sent -> backOff", Result{RPE: 8, Completion: CompletionSent}, bandBackOff},
		{"mid rpe sent -> hold", Result{RPE: 6, Completion: CompletionSent}, bandHold},
		{"low rpe sent -> stepUp", Result{RPE: 4, Completion: CompletionSent}, bandStepUp},
		{"low rpe failed -> backOff", Result{RPE: 2, Completion: CompletionFailed}, bandBackOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.r); got != tt.want {
				t.Fatalf("classify(%+v) = %d, want %d", tt.r, got, tt.want)
			}
		})
	}
}

func TestGradeWindow(t *testing.T) {
	ladder := []string{"6B", "6B+", "6C", "6C+", "7A"}

	lo, hi, ok := gradeWindow(ladder, "7A", bandStepUp)
	if !ok || hi != "7A" {
		t.Fatalf("top + stepUp = [%s,%s] ok=%v, want hi 7A", lo, hi, ok)
	}

	lo, hi, ok = gradeWindow(ladder, "6B", bandBackOff)
	if !ok || lo != "6B" {
		t.Fatalf("bottom + backOff = [%s,%s] ok=%v, want lo 6B", lo, hi, ok)
	}

	lo, hi, ok = gradeWindow(ladder, "6C", bandHold)
	if !ok || lo != "6C" || hi != "6C" {
		t.Fatalf("hold = [%s,%s] ok=%v, want [6C,6C]", lo, hi, ok)
	}

	lo, hi, ok = gradeWindow(ladder, "8A", bandHold)
	if ok {
		t.Fatalf("off-ladder cur = [%s,%s] ok=%v, want ok=false", lo, hi, ok)
	}
}

func TestPreferredIndexAndMin(t *testing.T) {
	ladder := []string{"6B", "6B+", "6C"}
	if got := preferredIndex(ladder, "6B+", bandStepUp); got != 2 {
		t.Fatalf("preferredIndex stepUp = %d, want 2", got)
	}
	if got := preferredIndex(ladder, "6C", bandStepUp); got != 2 {
		t.Fatalf("preferredIndex stepUp at top = %d, want 2 (clamped)", got)
	}
	if got := preferredIndex(ladder, "6B", bandBackOff); got != 0 {
		t.Fatalf("preferredIndex backOff at bottom = %d, want 0 (clamped)", got)
	}
	if got := minGradeOnLadder(ladder, "6C", "6B+"); got != "6B+" {
		t.Fatalf("minGradeOnLadder = %s, want 6B+", got)
	}
	if got := minGradeOnLadder(ladder, "6C", "9A"); got != "6C" {
		t.Fatalf("minGradeOnLadder with off-ladder = %s, want 6C", got)
	}
}
