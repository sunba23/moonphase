package catalog

import "testing"

func TestParseAngle(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    int
		wantErr bool
	}{
		{name: "25 degrees", s: "25°", want: 25},
		{name: "40 degrees", s: "40°", want: 40},
		{name: "missing degree sign", s: "25", want: 25},
		{name: "malformed", s: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAngle(tt.s)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAngle(%q) error = nil, want error", tt.s)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAngle(%q) error = %v, want nil", tt.s, err)
			}
			if got != tt.want {
				t.Errorf("ParseAngle(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}
