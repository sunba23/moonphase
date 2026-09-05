package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

var historyExt = 1000

func nextExt() int {
	historyExt++
	return historyExt
}

// seedEndedSession builds an ended session for userID with one rated problem
// per rpe in rpes (climb order), plus the trailing unrated recommended row.
func seedEndedSession(ctx context.Context, t *testing.T, store *session.Store, pool *pgxpool.Pool, userID string, rpes ...int16) string {
	t.Helper()
	p0, c0 := seedCandidate(ctx, t, pool, nextExt(), "6B", "crimp")
	started, err := store.StartSession(ctx, session.Session{
		UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: p0, ConfigurationID: c0})
	if err != nil {
		t.Fatalf("seed session start: %v", err)
	}
	for i, rpe := range rpes {
		pn, cn := seedCandidate(ctx, t, pool, nextExt(), "6B", "crimp")
		if err := store.AdvanceSession(ctx, started.ID, i, rpe, session.CompletionSent,
			session.SessionProblem{Seq: i + 1, ProblemID: pn, ConfigurationID: cn}); err != nil {
			t.Fatalf("seed session advance %d: %v", i, err)
		}
	}
	if err := store.EndSession(ctx, started.ID, userID); err != nil {
		t.Fatalf("seed session end: %v", err)
	}
	return started.ID
}

func historyRouter(hsp *historyPages) *chi.Mux {
	router := chi.NewRouter()
	router.Get("/sessions", hsp.handleList)
	router.Get("/sessions/{sessionID}", hsp.handleDetail)
	router.Get("/sessions/{sessionID}/problem/{seq}", hsp.handleProblem)
	return router
}

func getAs(ctx context.Context, router *chi.Mux, userID, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(auth.WithUserID(ctx, userID), http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHistoryList_ScopedToCallerAndEndedOnly(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	router := historyRouter(newHistoryPages(pool, store, &logger))

	owner := seedUserRow(ctx, t, pool)
	other := seedUserRow(ctx, t, pool)

	ended := seedEndedSession(ctx, t, store, pool, owner, 4, 6)
	seedEndedSession(ctx, t, store, pool, other, 5)

	// Owner also has a live session — it must not appear in the list.
	pa, ca := seedCandidate(ctx, t, pool, nextExt(), "6B", "crimp")
	if _, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: pa, ConfigurationID: ca}); err != nil {
		t.Fatalf("start active: %v", err)
	}

	rec := getAs(ctx, router, owner, "/sessions")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: code %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/sessions/"+ended) {
		t.Fatalf("list body missing owner's ended session link:\n%s", body)
	}
	if !strings.Contains(body, "2 problems climbed") {
		t.Fatalf("list body missing climbed count:\n%s", body)
	}
	if strings.Count(body, "<li>") != 1 {
		t.Fatalf("list body should show exactly one session row:\n%s", body)
	}

	// A fresh user sees the empty state.
	fresh := seedUserRow(ctx, t, pool)
	emptyRec := getAs(ctx, router, fresh, "/sessions")
	if emptyRec.Code != http.StatusOK || !strings.Contains(emptyRec.Body.String(), "No sessions yet") {
		t.Fatalf("empty state: code %d body %q", emptyRec.Code, emptyRec.Body.String())
	}
}

func TestHistoryDetail_404Discipline(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	router := historyRouter(newHistoryPages(pool, store, &logger))

	owner := seedUserRow(ctx, t, pool)
	other := seedUserRow(ctx, t, pool)

	ended := seedEndedSession(ctx, t, store, pool, owner, 4, 6)

	pa, ca := seedCandidate(ctx, t, pool, nextExt(), "6B", "crimp")
	active, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: pa, ConfigurationID: ca})
	if err != nil {
		t.Fatalf("start active: %v", err)
	}

	cases := []struct {
		name, user, target string
		want               int
	}{
		{"owner ended", owner, "/sessions/" + ended, http.StatusOK},
		{"non-owner ended", other, "/sessions/" + ended, http.StatusNotFound},
		{"owner active", owner, "/sessions/" + active.ID, http.StatusNotFound},
		{"random uuid", owner, "/sessions/00000000-0000-0000-0000-000000000000", http.StatusNotFound},
	}
	for _, c := range cases {
		if got := getAs(ctx, router, c.user, c.target).Code; got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestHistoryDetail_RendersClimbedProblems(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	router := historyRouter(newHistoryPages(pool, store, &logger))

	owner := seedUserRow(ctx, t, pool)
	ended := seedEndedSession(ctx, t, store, pool, owner, 4, 8)

	rec := getAs(ctx, router, owner, "/sessions/"+ended)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: code %d", rec.Code)
	}
	body := rec.Body.String()
	// Two rated rows, not the trailing unrated one.
	if n := strings.Count(body, "/problem/"); n != 2 {
		t.Fatalf("detail should link exactly 2 climbed problems, got %d:\n%s", n, body)
	}
	if !strings.Contains(body, "RPE 4") || !strings.Contains(body, "RPE 8") {
		t.Fatalf("detail body missing recorded RPE values:\n%s", body)
	}

	// A session ended before any rating shows the detail empty state.
	empty := seedEndedSession(ctx, t, store, pool, owner)
	emptyRec := getAs(ctx, router, owner, "/sessions/"+empty)
	if emptyRec.Code != http.StatusOK || !strings.Contains(emptyRec.Body.String(), "No problems climbed in this session") {
		t.Fatalf("detail empty state: code %d body %q", emptyRec.Code, emptyRec.Body.String())
	}
}

func TestHistoryProblem_404Discipline(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	router := historyRouter(newHistoryPages(pool, store, &logger))

	owner := seedUserRow(ctx, t, pool)
	other := seedUserRow(ctx, t, pool)
	ended := seedEndedSession(ctx, t, store, pool, owner, 4, 6) // rated seq 0,1; unrated trailing seq 2

	cases := []struct {
		name, user, target string
		want               int
	}{
		{"owner rated seq", owner, "/sessions/" + ended + "/problem/0", http.StatusOK},
		{"non-owner", other, "/sessions/" + ended + "/problem/0", http.StatusNotFound},
		{"unrated trailing seq", owner, "/sessions/" + ended + "/problem/2", http.StatusNotFound},
		{"out-of-range seq", owner, "/sessions/" + ended + "/problem/99", http.StatusNotFound},
		{"non-numeric seq", owner, "/sessions/" + ended + "/problem/abc", http.StatusNotFound},
	}
	for _, c := range cases {
		if got := getAs(ctx, router, c.user, c.target).Code; got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestHistoryProblem_RendersRecordedResult(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()
	store := session.NewStore(pool)
	router := historyRouter(newHistoryPages(pool, store, &logger))

	owner := seedUserRow(ctx, t, pool)
	ended := seedEndedSession(ctx, t, store, pool, owner, 7)

	rec := getAs(ctx, router, owner, "/sessions/"+ended+"/problem/0")
	if rec.Code != http.StatusOK {
		t.Fatalf("problem: code %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "You climbed this") || !strings.Contains(body, "RPE 7") || !strings.Contains(body, "sent") {
		t.Fatalf("problem body missing recorded result line:\n%s", body)
	}
	// Read-only: no result form, no End button.
	if strings.Contains(body, "/result") || strings.Contains(body, "End session") {
		t.Fatalf("problem card should be read-only (no result form / End button):\n%s", body)
	}
}
