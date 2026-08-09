package catalog

import (
	"bytes"
	"strings"
	"testing"
)

// keySeq builds a fake keystroke stream from single-character tokens, e.g.
// keySeq("1", "1", "2") for grid ref A1's primary type "1" then modifiers
// "1", "2".
func keySeq(keys ...string) *strings.Reader {
	return strings.NewReader(strings.Join(keys, ""))
}

func TestRunTagLoopFullPass(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "A2"},
		{GridRef: "A1"},
	}

	// A1: primary=1(crimp), modifiers=1(sharp),2(rounded)
	// A2: primary=3(pinch), modifiers=<none, non-digit key>
	keys := keySeq("1", "1", "2", "3", "x")

	var log bytes.Buffer
	saved := 0
	save := func([]HoldRow) error {
		saved++
		return nil
	}

	got, err := runTagLoop(rows, keys, &log, save)
	if err != nil {
		t.Fatalf("runTagLoop: %v", err)
	}

	want := map[string]HoldRow{
		"A1": {GridRef: "A1", PrimaryType: "crimp", Modifiers: []string{"sharp", "rounded"}},
		"A2": {GridRef: "A2", PrimaryType: "pinch"},
	}
	for _, row := range got {
		w := want[row.GridRef]
		if row.PrimaryType != w.PrimaryType {
			t.Errorf("%s: PrimaryType = %q, want %q", row.GridRef, row.PrimaryType, w.PrimaryType)
		}
		if strings.Join(row.Modifiers, ";") != strings.Join(w.Modifiers, ";") {
			t.Errorf("%s: Modifiers = %v, want %v", row.GridRef, row.Modifiers, w.Modifiers)
		}
	}
	if saved != 2 {
		t.Errorf("save called %d times, want 2 (once per hold)", saved)
	}
}

func TestRunTagLoopModifierCapEnforced(t *testing.T) {
	rows := []HoldRow{{GridRef: "A1"}}

	// primary=1(crimp), then 4 modifier digit keys in a row -- only the
	// first 2 should be kept, the loop must stop reading after 2.
	keys := keySeq("1", "1", "2", "3", "4")

	got, err := runTagLoop(rows, keys, &bytes.Buffer{}, func([]HoldRow) error { return nil })
	if err != nil {
		t.Fatalf("runTagLoop: %v", err)
	}

	if len(got[0].Modifiers) != 2 {
		t.Fatalf("Modifiers = %v, want exactly 2", got[0].Modifiers)
	}
	if got[0].Modifiers[0] != "sharp" || got[0].Modifiers[1] != "rounded" {
		t.Errorf("Modifiers = %v, want [sharp rounded]", got[0].Modifiers)
	}
}

func TestRunTagLoopQuitMidPass(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "A1"},
		{GridRef: "A2"},
		{GridRef: "A3"},
	}

	// A1 gets fully tagged, then quit before A2 is touched.
	keys := keySeq("1", "x", "q")

	var savedRows []HoldRow
	save := func(current []HoldRow) error {
		savedRows = append([]HoldRow(nil), current...)
		return nil
	}

	got, err := runTagLoop(rows, keys, &bytes.Buffer{}, save)
	if err != nil {
		t.Fatalf("runTagLoop: %v", err)
	}

	if got[0].GridRef != "A1" || got[0].PrimaryType != "crimp" {
		t.Errorf("A1 = %+v, want tagged crimp", got[0])
	}
	if got[1].PrimaryType != "" || got[2].PrimaryType != "" {
		t.Errorf("A2/A3 should remain untagged after quit, got %+v / %+v", got[1], got[2])
	}
	if len(savedRows) == 0 {
		t.Fatal("save was never called; A1's tag would be lost")
	}
}

func TestRunTagLoopSkipsAlreadyTagged(t *testing.T) {
	rows := []HoldRow{
		{GridRef: "A1", PrimaryType: "jug"},
		{GridRef: "A2"},
	}

	// Only one primary-type keystroke provided -- if A1 were (wrongly)
	// prompted, this would either error on EOF for A2 or misassign it.
	keys := keySeq("3", "x")

	got, err := runTagLoop(rows, keys, &bytes.Buffer{}, func([]HoldRow) error { return nil })
	if err != nil {
		t.Fatalf("runTagLoop: %v", err)
	}

	if got[0].PrimaryType != "jug" {
		t.Errorf("A1 PrimaryType = %q, want unchanged %q", got[0].PrimaryType, "jug")
	}
	if got[1].PrimaryType != "pinch" {
		t.Errorf("A2 PrimaryType = %q, want %q", got[1].PrimaryType, "pinch")
	}
}
