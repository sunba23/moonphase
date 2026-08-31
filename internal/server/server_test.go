package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/profile"
)

// testIdentity serves a self-signed key pair's public half over HTTP so a
// real auth.Verifier can validate tokens signed with the matching private
// key, without depending on a live Supabase project.
type testIdentity struct {
	cfg     config.Config
	privKey jwk.Key
}

func startTestJWKS(t *testing.T) *testIdentity {
	t.Helper()

	raw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	privKey, err := jwk.Import(raw)
	if err != nil {
		t.Fatalf("import key: %v", err)
	}
	_ = privKey.Set(jwk.KeyIDKey, "test-key")
	_ = privKey.Set(jwk.AlgorithmKey, jwa.ES256())

	pubKey, err := jwk.PublicKeyOf(privKey)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}

	set := jwk.NewSet()
	if err := set.AddKey(pubKey); err != nil {
		t.Fatalf("add key to set: %v", err)
	}

	body, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/v1/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testIdentity{
		cfg:     config.Config{SupabaseURL: srv.URL, SupabasePublishableKey: "test-anon-key"},
		privKey: privKey,
	}
}

func (id *testIdentity) newVerifier(t *testing.T) *auth.Verifier {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		t.Fatalf("new jwk cache: %v", err)
	}

	jwksURL := strings.TrimRight(id.cfg.SupabaseURL, "/") + "/auth/v1/.well-known/jwks.json"
	if err := cache.Register(ctx, jwksURL); err != nil {
		t.Fatalf("register jwks: %v", err)
	}

	return auth.NewVerifier(cache, id.cfg)
}

func (id *testIdentity) signToken(t *testing.T, userID string) string {
	t.Helper()

	tok, err := jwt.NewBuilder().
		Issuer(strings.TrimRight(id.cfg.SupabaseURL, "/") + "/auth/v1").
		Audience([]string{"authenticated"}).
		Subject(userID).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), id.privKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return string(signed)
}

// newTestRouter mirrors NewRouter's three-tier grouping (public /
// auth-only / auth+onboarding-complete) with trivial stand-in handlers for
// /, /api/me, and /onboarding, so the grouping and gating logic can be
// proven without a live Postgres pool or AuthClient — the real handlers'
// content is covered by Phase 5/6's manual verification instead.
func newTestRouter(verifier *auth.Verifier, authClient *auth.AuthClient, pc ProfileChecker) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(verifier, authClient, false))

		r.Get("/onboarding", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		r.Group(func(r chi.Router) {
			r.Use(OnboardingGate(pc))

			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			r.Get("/api/me", handleMe)
		})
	})

	return r
}

func TestRouter_PublicRoutesNeedNoSession(t *testing.T) {
	id := startTestJWKS(t)
	router := newTestRouter(id.newVerifier(t), auth.NewAuthClient(id.cfg), &fakeProfileChecker{err: profile.ErrNotFound})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz: expected 200, got %d", rec.Code)
	}
}

func TestRouter_ProtectedRoutesRedirectWithoutSession(t *testing.T) {
	id := startTestJWKS(t)
	router := newTestRouter(id.newVerifier(t), auth.NewAuthClient(id.cfg), &fakeProfileChecker{err: profile.ErrNotFound})

	for _, path := range []string{"/", "/api/me"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("%s: expected 302, got %d", path, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/signin" {
			t.Fatalf("%s: expected redirect to /signin, got %q", path, loc)
		}
	}
}

func TestRouter_SessionWithoutProfileRedirectsToOnboarding(t *testing.T) {
	id := startTestJWKS(t)
	router := newTestRouter(id.newVerifier(t), auth.NewAuthClient(id.cfg), &fakeProfileChecker{err: profile.ErrNotFound})
	token := id.signToken(t, "user-1")

	for _, path := range []string{"/", "/api/me"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "mp_session", Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("%s: expected 302, got %d", path, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/onboarding" {
			t.Fatalf("%s: expected redirect to /onboarding, got %q", path, loc)
		}
	}
}

func TestRouter_OnboardingReachableWithoutProfile(t *testing.T) {
	id := startTestJWKS(t)
	router := newTestRouter(id.newVerifier(t), auth.NewAuthClient(id.cfg), &fakeProfileChecker{err: profile.ErrNotFound})
	token := id.signToken(t, "user-1")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/onboarding", nil)
	req.AddCookie(&http.Cookie{Name: "mp_session", Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouter_SessionWithProfileReachesProtectedRoutes(t *testing.T) {
	id := startTestJWKS(t)
	router := newTestRouter(id.newVerifier(t), auth.NewAuthClient(id.cfg), &fakeProfileChecker{profile: &profile.Profile{UserID: "user-1"}})
	token := id.signToken(t, "user-1")

	for _, path := range []string{"/", "/api/me"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "mp_session", Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, rec.Code)
		}
	}
}
