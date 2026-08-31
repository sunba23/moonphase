package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunba23/moonphase/internal/config"
)

// gotrueResponse scripts one fake GoTrue endpoint's response.
type gotrueResponse struct {
	status int
	body   string
}

// startFakeGoTrue serves the scripted responses keyed by request path (query
// strings are ignored, matching how AuthClient builds its paths).
func startFakeGoTrue(t *testing.T, scripted map[string]gotrueResponse) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, ok := scripted[r.URL.Path]
		if !ok {
			t.Fatalf("fake gotrue: unscripted path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func newTestAuthClient(srv *httptest.Server) *AuthClient {
	return NewAuthClient(config.Config{SupabaseURL: srv.URL, SupabasePublishableKey: "test-anon-key"})
}

func TestAuthClient_SignUp(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/signup": {status: http.StatusOK, body: `{"access_token":"at","refresh_token":"rt","expires_in":3600,"user":{"id":"u1"}}`},
		})

		sess, err := newTestAuthClient(srv).SignUp(context.Background(), "a@b.com", "pw")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess.AccessToken != "at" || sess.RefreshToken != "rt" || sess.ExpiresIn != 3600 || sess.UserID != "u1" {
			t.Fatalf("unexpected session: %+v", sess)
		}
	})

	t.Run("duplicate email", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/signup": {status: http.StatusUnprocessableEntity, body: `{"code":422,"error_code":"user_already_exists","msg":"User already registered"}`},
		})

		_, err := newTestAuthClient(srv).SignUp(context.Background(), "a@b.com", "pw")

		var apiErr *AuthAPIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *AuthAPIError, got %v", err)
		}
		if apiErr.ErrorCode != "user_already_exists" {
			t.Fatalf("unexpected error code: %s", apiErr.ErrorCode)
		}
	})

	t.Run("email confirmation enabled", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/signup": {status: http.StatusOK, body: `{"user":{"id":"u1"}}`},
		})

		_, err := newTestAuthClient(srv).SignUp(context.Background(), "a@b.com", "pw")
		if !errors.Is(err, errEmailConfirmationEnabled) {
			t.Fatalf("expected errEmailConfirmationEnabled, got %v", err)
		}
	})
}

func TestAuthClient_SignInWithPassword(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/token": {status: http.StatusOK, body: `{"access_token":"at","refresh_token":"rt","expires_in":3600,"user":{"id":"u1"}}`},
		})

		sess, err := newTestAuthClient(srv).SignInWithPassword(context.Background(), "a@b.com", "pw")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess.UserID != "u1" {
			t.Fatalf("unexpected session: %+v", sess)
		}
	})

	t.Run("bad credentials", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/token": {status: http.StatusBadRequest, body: `{"code":400,"error_code":"invalid_credentials","msg":"Invalid login credentials"}`},
		})

		_, err := newTestAuthClient(srv).SignInWithPassword(context.Background(), "a@b.com", "wrong")

		var apiErr *AuthAPIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *AuthAPIError, got %v", err)
		}
		if apiErr.ErrorCode != "invalid_credentials" {
			t.Fatalf("unexpected error code: %s", apiErr.ErrorCode)
		}
	})
}

func TestAuthClient_RefreshSession(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/token": {status: http.StatusOK, body: `{"access_token":"at2","refresh_token":"rt2","expires_in":3600,"user":{"id":"u1"}}`},
		})

		sess, err := newTestAuthClient(srv).RefreshSession(context.Background(), "rt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess.AccessToken != "at2" {
			t.Fatalf("unexpected session: %+v", sess)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/token": {status: http.StatusUnauthorized, body: `{"code":401,"error_code":"invalid_grant","msg":"Invalid Refresh Token"}`},
		})

		_, err := newTestAuthClient(srv).RefreshSession(context.Background(), "bad-rt")

		var apiErr *AuthAPIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *AuthAPIError, got %v", err)
		}
	})
}

func TestAuthClient_SignOut(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := startFakeGoTrue(t, map[string]gotrueResponse{
			"/auth/v1/logout": {status: http.StatusNoContent, body: ""},
		})

		if err := newTestAuthClient(srv).SignOut(context.Background(), "at"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
