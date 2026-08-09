package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunba23/moonphase/internal/config"
)

const authRequestTimeout = 10 * time.Second

// errEmailConfirmationEnabled signals that GoTrue returned a 2xx response
// with no session (empty access_token) — a sign the Supabase project's
// "Confirm email" setting wasn't actually turned off (see plan Migration Notes).
var errEmailConfirmationEnabled = errors.New("auth: signup/signin succeeded without a session; is email confirmation still enabled?")

// Session is the normalized shape returned by every AuthClient call that
// yields a live session, regardless of which GoTrue endpoint produced it.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	UserID       string
}

// AuthAPIError wraps a non-2xx response from GoTrue's REST API, exposing its
// structured {code, error_code, msg} body instead of an opaque string so
// callers can branch on it with errors.As.
type AuthAPIError struct {
	StatusCode int
	ErrorCode  string
	Message    string
}

func (e *AuthAPIError) Error() string {
	return fmt.Sprintf("auth api: %s (%s)", e.Message, e.ErrorCode)
}

// AuthClient is a minimal, typed client for the GoTrue REST calls this app
// needs. It's hand-rolled rather than built on the community SDK because
// that SDK's errors are unstructured strings that can't drive the
// duplicate-email/bad-credentials/rate-limited branching the HTMX UI needs.
type AuthClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewAuthClient(cfg config.Config) *AuthClient {
	return &AuthClient{
		baseURL:    strings.TrimRight(cfg.SupabaseURL, "/") + "/auth/v1",
		apiKey:     cfg.SupabasePublishableKey,
		httpClient: &http.Client{Timeout: authRequestTimeout},
	}
}

func (c *AuthClient) SignUp(ctx context.Context, email, password string) (*Session, error) {
	return c.sessionRequest(ctx, "/signup", map[string]string{
		"email":    email,
		"password": password,
	})
}

func (c *AuthClient) SignInWithPassword(ctx context.Context, email, password string) (*Session, error) {
	return c.sessionRequest(ctx, "/token?grant_type=password", map[string]string{
		"email":    email,
		"password": password,
	})
}

func (c *AuthClient) RefreshSession(ctx context.Context, refreshToken string) (*Session, error) {
	return c.sessionRequest(ctx, "/token?grant_type=refresh_token", map[string]string{
		"refresh_token": refreshToken,
	})
}

func (c *AuthClient) SignOut(ctx context.Context, accessToken string) error {
	req, err := c.newRequest(ctx, "/logout", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth: call logout: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("auth: logout: %w", decodeAuthError(resp))
	}

	return nil
}

// sessionRequest issues a POST to a GoTrue endpoint that yields a session on
// success (signup, password grant, refresh grant) and normalizes the result.
func (c *AuthClient) sessionRequest(ctx context.Context, path string, body map[string]string) (*Session, error) {
	req, err := c.newRequest(ctx, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: call %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("auth: %s: %w", path, decodeAuthError(resp))
	}

	var out sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("auth: decode %s response: %w", path, err)
	}

	if out.AccessToken == "" {
		return nil, fmt.Errorf("auth: %s: %w", path, errEmailConfirmationEnabled)
	}

	return &Session{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		ExpiresIn:    out.ExpiresIn,
		UserID:       out.User.ID,
	}, nil
}

func (c *AuthClient) newRequest(ctx context.Context, path string, body map[string]string) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("auth: encode request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("auth: build request: %w", err)
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// sessionResponse is GoTrue's flat AccessTokenResponse shape, returned as a
// top-level JSON object (no session wrapper) by /signup (when autoconfirm is
// on), /token?grant_type=password, and /token?grant_type=refresh_token.
type sessionResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		ID string `json:"id"`
	} `json:"user"`
}

// authErrorResponse is GoTrue's standard error body shape.
type authErrorResponse struct {
	StatusCode int    `json:"code"`
	ErrorCode  string `json:"error_code"`
	Message    string `json:"msg"`
}

func decodeAuthError(resp *http.Response) error {
	var out authErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode error body (status %d): %w", resp.StatusCode, err)
	}

	return &AuthAPIError{
		StatusCode: out.StatusCode,
		ErrorCode:  out.ErrorCode,
		Message:    out.Message,
	}
}
