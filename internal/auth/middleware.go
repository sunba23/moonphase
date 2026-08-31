package auth

import (
	"errors"
	"net/http"

	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Middleware extracts the session from cookies. On success the resolved user
// id is attached to the request context. On an expired access token with a
// present refresh cookie, it attempts one transparent refresh before giving
// up. Any unrecoverable failure clears the session cookies and redirects
// (302) to /signin.
func Middleware(v *Verifier, ac *AuthClient, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accessToken, refreshToken, ok := sessionCookies(r)
			if !ok {
				redirectToSignin(w, r)
				return
			}

			userID, err := v.Verify(r.Context(), accessToken)
			if err != nil {
				if errors.Is(err, jwt.TokenExpiredError()) && refreshToken != "" {
					if sess, refreshErr := ac.RefreshSession(r.Context(), refreshToken); refreshErr == nil {
						SetSessionCookies(w, sess, secure)
						next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), sess.UserID)))
						return
					}
				}

				ClearSessionCookies(w, secure)
				redirectToSignin(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

func redirectToSignin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/signin", http.StatusFound)
}
