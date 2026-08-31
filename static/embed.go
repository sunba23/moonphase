// Package static holds the browser assets (CSS, HTMX, board images) embedded
// into the server binary so they ship independently of the container image and
// the process working directory.
package static

import "embed"

//go:embed app.css htmx.min.js moonboard
var FS embed.FS
