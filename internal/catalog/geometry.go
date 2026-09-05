package catalog

// boardGeometry models the linear map from a (col, row) grid cell to a
// percentage position on the board image: x% = originX + col*pitchX,
// y% = originY + (18-row)*pitchY.
type boardGeometry struct {
	originX, pitchX, originY, pitchY float64
}

// geometryByYear holds per-image overlay constants. Both shipped board images
// (static/moonboard/<year>.jpg, 600x923) show the same physical MoonBoard —
// the same 11x18 grid, only different resin holds — so one calibration serves
// both. Fitted (least squares) to four measured reference holds on the 600x923
// image: A18 (85,79), K18 (523,80), A1 (78,866), K1 (521,866), i.e. as
// percentages A18 (14.17,8.56), K18 (87.17,8.67), A1 (13.00,93.82),
// K1 (86.83,93.82). Row 18 is at the top, row 1 at the bottom. The per-year
// map is kept so a future edition could diverge.
var geometryByYear = map[string]boardGeometry{
	"2016": {originX: 13.6, pitchX: 7.34, originY: 8.6, pitchY: 5.01},
	"2024": {originX: 13.6, pitchX: 7.34, originY: 8.6, pitchY: 5.01},
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
