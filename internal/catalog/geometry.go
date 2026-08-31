package catalog

// boardGeometry models the linear map from a (col, row) grid cell to a
// percentage position on the board image: x% = originX + col*pitchX,
// y% = originY + (18-row)*pitchY.
type boardGeometry struct {
	originX, pitchX, originY, pitchY float64
}

// geometryByYear holds per-image overlay constants. These are PLACEHOLDER
// estimates — Phase 6 calibrates them against the rendered result.
var geometryByYear = map[string]boardGeometry{
	"2016": {originX: 14, pitchX: 7.7, originY: 8.5, pitchY: 4.97},
	"2024": {originX: 14, pitchX: 7.7, originY: 8.5, pitchY: 4.97},
}

// HoldXY returns the percentage position of a hold's centre on the board
// image for year. col is 0-based (A=0); row is the 1-based MoonBoard row
// (1 at the bottom, 18 at the top). Unknown years fall back to the 2016 map.
func HoldXY(year string, col, row int) (x, y float64) {
	g, ok := geometryByYear[year]
	if !ok {
		g = geometryByYear["2016"]
	}
	return g.originX + float64(col)*g.pitchX, g.originY + float64(18-row)*g.pitchY
}
