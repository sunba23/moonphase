package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/internal/testdb"
)

// fakeGoTrueAdmin stands in for Supabase's admin API. On DELETE
// /auth/v1/admin/users/{id} it removes the auth.users row itself — exactly what
// real GoTrue does — so the ON DELETE CASCADE to profiles / sessions is
// exercised end to end. status, when non-zero, overrides the 204 response so a
// test can drive the failure path.
type fakeGoTrueAdmin struct {
	*httptest.Server
	calls  int
	status int
}

func newFakeGoTrueAdmin(t *testing.T, pool *pgxpool.Pool) *fakeGoTrueAdmin {
	t.Helper()

	f := &fakeGoTrueAdmin{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls++
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"code":500,"error_code":"unexpected_failure","msg":"boom"}`))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/auth/v1/admin/users/")
		if _, err := pool.Exec(r.Context(), `DELETE FROM auth.users WHERE id = $1`, id); err != nil {
			t.Errorf("fake gotrue: delete auth.users: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(f.Close)

	return f
}

func seedProfileRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO profiles (id, max_grade, holdsetup, angle) VALUES ($1, '6B', 1, 40)
	`, userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
}

func countRows(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query, arg string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, query, arg).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func TestHandleDeleteAccount(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	logger := zerolog.Nop()

	newPages := func(srv *httptest.Server) *authPages {
		client := auth.NewAuthClient(config.Config{
			SupabaseURL:            srv.URL,
			SupabasePublishableKey: "test-anon-key",
			SupabaseSecretKey:      "test-secret-key",
		})
		return newAuthPages(client, false, &logger)
	}

	seedAccount := func(t *testing.T) string {
		t.Helper()
		userID := seedUserRow(ctx, t, pool)
		seedProfileRow(ctx, t, pool, userID)
		pid, cid := seedCandidate(ctx, t, pool, int(time.Now().UnixNano()%1_000_000), "6B", "crimp")
		if _, err := session.NewStore(pool).StartSession(ctx, session.Session{
			UserID: userID, Holdsetup: 1, Angle: 40, MaxGrade: "6B",
		}, session.SessionProblem{Seq: 0, ProblemID: pid, ConfigurationID: cid}); err != nil {
			t.Fatalf("start session: %v", err)
		}
		return userID
	}

	call := func(ap *authPages, userID string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(auth.WithUserID(ctx, userID), http.MethodPost, "/account/delete", nil)
		rec := httptest.NewRecorder()
		ap.handleDeleteAccount(rec, req)
		return rec
	}

	t.Run("deletes the user and cascades to profile and sessions, then clears the session", func(t *testing.T) {
		userID := seedAccount(t)
		ap := newPages(newFakeGoTrueAdmin(t, pool).Server)

		rec := call(ap, userID)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("HX-Redirect"); got != "/signin" {
			t.Fatalf("HX-Redirect = %q, want /signin", got)
		}
		if hasCookie(rec, "mp_session") || hasCookie(rec, "mp_refresh") {
			t.Fatalf("session cookies were not cleared")
		}
		if n := countRows(ctx, t, pool, `SELECT count(*) FROM auth.users WHERE id = $1`, userID); n != 0 {
			t.Fatalf("auth.users rows = %d, want 0", n)
		}
		if n := countRows(ctx, t, pool, `SELECT count(*) FROM profiles WHERE id = $1`, userID); n != 0 {
			t.Fatalf("profiles rows = %d, want 0 (cascade)", n)
		}
		if n := countRows(ctx, t, pool, `SELECT count(*) FROM sessions WHERE user_id = $1`, userID); n != 0 {
			t.Fatalf("sessions rows = %d, want 0 (cascade)", n)
		}
		if n := countRows(ctx, t, pool, `
			SELECT count(*) FROM session_problems sp
			JOIN sessions s ON s.id = sp.session_id WHERE s.user_id = $1`, userID); n != 0 {
			t.Fatalf("session_problems rows = %d, want 0 (cascade)", n)
		}
	})

	t.Run("a GoTrue failure keeps the account and the session intact", func(t *testing.T) {
		userID := seedAccount(t)
		fake := newFakeGoTrueAdmin(t, pool)
		fake.status = http.StatusInternalServerError
		ap := newPages(fake.Server)

		rec := call(ap, userID)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if hasCookie(rec, "mp_session") || rec.Header().Get("HX-Redirect") != "" {
			t.Fatalf("session must not be cleared on a failed delete")
		}
		if n := countRows(ctx, t, pool, `SELECT count(*) FROM profiles WHERE id = $1`, userID); n != 1 {
			t.Fatalf("profiles rows = %d, want 1 (delete failed)", n)
		}
		if n := countRows(ctx, t, pool, `SELECT count(*) FROM sessions WHERE user_id = $1`, userID); n != 1 {
			t.Fatalf("sessions rows = %d, want 1 (delete failed)", n)
		}
	})
}
