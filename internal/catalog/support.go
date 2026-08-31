package catalog

import "sort"

// boardNames mirrors migrations/0001_board_editions.up.sql so app-ready-board
// helpers stay DB-free (they run in form loaders on every request).
var boardNames = map[int16]string{
	1:  "2016",
	15: "Masters 2017",
	17: "Masters 2019",
	21: "2024",
}

// supportedBoardImage maps an app-ready board's holdsetup to the bare year of
// its shipped image (static/moonboard/<year>.jpg). A board is app-ready only
// when its image ships AND every hold is tagged — currently 2016 and 2024.
var supportedBoardImage = map[int16]string{
	1:  "2016",
	21: "2024",
}

// SupportedBoards returns the app-ready boards, ascending by holdsetup, for
// the onboarding + profile-edit dropdowns.
func SupportedBoards() []BoardEdition {
	out := make([]BoardEdition, 0, len(supportedBoardImage))
	for hs := range supportedBoardImage {
		out = append(out, BoardEdition{Holdsetup: hs, Name: boardNames[hs]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Holdsetup < out[j].Holdsetup })
	return out
}

// BoardImageYear returns the image year for an app-ready board, or ok=false
// for a board the app can't run a session on.
func BoardImageYear(holdsetup int16) (string, bool) {
	year, ok := supportedBoardImage[holdsetup]
	return year, ok
}

// BoardName returns the human label for a holdsetup code.
func BoardName(holdsetup int16) (string, bool) {
	name, ok := boardNames[holdsetup]
	return name, ok
}
