package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware extracts and verifies the Authorization: Bearer <token> header.
// On success the resolved user id is attached to the request context; on any
// failure (missing header, malformed header, verification error) it responds
// 401 with the same JSON shape and a WWW-Authenticate header.
func Middleware(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w)
				return
			}

			userID, err := v.Verify(r.Context(), token)
			if err != nil {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "

	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}

	token := strings.TrimPrefix(header, prefix)
	if token == "" {
		return "", false
	}

	return token, true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
