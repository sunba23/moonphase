package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/rs/zerolog"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/templates/pages"
)

// authPages wires the signup/signin/signout HTTP handlers to AuthClient and
// the session-cookie helpers.
type authPages struct {
	authClient *auth.AuthClient
	secure     bool
	logger     *zerolog.Logger
}

func newAuthPages(authClient *auth.AuthClient, secure bool, logger *zerolog.Logger) *authPages {
	return &authPages{authClient: authClient, secure: secure, logger: logger}
}

func (a *authPages) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, pages.SignupPage(pages.AuthFormModel{}), http.StatusOK)
}

func (a *authPages) handleSigninPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, pages.SigninPage(pages.AuthFormModel{}), http.StatusOK)
}

func (a *authPages) handleSignupSubmit(w http.ResponseWriter, r *http.Request) {
	// Every fresh signup needs onboarding — go straight there.
	a.submit(w, r, a.authClient.SignUp, pages.SignupPage, "/onboarding")
}

func (a *authPages) handleSigninSubmit(w http.ResponseWriter, r *http.Request) {
	// A returning user may already be onboarded; redirect to the hub and let
	// OnboardingGate bounce them to /onboarding if they're not.
	a.submit(w, r, a.authClient.SignInWithPassword, pages.SigninPage, "/")
}

// authCall is the shared shape of AuthClient.SignUp and
// AuthClient.SignInWithPassword, letting handleSignupSubmit and
// handleSigninSubmit share one submit implementation.
type authCall func(ctx context.Context, email, password string) (*auth.Session, error)

const maxAuthFormBytes = 1 << 16 // 64KiB — generous for an email+password form

func (a *authPages) submit(w http.ResponseWriter, r *http.Request, call authCall, page func(pages.AuthFormModel) templ.Component, successRedirect string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	sess, err := call(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		var apiErr *auth.AuthAPIError
		if errors.As(err, &apiErr) {
			renderPage(w, r, page(pages.AuthFormModel{Error: apiErr.Message}), http.StatusUnprocessableEntity)
			return
		}

		// Anything else (network failure, the email-confirmation-enabled
		// sentinel) is an ops misconfiguration or transient failure, not a
		// user-facing form error — fail loud per plan Migration Notes.
		a.logger.Error().Err(err).Msg("auth submit failed unexpectedly")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	auth.SetSessionCookies(w, sess, a.secure)
	w.Header().Set("HX-Redirect", successRedirect)
	w.WriteHeader(http.StatusOK)
}

func (a *authPages) handleSignout(w http.ResponseWriter, r *http.Request) {
	if accessToken, ok := auth.AccessTokenCookie(r); ok {
		if err := a.authClient.SignOut(r.Context(), accessToken); err != nil {
			a.logger.Warn().Err(err).Msg("signout: best-effort GoTrue logout failed")
		}
	}

	auth.ClearSessionCookies(w, a.secure)
	w.Header().Set("HX-Redirect", "/signin")
	w.WriteHeader(http.StatusOK)
}

func renderPage(w http.ResponseWriter, r *http.Request, c templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = c.Render(r.Context(), w)
}
