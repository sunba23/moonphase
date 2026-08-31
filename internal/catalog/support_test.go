package catalog

import "testing"

func TestSupportedBoards(t *testing.T) {
	got := SupportedBoards()
	if len(got) != 2 {
		t.Fatalf("SupportedBoards() len = %d, want 2 (%+v)", len(got), got)
	}
	if got[0].Holdsetup != 1 || got[0].Name != "2016" {
		t.Errorf("got[0] = %+v, want {1 2016}", got[0])
	}
	if got[1].Holdsetup != 21 || got[1].Name != "2024" {
		t.Errorf("got[1] = %+v, want {21 2024}", got[1])
	}
}

func TestBoardImageYear(t *testing.T) {
	tests := []struct {
		holdsetup int16
		wantYear  string
		wantOK    bool
	}{
		{1, "2016", true},
		{21, "2024", true},
		{15, "", false},
		{17, "", false},
	}
	for _, tt := range tests {
		year, ok := BoardImageYear(tt.holdsetup)
		if year != tt.wantYear || ok != tt.wantOK {
			t.Errorf("BoardImageYear(%d) = (%q, %v), want (%q, %v)", tt.holdsetup, year, ok, tt.wantYear, tt.wantOK)
		}
	}
}

func TestMoveTypeRole(t *testing.T) {
	tests := map[string]string{
		"s": "start", "e": "finish", "f": "foot",
		"l": "hand", "r": "hand", "p": "hand", "m": "hand", "o": "hand", "": "hand",
	}
	for code, want := range tests {
		if got := moveTypeRole(code); got != want {
			t.Errorf("moveTypeRole(%q) = %q, want %q", code, got, want)
		}
	}
}
