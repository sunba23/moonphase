package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/internal/recommender"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

func seedUserRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO auth.users (id) VALUES (gen_random_uuid()) RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedFirstProblem(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (problemID, configID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES (901, 1, 'Owned Problem', '') RETURNING id
	`).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade)
		VALUES ($1, 1, 901, 40, '6B') RETURNING id
	`, problemID).Scan(&configID); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
	return problemID, configID
}

// seedCandidate inserts a problem + config on (1, 40) at grade with a
// problem_hold_types row, so PickNext has a pool to score.
func seedCandidate(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ext int, grade, dominant string) (problemID, configID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, $2, '') RETURNING id
	`, ext, fmt.Sprintf("Cand %d", ext)).Scan(&problemID); err != nil {
		t.Fatalf("seed candidate problem: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade, is_benchmark, repeats)
		VALUES ($1, 1, $2, 40, $3, true, 20) RETURNING id
	`, problemID, ext, grade).Scan(&configID); err != nil {
		t.Fatalf("seed candidate config: %v", err)
	}
	var dom any
	if dominant != "" {
		dom = dominant
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO problem_hold_types (problem_id, total_scored, dominant) VALUES ($1, 4, $2)
	`, problemID, dom); err != nil {
		t.Fatalf("seed candidate hold types: %v", err)
	}
	return problemID, configID
}

func TestHandleResult(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	sp := newSessionPages(pool, profile.NewStore(pool), store, recommender.New(pool), &logger)

	owner := seedUserRow(ctx, t, pool)
	other := seedUserRow(ctx, t, pool)

	p0, c0 := seedCandidate(ctx, t, pool, 800, "6B", "crimp")
	seedCandidate(ctx, t, pool, 801, "6B", "sloper")
	seedCandidate(ctx, t, pool, 802, "6B+", "jug")

	started, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	router := chi.NewRouter()
	router.Post("/session/{sessionID}/result", sp.handleResult)
	router.Post("/session/{sessionID}/end", sp.handleEnd)

	post := func(userID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(auth.WithUserID(ctx, userID), http.MethodPost, "/session/"+started.ID+"/result", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if got := post(other, "seq=0&rpe=5&completion=sent").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner: expected 404, got %d", got)
	}
	for _, bad := range []string{"seq=0&rpe=0&completion=sent", "seq=0&rpe=11&completion=sent", "seq=0&rpe=5&completion=lol", "rpe=5&completion=sent"} {
		if got := post(owner, bad).Code; got != http.StatusUnprocessableEntity {
			t.Fatalf("%q: expected 422, got %d", bad, got)
		}
	}

	rec := post(owner, "seq=0&rpe=3&completion=sent")
	if rec.Code != http.StatusOK {
		t.Fatalf("happy path: expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.HasPrefix(strings.TrimSpace(body), `<div id="session-card"`) {
		t.Fatalf("body is not a bare session-card fragment: %.80s", body)
	}

	// The old row is rated and a seq-1 row now exists.
	shown, _ := store.ShownProblems(ctx, started.ID)
	if len(shown) != 2 || shown[0].RPE == nil {
		t.Fatalf("after result shown = %+v", shown)
	}

	// Re-submitting the now-rated seq 0 is a 409.
	if got := post(owner, "seq=0&rpe=3&completion=sent").Code; got != http.StatusConflict {
		t.Fatalf("stale seq: expected 409, got %d", got)
	}

	// Rate seq 1, then end; two rated rows + ended status.
	if got := post(owner, "seq=1&rpe=4&completion=sent").Code; got != http.StatusOK {
		t.Fatalf("second result: expected 200, got %d", got)
	}
	endReq := httptest.NewRequestWithContext(auth.WithUserID(ctx, owner), http.MethodPost, "/session/"+started.ID+"/end", nil)
	endRec := httptest.NewRecorder()
	router.ServeHTTP(endRec, endReq)
	if endRec.Code != http.StatusOK || endRec.Header().Get("HX-Redirect") != "/" {
		t.Fatalf("end: code %d redirect %q", endRec.Code, endRec.Header().Get("HX-Redirect"))
	}

	var status string
	var rated int
	_ = pool.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1`, started.ID).Scan(&status)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM session_problems WHERE session_id = $1 AND rpe IS NOT NULL`, started.ID).Scan(&rated)
	if status != "ended" || rated != 2 {
		t.Fatalf("post-end: status %q rated %d, want ended/2", status, rated)
	}
}

func TestHandleView_ShowsLatestProblem(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	sp := newSessionPages(pool, profile.NewStore(pool), store, recommender.New(pool), &logger)

	owner := seedUserRow(ctx, t, pool)
	p0, c0 := seedCandidate(ctx, t, pool, 810, "6B", "crimp")
	p1, c1 := seedCandidate(ctx, t, pool, 811, "6B+", "jug")

	started, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := store.AdvanceSession(ctx, started.ID, 0, 5, session.CompletionSent,
		session.SessionProblem{Seq: 1, ProblemID: p1, ConfigurationID: c1}); err != nil {
		t.Fatalf("AdvanceSession: %v", err)
	}

	router := chi.NewRouter()
	router.Get("/session/{sessionID}", sp.handleView)
	req := httptest.NewRequestWithContext(auth.WithUserID(ctx, owner), http.MethodGet, "/session/"+started.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Cand 811") {
		t.Fatalf("handleView body did not show the latest problem: code %d", rec.Code)
	}
}

func TestHandleView_OwnershipIs404ForNonOwner(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()

	sp := newSessionPages(pool, profile.NewStore(pool), session.NewStore(pool), recommender.New(pool), &logger)

	owner := seedUserRow(ctx, t, pool)
	other := seedUserRow(ctx, t, pool)
	problemID, configID := seedFirstProblem(ctx, t, pool)

	started, err := session.NewStore(pool).StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: problemID, ConfigurationID: configID})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	router := chi.NewRouter()
	router.Get("/session/{sessionID}", sp.handleView)

	do := func(userID string) int {
		req := httptest.NewRequestWithContext(auth.WithUserID(ctx, userID), http.MethodGet, "/session/"+started.ID, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := do(owner); got != http.StatusOK {
		t.Fatalf("owner GET: expected 200, got %d", got)
	}
	if got := do(other); got != http.StatusNotFound {
		t.Fatalf("non-owner GET: expected 404, got %d", got)
	}
}
