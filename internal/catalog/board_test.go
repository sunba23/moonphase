package catalog

import "testing"

func TestResolveBoardYear(t *testing.T) {
	tests := []struct {
		year          string
		wantHoldsetup int
		wantErr       bool
	}{
		{"2016", 1, false},
		{"2017", 15, false},
		{"2019", 17, false},
		{"2024", 21, false},
		{"2020", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.year, func(t *testing.T) {
			got, err := ResolveBoardYear(tt.year)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveBoardYear(%q) = %d, nil; want error", tt.year, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveBoardYear(%q) unexpected error: %v", tt.year, err)
			}
			if got != tt.wantHoldsetup {
				t.Errorf("ResolveBoardYear(%q) = %d; want %d", tt.year, got, tt.wantHoldsetup)
			}
		})
	}
}
