package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// BoardYears maps the 4 seeded board_editions.holdsetup codes to the bare
// year that uniquely identifies each board (2016, Masters 2017, Masters
// 2019, 2024 all carry distinct years, so no separate slug scheme is
// needed). Mirrors KnownBoards and migrations/0001_board_editions.up.sql.
var BoardYears = map[int]string{
	1:  "2016",
	15: "2017",
	17: "2019",
	21: "2024",
}

// ResolveBoardYear reverse-looks-up a board year (as typed on the CLI) into
// its holdsetup code. This is the external identity every human-facing
// surface (CLI flags, CSV filenames) should use instead of the raw
// holdsetup integer.
func ResolveBoardYear(year string) (int, error) {
	for holdsetup, y := range BoardYears {
		if y == year {
			return holdsetup, nil
		}
	}

	years := make([]string, 0, len(BoardYears))
	for _, y := range BoardYears {
		years = append(years, y)
	}
	sort.Strings(years)

	return 0, fmt.Errorf("catalog: unknown board year %q, expected one of: %s", year, strings.Join(years, ", "))
}
