package server

import (
	"context"
	"net/http"
	"net/http/httptest"
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
