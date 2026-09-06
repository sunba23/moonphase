package config

import (
	"errors"
	"os"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	AppEnv                 string
	SupabaseURL            string
	SupabasePublishableKey string
	// Version is the full commit SHA the running binary was built from, or
	// "dev" when RAILWAY_GIT_COMMIT_SHA is not set (local runs). The page
	// footer renders a short form of it.
	Version string
}

func Load() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		return Config{}, errors.New("SUPABASE_URL is required")
	}

	supabasePublishableKey := os.Getenv("SUPABASE_PUBLISHABLE_KEY")
	if supabasePublishableKey == "" {
		return Config{}, errors.New("SUPABASE_PUBLISHABLE_KEY is required")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	version := os.Getenv("RAILWAY_GIT_COMMIT_SHA")
	if version == "" {
		version = "dev"
	}

	return Config{
		Port:                   port,
		DatabaseURL:            databaseURL,
		AppEnv:                 appEnv,
		SupabaseURL:            supabaseURL,
		SupabasePublishableKey: supabasePublishableKey,
		Version:                version,
	}, nil
}
