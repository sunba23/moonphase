package server

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/sunba23/moonphase/templates/layout"
	"github.com/sunba23/moonphase/templates/pages"
)

func renderToString(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestFooter_ShowsLinkedShortSHAWhenVersionInContext(t *testing.T) {
	const full = "abc1234def5678901234567890123456789012ab"
	ctx := layout.WithVersion(context.Background(), full)

	html := renderToString(t, ctx, layout.Page("Sign in", templ.NopComponent))

	if !strings.Contains(html, "<footer") {
		t.Fatalf("expected a <footer> in the page, got:\n%s", html)
	}
	if !strings.Contains(html, ">abc1234</a>") {
		t.Errorf("expected the link text to be the 7-char short SHA, got:\n%s", html)
	}
	if want := `href="https://github.com/sunba23/moonphase/commit/` + full + `"`; !strings.Contains(html, want) {
		t.Errorf("expected the commit link %q in the footer", want)
	}
	if strings.Contains(html, ">"+full+"<") {
		t.Errorf("footer link text should be the short SHA, not the full one")
	}
}

func TestFooter_ShowsPlainDevWithNoVersionInContext(t *testing.T) {
	html := renderToString(t, context.Background(), layout.Page("Sign in", templ.NopComponent))

	if !strings.Contains(html, "<footer") {
		t.Fatalf("expected a <footer> in the page")
	}
	if !strings.Contains(html, ">dev<") {
		t.Errorf("expected a plain \"dev\" footer, got:\n%s", html)
	}
	if strings.Contains(html, "sitefooter__link") {
		t.Errorf("dev footer must not be a link")
	}
	if strings.Contains(html, "/commit/") {
		t.Errorf("dev footer must carry no commit link")
	}
}

func TestFooter_AppPageAlsoCarriesIt(t *testing.T) {
	ctx := layout.WithVersion(context.Background(), "abc1234def5678901234567890123456789012ab")
	html := renderToString(t, ctx, layout.AppPage("Session", layout.NavModel{BoardName: "2016", Angle: 40, MaxGrade: "7A"}, templ.NopComponent))

	if !strings.Contains(html, "<footer") || !strings.Contains(html, "abc1234") {
		t.Fatalf("AppPage should render the version footer, got:\n%s", html)
	}
}

func TestFooter_AbsentFromSessionCardFragment(t *testing.T) {
	// renderNextCard renders pages.SessionCard directly (no shell) for the HTMX
	// swap, so a footer added to the shells must not appear here.
	ctx := layout.WithVersion(context.Background(), "abc1234def5678901234567890123456789012ab")
	html := renderToString(t, ctx, pages.SessionCard(pages.SessionCardModel{}))

	if strings.Contains(html, "<footer") {
		t.Errorf("the #session-card fragment must not contain a <footer>")
	}
}
