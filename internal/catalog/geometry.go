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
// both. Fitted to the four corner hold mounts measured on the 2016 image as an
// image-map polygon "88.0,82.0,549.5,81.0,85.5,865.0,549.0,866.5", i.e. as
// percentages of 600x923: A18 (14.667,8.884), K18 (91.583,8.776),
// A1 (14.250,93.716), K1 (91.500,93.878). originX/originY are the column-A /
// row-18 edge means; pitchX/pitchY divide the span by 10 columns / 17 rows.
// Row 18 is at the top, row 1 at the bottom. The per-year map is kept so a
// future edition could diverge.
var geometryByYear = map[string]boardGeometry{
	"2016": {originX: 14.46, pitchX: 7.71, originY: 8.83, pitchY: 5.00},
	"2024": {originX: 14.46, pitchX: 7.71, originY: 8.83, pitchY: 5.00},
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
