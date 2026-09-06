package server

import (
	"strings"
	"testing"
)

func TestValidateSignupCredentials(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		wantOK   bool
	}{
		{"empty email", "", "longenough", false},
		{"whitespace email", "   ", "longenough", false},
		{"short password", "a@b.com", "1234567", false},
		{"exactly min length", "a@b.com", "12345678", true},
		{"normal password", "a@b.com", "correct horse battery staple", true},
		{"over the bcrypt ceiling", "a@b.com", strings.Repeat("x", maxPasswordBytes+1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := validateSignupCredentials(tt.email, tt.password)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (msg %q)", ok, tt.wantOK, msg)
			}
			if !ok && msg == "" {
				t.Fatalf("expected a user-facing message on failure")
			}
			if ok && msg != "" {
				t.Fatalf("expected an empty message on success, got %q", msg)
			}
		})
	}
}
