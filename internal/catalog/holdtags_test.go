package catalog

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateHoldType(t *testing.T) {
	for _, ht := range AllowedHoldTypes {
		if err := ValidateHoldType(ht); err != nil {
			t.Errorf("ValidateHoldType(%q) = %v, want nil", ht, err)
		}
	}

	if err := ValidateHoldType("edge"); err == nil {
		t.Error("ValidateHoldType(\"edge\") = nil, want error")
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "A2", PrimaryType: "crimp", Modifiers: []string{"incut", "small"}},
		{GridRef: "A1", PrimaryType: "jug"},
		{GridRef: "B1"},
	}

	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, rows); err != nil {
		t.Fatalf("WriteInventoryCSV: %v", err)
	}

	got, err := ReadTagsCSV(&buf)
	if err != nil {
		t.Fatalf("ReadTagsCSV: %v", err)
	}

	want := []HoldRow{
		{GridRef: "A1", PrimaryType: "jug"},
		{GridRef: "A2", PrimaryType: "crimp", Modifiers: []string{"incut", "small"}},
		{GridRef: "B1"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].GridRef != want[i].GridRef || got[i].PrimaryType != want[i].PrimaryType {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
		if strings.Join(got[i].Modifiers, ";") != strings.Join(want[i].Modifiers, ";") {
			t.Errorf("row %d modifiers = %v, want %v", i, got[i].Modifiers, want[i].Modifiers)
		}
	}
}

func TestWriteInventoryCSVSortsGridRefs(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "K2"},
		{GridRef: "A10"},
		{GridRef: "A2"},
	}

	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, rows); err != nil {
		t.Fatalf("WriteInventoryCSV: %v", err)
	}

	got, err := ReadTagsCSV(&buf)
	if err != nil {
		t.Fatalf("ReadTagsCSV: %v", err)
	}

	want := []string{"A2", "A10", "K2"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].GridRef != w {
			t.Errorf("row %d grid_ref = %q, want %q (full: %+v)", i, got[i].GridRef, w, got)
		}
	}
}

func TestReadTagsCSVMalformedHeader(t *testing.T) {
	_, err := ReadTagsCSV(strings.NewReader("foo,bar\nA1,crimp,\n"))
	if err == nil {
		t.Fatal("ReadTagsCSV with bad header = nil error, want error")
	}
}
