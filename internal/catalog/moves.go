package catalog

import (
	"fmt"
	"strings"
)

// Move is one typed, ordered token parsed out of a problem's raw moves
// string, e.g. "s~C5~" -> {Seq: 0, Type: "s", GridRef: "C5"}.
type Move struct {
	Seq     int
	Type    string
	GridRef string
}

// ParseMoves splits a pipe-delimited moves string into ordered tokens. Seq is
// the 0-based index in source order, not an inferred climbing-path order —
// the source data can contain two "s~" tokens for a two-hand start.
func ParseMoves(raw string) ([]Move, error) {
	tokens := strings.Split(raw, "|")
	moves := make([]Move, 0, len(tokens))

	seq := 0
	for _, tok := range tokens {
		if tok == "" {
			continue
		}

		parts := strings.Split(tok, "~")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != "" {
			return nil, fmt.Errorf("catalog: malformed move token %q", tok)
		}

		moves = append(moves, Move{Seq: seq, Type: parts[0], GridRef: parts[1]})
		seq++
	}

	return moves, nil
}
