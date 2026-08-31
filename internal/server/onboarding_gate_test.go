package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/profile"
)

type fakeProfileChecker struct {
	profile *profile.Profile
	err     error
}

func (f *fakeProfileChecker) Get(_ context.Context, _ string) (*profile.Profile, error) {
	return f.profile, f.err
}

func TestOnboardingGate_NotFound(t *testing.T) {
	pc := &fakeProfileChecker{err: profile.ErrNotFound}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	OnboardingGate(pc)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/onboarding" {
		t.Fatalf("expected redirect to /onboarding, got %q", loc)
	}
}

func TestOnboardingGate_ExistingProfile(t *testing.T) {
	pc := &fakeProfileChecker{profile: &profile.Profile{UserID: "user-1"}}

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	OnboardingGate(pc)(next).ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestOnboardingGate_OtherError(t *testing.T) {
	pc := &fakeProfileChecker{err: errors.New("db exploded")}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	OnboardingGate(pc)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
