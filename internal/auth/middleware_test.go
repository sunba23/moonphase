package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/sunba23/moonphase/internal/config"
)

func generateTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return key
}

// testJWKS serves a self-signed key pair's public half over HTTP so a real
// Verifier can validate tokens signed with the matching private key, without
// depending on a live Supabase project.
type testJWKS struct {
	cfg     config.Config
	privKey jwk.Key
}

func startTestJWKS(t *testing.T) *testJWKS {
	t.Helper()

	privKey, err := jwk.Import(generateTestKey(t))
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

	return &testJWKS{
		cfg:     config.Config{SupabaseURL: srv.URL, SupabasePublishableKey: "test-anon-key"},
		privKey: privKey,
	}
}

func (j *testJWKS) newVerifier(t *testing.T) *Verifier {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		t.Fatalf("new jwk cache: %v", err)
	}
	if err := cache.Register(ctx, jwksURL(j.cfg)); err != nil {
		t.Fatalf("register jwks: %v", err)
	}

	return NewVerifier(cache, j.cfg)
}

func (j *testJWKS) signToken(t *testing.T, userID string, exp time.Time) string {
	t.Helper()

	tok, err := jwt.NewBuilder().
		Issuer(strings.TrimRight(j.cfg.SupabaseURL, "/") + "/auth/v1").
		Audience([]string{"authenticated"}).
		Subject(userID).
		IssuedAt(time.Now()).
		Expiration(exp).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), j.privKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return string(signed)
}

func TestMiddleware_ValidSession(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)
	token := jwks.signToken(t, "user-1", time.Now().Add(time.Hour))

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	Middleware(verifier, NewAuthClient(jwks.cfg), false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUserID != "user-1" {
		t.Fatalf("expected user id %q, got %q", "user-1", gotUserID)
	}
}

func TestMiddleware_NoCookies(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Middleware(verifier, NewAuthClient(jwks.cfg), false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/signin" {
		t.Fatalf("expected redirect to /signin, got %q", loc)
	}
}

func TestMiddleware_ExpiredWithValidRefresh(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)
	expired := jwks.signToken(t, "user-1", time.Now().Add(-time.Hour))

	gotrue := startFakeGoTrue(t, map[string]gotrueResponse{
		"/auth/v1/token": {status: http.StatusOK, body: `{"access_token":"new-access-token","refresh_token":"new-rt","expires_in":3600,"user":{"id":"user-1"}}`},
	})
	ac := NewAuthClient(config.Config{SupabaseURL: gotrue.URL, SupabasePublishableKey: "test-anon-key"})

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: expired})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-rt"})
	rec := httptest.NewRecorder()

	Middleware(verifier, ac, false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUserID != "user-1" {
		t.Fatalf("expected user id %q, got %q", "user-1", gotUserID)
	}

	found := false
	for _, c := range cookiesFromRecorder(rec) {
		if c.Name == sessionCookieName && c.Value == "new-access-token" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected refreshed session cookie to be set")
	}
}

func TestMiddleware_ExpiredNoRefreshCookie(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)
	expired := jwks.signToken(t, "user-1", time.Now().Add(-time.Hour))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: expired})
	rec := httptest.NewRecorder()

	Middleware(verifier, NewAuthClient(jwks.cfg), false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	for _, c := range cookiesFromRecorder(rec) {
		if c.MaxAge != -1 {
			t.Fatalf("expected cleared cookie, got %+v", c)
		}
	}
}

func TestMiddleware_ExpiredRefreshFails(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)
	expired := jwks.signToken(t, "user-1", time.Now().Add(-time.Hour))

	gotrue := startFakeGoTrue(t, map[string]gotrueResponse{
		"/auth/v1/token": {status: http.StatusUnauthorized, body: `{"code":401,"error_code":"invalid_grant","msg":"Invalid Refresh Token"}`},
	})
	ac := NewAuthClient(config.Config{SupabaseURL: gotrue.URL, SupabasePublishableKey: "test-anon-key"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: expired})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "bad-rt"})
	rec := httptest.NewRecorder()

	Middleware(verifier, ac, false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/signin" {
		t.Fatalf("expected redirect to /signin, got %q", loc)
	}
}

func TestMiddleware_MalformedTokenSkipsRefresh(t *testing.T) {
	jwks := startTestJWKS(t)
	verifier := jwks.newVerifier(t)

	var refreshCalls int32
	gotrue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&refreshCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(gotrue.Close)
	ac := NewAuthClient(config.Config{SupabaseURL: gotrue.URL, SupabasePublishableKey: "test-anon-key"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-jwt"})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "some-rt"})
	rec := httptest.NewRecorder()

	Middleware(verifier, ac, false)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if calls := atomic.LoadInt32(&refreshCalls); calls != 0 {
		t.Fatalf("expected no refresh call attempt, got %d", calls)
	}
}
