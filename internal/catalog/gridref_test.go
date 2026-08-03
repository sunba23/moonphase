package catalog

import (
	"sort"
	"testing"
)

func TestParseGridRef(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantCol string
		wantRow int
		wantErr bool
	}{
		{name: "C5", s: "C5", wantCol: "C", wantRow: 5},
		{name: "K18", s: "K18", wantCol: "K", wantRow: 18},
		{name: "no digits", s: "C", wantErr: true},
		{name: "no letters", s: "5", wantErr: true},
		{name: "row too long", s: "C123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, row, err := ParseGridRef(tt.s)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseGridRef(%q) error = nil, want error", tt.s)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGridRef(%q) error = %v, want nil", tt.s, err)
			}
			if col != tt.wantCol || row != tt.wantRow {
				t.Errorf("ParseGridRef(%q) = (%q, %d), want (%q, %d)", tt.s, col, row, tt.wantCol, tt.wantRow)
			}
		})
	}
}

func TestLessSortOrder(t *testing.T) {
	refs := []string{"B1", "A2", "A18", "A1"}
	want := []string{"A1", "A2", "A18", "B1"}

	sort.Slice(refs, func(i, j int) bool { return Less(refs[i], refs[j]) })

	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("sorted[%d] = %q, want %q (full: %v)", i, refs[i], want[i], refs)
		}
	}
}
