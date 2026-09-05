package catalog

import (
	"math"
	"testing"
)

// TestHoldXY_ReferencePoints pins the overlay calibration to the four measured
// reference holds on the 600x923 board image (see geometry.go). col is 0-based
// (A=0), row is 1-based (1 bottom, 18 top).
func TestHoldXY_ReferencePoints(t *testing.T) {
	const tol = 1.0 // percentage points

	cases := []struct {
		name     string
		col, row int
		wantX    float64
		wantY    float64
	}{
		{"A18 top-left", 0, 18, 14.17, 8.56},
		{"K18 top-right", 10, 18, 87.17, 8.67},
		{"A1 bottom-left", 0, 1, 13.00, 93.82},
		{"K1 bottom-right", 10, 1, 86.83, 93.82},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y := HoldXY("2016", tc.col, tc.row)
			if math.Abs(x-tc.wantX) > tol || math.Abs(y-tc.wantY) > tol {
				t.Errorf("HoldXY(2016, %d, %d) = (%.2f, %.2f), want ~(%.2f, %.2f) ±%.1f",
					tc.col, tc.row, x, y, tc.wantX, tc.wantY, tol)
			}
		})
	}
}

func TestHoldXY_InteriorSanity(t *testing.T) {
	// F9 (col 5, row 9) sits near the middle of the board.
	x, y := HoldXY("2016", 5, 9)
	if x < 45 || x > 55 || y < 48 || y > 58 {
		t.Errorf("HoldXY(2016, 5, 9) = (%.2f, %.2f), want roughly board-centre", x, y)
	}
}

func TestHoldXY_SharedGeometryAcrossEditions(t *testing.T) {
	for _, tc := range []struct{ col, row int }{{0, 1}, {10, 18}, {5, 9}, {3, 14}} {
		x16, y16 := HoldXY("2016", tc.col, tc.row)
		x24, y24 := HoldXY("2024", tc.col, tc.row)
		if x16 != x24 || y16 != y24 {
			t.Errorf("2016 and 2024 geometry diverge at (%d,%d): (%.2f,%.2f) vs (%.2f,%.2f)",
				tc.col, tc.row, x16, y16, x24, y24)
		}
	}
}

func TestHoldXY_UnknownYearFallsBackTo2016(t *testing.T) {
	xWant, yWant := HoldXY("2016", 4, 7)
	xGot, yGot := HoldXY("1999", 4, 7)
	if xGot != xWant || yGot != yWant {
		t.Errorf("HoldXY(1999, …) = (%.2f, %.2f), want 2016 fallback (%.2f, %.2f)", xGot, yGot, xWant, yWant)
	}
}
