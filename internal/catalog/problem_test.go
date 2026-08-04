package catalog

import (
	"encoding/json"
	"testing"
	"time"
)

func TestShouldIngest(t *testing.T) {
	deleted := SourceTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name string
		p    Problem
		want bool
	}{
		{
			name: "active and not deleted",
			p:    Problem{Active: true, DateDeleted: nil},
			want: true,
		},
		{
			name: "inactive",
			p:    Problem{Active: false, DateDeleted: nil},
			want: false,
		},
		{
			name: "deleted",
			p:    Problem{Active: true, DateDeleted: &deleted},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldIngest(tt.p); got != tt.want {
				t.Errorf("ShouldIngest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceTimeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    time.Time
		wantErr bool
	}{
		{
			name: "no fractional seconds",
			raw:  `"2016-03-01T10:00:27"`,
			want: time.Date(2016, 3, 1, 10, 0, 27, 0, time.UTC),
		},
		{
			name: "millisecond fractional seconds",
			raw:  `"2026-06-25T21:58:25.013"`,
			want: time.Date(2026, 6, 25, 21, 58, 25, 13_000_000, time.UTC),
		},
		{
			name:    "RFC3339 with Z is rejected",
			raw:     `"2019-05-01T12:00:00Z"`,
			wantErr: true,
		},
		{
			name:    "malformed",
			raw:     `"not-a-time"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st SourceTime
			err := json.Unmarshal([]byte(tt.raw), &st)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) error = nil, want error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s) error = %v, want nil", tt.raw, err)
			}
			if !st.Time().Equal(tt.want) {
				t.Errorf("Unmarshal(%s) = %v, want %v", tt.raw, st.Time(), tt.want)
			}
		})
	}
}

func TestProblemUnmarshalJSON(t *testing.T) {
	raw := `{
		"id": 12345,
		"name": "Test Problem",
		"dateInserted": "2019-05-01T12:00:00",
		"dateUpdated": "2020-06-02T08:30:00.123",
		"dateDeleted": null,
		"holdsetup": 1,
		"Active": true,
		"climbMethod": "Feet follow hands",
		"setbyId": "abc-123",
		"setter": "Some Setter",
		"moves": "s~C5~|p~C13~|p~D15~|e~D18~",
		"coordinates": null,
		"holdsets": "",
		"betaVideos": 0,
		"configurations": [
			{
				"apiId": 1,
				"grade": "6B+",
				"userGrade": "6B",
				"userRating": 3,
				"dateUpdated": "2020-06-02T08:30:00.123",
				"dateDeleted": null,
				"isBenchmark": false,
				"isCompetitionProblem": false,
				"comment": null,
				"isPrimary": true,
				"repeats": 42,
				"configuration": "25°",
				"primaryAngle": "25°"
			}
		]
	}`

	var p Problem
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if p.ID != 12345 {
		t.Errorf("ID = %d, want 12345", p.ID)
	}
	if p.Name != "Test Problem" {
		t.Errorf("Name = %q, want %q", p.Name, "Test Problem")
	}
	if !p.Active {
		t.Errorf("Active = false, want true")
	}
	if p.DateDeleted != nil {
		t.Errorf("DateDeleted = %v, want nil", p.DateDeleted)
	}
	if p.DateInserted == nil || !p.DateInserted.Time().Equal(time.Date(2019, 5, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("DateInserted = %v, want 2019-05-01T12:00:00", p.DateInserted)
	}
	if p.Holdsetup != 1 {
		t.Errorf("Holdsetup = %d, want 1", p.Holdsetup)
	}
	if p.SetbyID != "abc-123" {
		t.Errorf("SetbyID = %q, want %q", p.SetbyID, "abc-123")
	}
	if p.Moves != "s~C5~|p~C13~|p~D15~|e~D18~" {
		t.Errorf("Moves = %q", p.Moves)
	}
	if len(p.Configurations) != 1 {
		t.Fatalf("len(Configurations) = %d, want 1", len(p.Configurations))
	}

	cfg := p.Configurations[0]
	if cfg.APIID != 1 {
		t.Errorf("APIID = %d, want 1", cfg.APIID)
	}
	if cfg.Grade != "6B+" {
		t.Errorf("Grade = %q, want %q", cfg.Grade, "6B+")
	}
	if cfg.Configuration != "25°" {
		t.Errorf("Configuration = %q, want %q", cfg.Configuration, "25°")
	}
	if !cfg.IsPrimary {
		t.Errorf("IsPrimary = false, want true")
	}
}
