package catalog

import (
	"reflect"
	"testing"
)

func TestParseMoves(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []Move
		wantErr bool
	}{
		{
			name: "valid multi-token string",
			raw:  "s~C5~|p~C13~|p~D15~|e~D18~",
			want: []Move{
				{Seq: 0, Type: "s", GridRef: "C5"},
				{Seq: 1, Type: "p", GridRef: "C13"},
				{Seq: 2, Type: "p", GridRef: "D15"},
				{Seq: 3, Type: "e", GridRef: "D18"},
			},
		},
		{
			name: "single start single end",
			raw:  "s~A1~|e~A2~",
			want: []Move{
				{Seq: 0, Type: "s", GridRef: "A1"},
				{Seq: 1, Type: "e", GridRef: "A2"},
			},
		},
		{
			name: "two start tokens",
			raw:  "s~A1~|s~B1~|e~C1~",
			want: []Move{
				{Seq: 0, Type: "s", GridRef: "A1"},
				{Seq: 1, Type: "s", GridRef: "B1"},
				{Seq: 2, Type: "e", GridRef: "C1"},
			},
		},
		{
			name:    "malformed token missing tilde",
			raw:     "s~A1|e~B1~",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMoves(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMoves(%q) error = nil, want error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMoves(%q) error = %v, want nil", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMoves(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}
