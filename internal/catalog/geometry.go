package catalog

// boardGeometry models the linear map from a (col, row) grid cell to a
// percentage position on the board image: x% = originX + col*pitchX,
// y% = originY + (18-row)*pitchY.
type boardGeometry struct {
	originX, pitchX, originY, pitchY float64
}

// geometryByYear holds per-image overlay constants, calibrated against the
// shipped 600x923 board images (static/moonboard/<year>.jpg) by overlaying a
// full A1..K18 grid and eyeballing the fit. Both images share the same grid
// geometry: column A centre ~10.3%, column pitch ~7.84%; row 18 centre
// ~7.8%, row pitch ~4.98% (row 18 at the top, row 1 at the bottom).
var geometryByYear = map[string]boardGeometry{
	"2016": {originX: 10.3, pitchX: 7.84, originY: 7.8, pitchY: 4.98},
	"2024": {originX: 10.3, pitchX: 7.84, originY: 7.8, pitchY: 4.98},
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
