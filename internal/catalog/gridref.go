package catalog

import (
	"fmt"
	"strconv"
)

// ParseGridRef parses a "C5"-style board grid reference into its column
// letters and row number. Shape validation is lenient: one or more uppercase
// letters followed by a 1-2 digit number. Out-of-expected-range values
// (letter beyond K, row beyond 18) are not rejected here — real production
// data shouldn't be rejected mid-run on an unexpected edge.
func ParseGridRef(s string) (col string, row int, err error) {
	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	if i == 0 {
		return "", 0, fmt.Errorf("catalog: invalid grid ref %q: no column letters", s)
	}

	rowStr := s[i:]
	if rowStr == "" || len(rowStr) > 2 {
		return "", 0, fmt.Errorf("catalog: invalid grid ref %q: row must be 1-2 digits", s)
	}

	row, err = strconv.Atoi(rowStr)
	if err != nil {
		return "", 0, fmt.Errorf("catalog: invalid grid ref %q: %w", s, err)
	}

	return s[:i], row, nil
}

// Less orders two grid refs by (column, row), e.g. A1 < A2 < ... < A18 < B1.
// Falls back to a plain string comparison for unparseable input.
func Less(a, b string) bool {
	colA, rowA, errA := ParseGridRef(a)
	colB, rowB, errB := ParseGridRef(b)
	if errA != nil || errB != nil {
		return a < b
	}

	if colA != colB {
		return colA < colB
	}

	return rowA < rowB
}
