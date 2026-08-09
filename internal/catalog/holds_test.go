package catalog

import (
	"strings"
	"testing"
)

func TestValidateHoldRows(t *testing.T) {
	tests := []struct {
		name    string
		rows    []HoldRow
		wantErr bool
	}{
		{
			name: "all valid",
			rows: []HoldRow{
				{GridRef: "A1", PrimaryType: "crimp"},
				{GridRef: "A2", PrimaryType: "sloper"},
			},
			wantErr: false,
		},
		{
			name: "blank primary type is untagged, not invalid",
			rows: []HoldRow{
				{GridRef: "A1", PrimaryType: ""},
			},
			wantErr: false,
		},
		{
			name: "one invalid row fails the whole batch",
			rows: []HoldRow{
				{GridRef: "A1", PrimaryType: "crimp"},
				{GridRef: "A2", PrimaryType: "not-a-real-type"},
			},
			wantErr: true,
		},
		{
			name: "multiple invalid rows all reported",
			rows: []HoldRow{
				{GridRef: "A1", PrimaryType: "bogus1"},
				{GridRef: "A2", PrimaryType: "bogus2"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHoldRows(tt.rows)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHoldRows() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateHoldRowsReportsEveryBadRow(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "A1", PrimaryType: "bogus1"},
		{GridRef: "A2", PrimaryType: "crimp"},
		{GridRef: "A3", PrimaryType: "bogus2"},
	}

	err := validateHoldRows(rows)
	if err == nil {
		t.Fatal("validateHoldRows() = nil, want error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "A1") || !strings.Contains(msg, "A3") {
		t.Errorf("validateHoldRows() error = %q, want both A1 and A3 named", msg)
	}
	if strings.Contains(msg, "A2") {
		t.Errorf("validateHoldRows() error = %q, should not mention valid row A2", msg)
	}
}
