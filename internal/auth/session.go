package auth

import "net/http"

const (
	sessionCookieName = "mp_session"
	refreshCookieName = "mp_refresh"
)

// SetSessionCookies writes the access and refresh tokens as HttpOnly cookies.
// The access-token cookie expires with the session; the refresh-token cookie
// is a session cookie (no Max-Age) since GoTrue owns its actual expiry and
// sign-out is what clears it explicitly.
func SetSessionCookies(w http.ResponseWriter, sess *Session, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.AccessToken,
		Path:     "/",
		MaxAge:   sess.ExpiresIn,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    sess.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies expires both session cookies immediately.
func ClearSessionCookies(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// AccessTokenCookie returns the access-token cookie value from the request,
// if present. Exposed for handlers (e.g. sign-out) that need the raw token
// outside the auth middleware's verify/refresh flow.
func AccessTokenCookie(r *http.Request) (string, bool) {
	accessToken, _, ok := sessionCookies(r)
	return accessToken, ok
}

// sessionCookies reads the access and refresh tokens off the request. ok is
// true only when an access-token cookie is present; refreshToken may be
// empty even when ok is true (no refresh cookie set).
func sessionCookies(r *http.Request) (accessToken, refreshToken string, ok bool) {
	access, err := r.Cookie(sessionCookieName)
	if err != nil || access.Value == "" {
		return "", "", false
	}

	if refresh, err := r.Cookie(refreshCookieName); err == nil {
		refreshToken = refresh.Value
	}

	return access.Value, refreshToken, true
}
