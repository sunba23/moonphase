package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func cookiesFromRecorder(rec *httptest.ResponseRecorder) []*http.Cookie {
	return rec.Result().Cookies()
}

func TestSessionCookies_RoundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookies(rec, &Session{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600}, true)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	for _, c := range cookiesFromRecorder(rec) {
		req.AddCookie(c)
	}

	access, refresh, ok := sessionCookies(req)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if access != "at" {
		t.Fatalf("expected access token %q, got %q", "at", access)
	}
	if refresh != "rt" {
		t.Fatalf("expected refresh token %q, got %q", "rt", refresh)
	}
}

func TestSessionCookies_Absent(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)

	_, _, ok := sessionCookies(req)
	if ok {
		t.Fatal("expected ok=false with no cookies")
	}
}

func TestClearSessionCookies(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearSessionCookies(rec, true)

	cookies := cookiesFromRecorder(rec)
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies cleared, got %d", len(cookies))
	}
	for _, c := range cookies {
		if c.MaxAge != -1 {
			t.Fatalf("cookie %s: expected MaxAge -1, got %d", c.Name, c.MaxAge)
		}
		if c.Value != "" {
			t.Fatalf("cookie %s: expected empty value, got %q", c.Name, c.Value)
		}
	}
}

func TestAccessTokenCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookies(rec, &Session{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600}, false)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	for _, c := range cookiesFromRecorder(rec) {
		req.AddCookie(c)
	}

	token, ok := AccessTokenCookie(req)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if token != "at" {
		t.Fatalf("expected token %q, got %q", "at", token)
	}
}

func TestAccessTokenCookie_Absent(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)

	_, ok := AccessTokenCookie(req)
	if ok {
		t.Fatal("expected ok=false with no cookie")
	}
}
