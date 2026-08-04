package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// AllowedHoldTypes is the fixed hold-type taxonomy every hold must be tagged
// against.
var AllowedHoldTypes = []string{"crimp", "sloper", "pinch", "jug", "pocket"}

// MaxHoldModifiers is the soft cap on modifiers per hold. Exceeding it is not
// rejected by ReadTagsCSV -- real tagging work shouldn't be blocked mid-pass
// on an unexpected edge -- callers may warn instead.
const MaxHoldModifiers = 3

// ValidateHoldType reports an error unless s is one of AllowedHoldTypes.
func ValidateHoldType(s string) error {
	for _, t := range AllowedHoldTypes {
		if s == t {
			return nil
		}
	}
	return fmt.Errorf("catalog: invalid hold type %q, expected one of %v", s, AllowedHoldTypes)
}

// HoldRow is one physical hold's tagging state, shared by the inventory CSV
// (round-tripped for hand-tagging) and the DB-backed HoldStore.
type HoldRow struct {
	GridRef     string
	PrimaryType string
	Modifiers   []string
}

// WriteInventoryCSV writes rows sorted by grid ref (column then row, not
// lexicographically) with the fixed header grid_ref,primary_type,modifiers.
func WriteInventoryCSV(w io.Writer, rows []HoldRow) error {
	sorted := make([]HoldRow, len(rows))
	copy(sorted, rows)
	sort.Slice(sorted, func(i, j int) bool { return Less(sorted[i].GridRef, sorted[j].GridRef) })

	cw := csv.NewWriter(w)

	if err := cw.Write([]string{"grid_ref", "primary_type", "modifiers"}); err != nil {
		return fmt.Errorf("catalog: write inventory header: %w", err)
	}

	for _, r := range sorted {
		if err := cw.Write([]string{r.GridRef, r.PrimaryType, strings.Join(r.Modifiers, ";")}); err != nil {
			return fmt.Errorf("catalog: write inventory row %q: %w", r.GridRef, err)
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("catalog: flush inventory csv: %w", err)
	}

	return nil
}

// ReadTagsCSV reads a hand-filled (or partially-filled) hold-tags CSV in the
// WriteInventoryCSV format. modifiers is split on ";" and trimmed; a blank
// primary_type is preserved as an untagged row rather than an error, since a
// partially-filled inventory is the expected mid-pass state.
func ReadTagsCSV(r io.Reader) ([]HoldRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("catalog: read tags header: %w", err)
	}
	if len(header) < 2 || header[0] != "grid_ref" || header[1] != "primary_type" {
		return nil, fmt.Errorf("catalog: unexpected tags header %v", header)
	}

	var rows []HoldRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: read tags row: %w", err)
		}
		if len(rec) < 2 {
			return nil, fmt.Errorf("catalog: malformed tags row %v", rec)
		}

		row := HoldRow{GridRef: rec[0], PrimaryType: strings.TrimSpace(rec[1])}
		if len(rec) >= 3 && rec[2] != "" {
			for _, m := range strings.Split(rec[2], ";") {
				m = strings.TrimSpace(m)
				if m != "" {
					row.Modifiers = append(row.Modifiers, m)
				}
			}
		}

		rows = append(rows, row)
	}

	return rows, nil
}
