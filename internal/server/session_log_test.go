package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/internal/recommender"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

// recPickLines returns every rec_pick log object written to buf, in order.
func recPickLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON (%v): %s", err, line)
		}
		if m["message"] == "rec_pick" {
			out = append(out, m)
		}
	}
	return out
}

// TestRecPickLog pins the rec_pick decision line for all four pick call paths:
// handleStart (first_pick), handleSkip with no anchor (first_pick), handleResult
// (adaptive), and handleSkip with an anchor (adaptive).
func TestRecPickLog(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)

	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Timestamp().Logger()
	store := session.NewStore(pool)
	sp := newSessionPages(pool, profile.NewStore(pool), store, recommender.New(pool), &logger)

	owner := seedUserRow(ctx, t, pool)
	if err := profile.NewStore(pool).Upsert(ctx, profile.Profile{
		UserID: owner, MaxGrade: "7A", Holdsetup: 1, Angle: 40,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	ext := 900
	next := func() int { ext++; return ext }
	for _, d := range []string{"crimp", "sloper", "jug", "pinch", "pocket"} {
		seedCandidate(ctx, t, pool, next(), "6B", d)
	}
	seedCandidate(ctx, t, pool, next(), "6B+", "sloper")
	seedCandidate(ctx, t, pool, next(), "6B+", "jug")

	router := chi.NewRouter()
	router.Post("/session", sp.handleStart)
	router.Post("/session/{sessionID}/result", sp.handleResult)
	router.Post("/session/{sessionID}/skip", sp.handleSkip)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(auth.WithUserID(ctx, owner), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	field := func(m map[string]any, k string) any {
		t.Helper()
		v, ok := m[k]
		if !ok {
			t.Fatalf("rec_pick line missing %q: %v", k, m)
		}
		return v
	}

	// 1. handleStart -> first_pick, seq 0.
	rr := do(http.MethodPost, "/session", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("start: code %d", rr.Code)
	}
	sessionID := strings.TrimPrefix(rr.Header().Get("HX-Redirect"), "/session/")
	if sessionID == "" {
		t.Fatalf("no session id in HX-Redirect %q", rr.Header().Get("HX-Redirect"))
	}
	lines := recPickLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("after start: %d rec_pick lines, want 1", len(lines))
	}
	if field(lines[0], "kind") != "first_pick" || field(lines[0], "session_id") != sessionID || field(lines[0], "seq").(float64) != 0 {
		t.Fatalf("start line = %v", lines[0])
	}
	if _, ok := lines[0]["pool_size"]; !ok {
		t.Fatalf("start line missing pool_size: %v", lines[0])
	}

	// 2. handleSkip with nothing rated -> first_pick, seq 1.
	buf.Reset()
	if rr := do(http.MethodPost, "/session/"+sessionID+"/skip", "seq=0"); rr.Code != http.StatusOK {
		t.Fatalf("skip seq 0: code %d", rr.Code)
	}
	lines = recPickLines(t, &buf)
	if len(lines) != 1 || field(lines[0], "kind") != "first_pick" || field(lines[0], "seq").(float64) != 1 {
		t.Fatalf("skip-no-anchor line = %v", lines)
	}

	// 3. handleResult -> adaptive, seq 2. Easy send from 6B steps up.
	buf.Reset()
	if rr := do(http.MethodPost, "/session/"+sessionID+"/result", "seq=1&rpe=3&completion=sent"); rr.Code != http.StatusOK {
		t.Fatalf("result seq 1: code %d", rr.Code)
	}
	lines = recPickLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("after result: %d rec_pick lines, want 1", len(lines))
	}
	m := lines[0]
	if field(m, "kind") != "adaptive" || field(m, "session_id") != sessionID || field(m, "seq").(float64) != 2 {
		t.Fatalf("result line identity = %v", m)
	}
	if field(m, "rpe").(float64) != 3 || field(m, "completion") != "sent" || field(m, "band") != "step_up" {
		t.Fatalf("result line input/decision = %v", m)
	}
	for _, k := range []string{"grade_lo", "grade_hi", "pref_grade", "fallback_tier", "tie_set_size", "pick_problem_id", "pick_grade"} {
		field(m, k)
	}
	if field(m, "pick_problem_id").(float64) == 0 {
		t.Fatalf("result line pick_problem_id is zero: %v", m)
	}

	// 4. handleSkip with an anchor -> adaptive, seq 3.
	buf.Reset()
	if rr := do(http.MethodPost, "/session/"+sessionID+"/skip", "seq=2"); rr.Code != http.StatusOK {
		t.Fatalf("skip seq 2: code %d", rr.Code)
	}
	lines = recPickLines(t, &buf)
	if len(lines) != 1 || field(lines[0], "kind") != "adaptive" || field(lines[0], "seq").(float64) != 3 {
		t.Fatalf("skip-anchored line = %v", lines)
	}
}
