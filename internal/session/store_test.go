package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

// seedUser inserts an auth.users row and returns its id.
func seedUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO auth.users (id) VALUES (gen_random_uuid()) RETURNING id
	`).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedProblem inserts one problem + one problem_configuration on holdsetup 1
// and returns their ids.
func seedProblem(ctx context.Context, t *testing.T, pool *pgxpool.Pool, grade string) (problemID, configID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO problems (external_id, holdsetup, name, moves_raw)
		VALUES ($1, 1, 'seed', '')
		RETURNING id
	`, mustUniqueInt(t)).Scan(&problemID); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO problem_configurations (problem_id, holdsetup, api_id, angle, grade)
		VALUES ($1, 1, $2, 40, $3)
		RETURNING id
	`, problemID, mustUniqueInt(t), grade).Scan(&configID); err != nil {
		t.Fatalf("seed problem_configuration: %v", err)
	}
	return problemID, configID
}

var uniqueCounter int

func mustUniqueInt(t *testing.T) int {
	t.Helper()
	uniqueCounter++
	return uniqueCounter
}

func TestStore_StartSessionAndReads(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	userID := seedUser(ctx, t, pool)
	problemID, configID := seedProblem(ctx, t, pool, "6B")

	started, err := store.StartSession(ctx, session.Session{
		UserID:    userID,
		Holdsetup: 1,
		Angle:     40,
		MaxGrade:  "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: problemID, ConfigurationID: configID})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if started.ID == "" || started.Status != session.StatusActive {
		t.Fatalf("StartSession returned %+v", started)
	}

	active, err := store.ActiveForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ActiveForUser: %v", err)
	}
	if active.ID != started.ID {
		t.Fatalf("ActiveForUser id = %s, want %s", active.ID, started.ID)
	}

	first, err := store.FirstProblem(ctx, started.ID)
	if err != nil {
		t.Fatalf("FirstProblem: %v", err)
	}
	if first.Seq != 0 || first.ConfigurationID != configID {
		t.Fatalf("FirstProblem = %+v, want seq 0 config %d", first, configID)
	}
}

func TestStore_StartSessionSecondActiveRejected(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	userID := seedUser(ctx, t, pool)
	problemID, configID := seedProblem(ctx, t, pool, "6B")
	first := session.SessionProblem{Seq: 0, ProblemID: problemID, ConfigurationID: configID}
	base := session.Session{UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "7A"}

	if _, err := store.StartSession(ctx, base, first); err != nil {
		t.Fatalf("first StartSession: %v", err)
	}
	_, err := store.StartSession(ctx, base, first)
	if !errors.Is(err, session.ErrActiveExists) {
		t.Fatalf("second StartSession err = %v, want ErrActiveExists", err)
	}
}

func TestStore_GetIsNotUserScoped(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	store := session.NewStore(pool)

	owner := seedUser(ctx, t, pool)
	problemID, configID := seedProblem(ctx, t, pool, "6B")

	started, err := store.StartSession(ctx, session.Session{
		UserID: owner, Holdsetup: 1, Angle: 40, MaxGrade: "7A",
	}, session.SessionProblem{Seq: 0, ProblemID: problemID, ConfigurationID: configID})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	got, err := store.Get(ctx, started.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != owner {
		t.Fatalf("Get returned session for %s, want %s", got.UserID, owner)
	}

	if _, err := store.Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}
}
