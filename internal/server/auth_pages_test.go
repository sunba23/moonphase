package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
)

// gotrueResp scripts the single response the fake GoTrue server returns.
type gotrueResp struct {
	status int
	body   string
}

// fakeGoTrue is a stand-in for the Supabase auth API. It counts requests so a
// test can assert the handler did (or did not) call out.
type fakeGoTrue struct {
	*httptest.Server
	calls int
}

func newFakeGoTrue(t *testing.T, resp gotrueResp) *fakeGoTrue {
	t.Helper()

	f := &fakeGoTrue{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = io.WriteString(w, resp.body)
	}))
	t.Cleanup(f.Close)

	return f
}

func newTestAuthPages(srv *httptest.Server) *authPages {
	logger := zerolog.Nop()
	client := auth.NewAuthClient(config.Config{SupabaseURL: srv.URL, SupabasePublishableKey: "test-key"})
	return newAuthPages(client, false, &logger)
}

func postCredentials(h http.HandlerFunc, email, password string) *httptest.ResponseRecorder {
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	h(rec, req)

	return rec
}

func hasCookie(rec *httptest.ResponseRecorder, name string) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}

func TestAuthPages_SignupSubmit(t *testing.T) {
	t.Run("short password fails locally without calling gotrue", func(t *testing.T) {
		fake := newFakeGoTrue(t, gotrueResp{status: http.StatusOK, body: "{}"})
		ap := newTestAuthPages(fake.Server)

		rec := postCredentials(ap.handleSignupSubmit, "a@b.com", "short")

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "at least 8 characters") {
			t.Fatalf("body missing the local message:\n%s", body)
		}
		if !strings.Contains(body, `id="authForm"`) {
			t.Fatalf("body did not re-render the form:\n%s", body)
		}
		if fake.calls != 0 {
			t.Fatalf("gotrue called %d times, want 0", fake.calls)
		}
	})

	t.Run("gotrue rejection is forwarded and email is echoed back", func(t *testing.T) {
		fake := newFakeGoTrue(t, gotrueResp{
			status: http.StatusUnprocessableEntity,
			body:   `{"code":422,"error_code":"user_already_exists","msg":"User already registered"}`,
		})
		ap := newTestAuthPages(fake.Server)

		rec := postCredentials(ap.handleSignupSubmit, "a@b.com", "longenough")

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "User already registered") {
			t.Fatalf("gotrue message not forwarded:\n%s", body)
		}
		if !strings.Contains(body, `value="a@b.com"`) {
			t.Fatalf("email not echoed back into the form:\n%s", body)
		}
	})

	t.Run("success sets session cookie and HX-Redirect to onboarding", func(t *testing.T) {
		fake := newFakeGoTrue(t, gotrueResp{
			status: http.StatusOK,
			body:   `{"access_token":"at","refresh_token":"rt","expires_in":3600,"user":{"id":"u1"}}`,
		})
		ap := newTestAuthPages(fake.Server)

		rec := postCredentials(ap.handleSignupSubmit, "a@b.com", "longenough")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("HX-Redirect"); got != "/onboarding" {
			t.Fatalf("HX-Redirect = %q, want /onboarding", got)
		}
		if !hasCookie(rec, "mp_session") {
			t.Fatalf("session cookie not set")
		}
	})
}

func TestAuthPages_SigninSubmit(t *testing.T) {
	t.Run("no local password rules: a short password reaches gotrue and its message is forwarded", func(t *testing.T) {
		fake := newFakeGoTrue(t, gotrueResp{
			status: http.StatusBadRequest,
			body:   `{"code":400,"error_code":"invalid_credentials","msg":"Invalid login credentials"}`,
		})
		ap := newTestAuthPages(fake.Server)

		rec := postCredentials(ap.handleSigninSubmit, "a@b.com", "x")

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		if fake.calls != 1 {
			t.Fatalf("gotrue called %d times, want 1", fake.calls)
		}
		if body := rec.Body.String(); !strings.Contains(body, "Invalid login credentials") {
			t.Fatalf("gotrue message not forwarded:\n%s", body)
		}
	})

	t.Run("success redirects to the hub", func(t *testing.T) {
		fake := newFakeGoTrue(t, gotrueResp{
			status: http.StatusOK,
			body:   `{"access_token":"at","refresh_token":"rt","expires_in":3600,"user":{"id":"u1"}}`,
		})
		ap := newTestAuthPages(fake.Server)

		rec := postCredentials(ap.handleSigninSubmit, "a@b.com", "whatever8")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("HX-Redirect"); got != "/" {
			t.Fatalf("HX-Redirect = %q, want /", got)
		}
	})
}
