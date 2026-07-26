package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/sunba23/moonphase/internal/config"
)

const expectedAudience = "authenticated"

var errMissingSubject = errors.New("auth: token has no sub claim")

// Verifier turns a raw bearer token into a resolved Supabase user id.
type Verifier struct {
	cache   *jwk.Cache
	jwksURL string
	issuer  string
}

func NewVerifier(cache *jwk.Cache, cfg config.Config) *Verifier {
	return &Verifier{
		cache:   cache,
		jwksURL: jwksURL(cfg),
		issuer:  strings.TrimRight(cfg.SupabaseURL, "/") + "/auth/v1",
	}
}

// Verify checks the token's signature against the cached JWKS and validates
// exp, aud ("authenticated") and iss (the project's auth issuer) before
// trusting the sub claim as the resolved user id.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	keySet, err := v.cache.CachedSet(v.jwksURL)
	if err != nil {
		return "", fmt.Errorf("auth: look up jwks: %w", err)
	}

	tok, err := jwt.Parse([]byte(token),
		jwt.WithKeySet(keySet),
		jwt.WithAudience(expectedAudience),
		jwt.WithIssuer(v.issuer),
		jwt.WithContext(ctx),
	)
	if err != nil {
		return "", fmt.Errorf("auth: verify token: %w", err)
	}

	sub, ok := tok.Subject()
	if !ok || sub == "" {
		return "", errMissingSubject
	}

	return sub, nil
}
