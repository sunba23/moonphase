package catalog

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeometryCheck writes a self-contained geometry-check.html into the
// package directory: both shipped boards with all 198 grid positions
// (A1..K18) circled via the real HoldXY map and a faithful copy of the
// production .board / .hold CSS. It is a visual calibration aid, not an
// assertion — skipped unless GEOMETRY_CHECK is set:
//
//	GEOMETRY_CHECK=1 go test ./internal/catalog -run TestGeometryCheck -v
//
// Open the logged file in a browser and confirm every circle sits centred on
// its resin hold on both holdsets.
func TestGeometryCheck(t *testing.T) {
	if os.Getenv("GEOMETRY_CHECK") == "" {
		t.Skip("set GEOMETRY_CHECK=1 to (re)generate geometry-check.html")
	}

	// Border colour per role, cycled so every production .hold--* colour shows.
	// Mirrors static/app.css: start / finish / hand (--accent) / foot (--ink-soft).
	roleColours := [4]string{"#1f6f43", "#7b2f8f", "#2e4b8f", "#6b5d49"}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<meta charset="utf-8">
<title>MoonPhase board-overlay geometry check</title>
<style>
  body{font:14px system-ui,sans-serif;margin:1.5rem;background:#e9dcc3;color:#2b2115}
  figure{margin:0 0 2rem}
  figcaption{margin-bottom:.5rem;font-weight:600}
  .board{position:relative;max-width:440px;margin:0 auto}
  .board img{width:100%;display:block;border-radius:8px}
  .hold{position:absolute;width:7%;aspect-ratio:1;border:3px solid;
        border-radius:50%;transform:translate(-50%,-50%)}
</style>
`)

	for _, year := range []string{"2016", "2024"} {
		imgPath := filepath.Join("..", "..", "static", "moonboard", year+".jpg")
		raw, err := os.ReadFile(imgPath)
		if err != nil {
			t.Fatalf("read board image %s: %v", imgPath, err)
		}
		dataURI := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(raw)

		fmt.Fprintf(&b, "<figure>\n<figcaption>MoonBoard %s — all 198 grid positions</figcaption>\n<div class=\"board\">\n<img alt=\"MoonBoard %s\" src=\"%s\">\n", year, year, dataURI)

		for col := 0; col <= 10; col++ {
			for row := 1; row <= 18; row++ {
				x, y := HoldXY(year, col, row)
				colour := roleColours[(col+row)%4]
				ref := fmt.Sprintf("%c%d", 'A'+col, row)
				fmt.Fprintf(&b,
					"<span class=\"hold\" title=\"%s\" style=\"left:%.2f%%;top:%.2f%%;border-color:%s\"></span>\n",
					ref, x, y, colour)
			}
		}
		b.WriteString("</div>\n</figure>\n")
	}

	out := "geometry-check.html"
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		abs = out
	}
	t.Logf("wrote %s — open it in a browser to check the overlay fit", abs)
}
