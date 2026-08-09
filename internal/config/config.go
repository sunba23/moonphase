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

	return Config{
		Port:                   port,
		DatabaseURL:            databaseURL,
		AppEnv:                 appEnv,
		SupabaseURL:            supabaseURL,
		SupabasePublishableKey: supabasePublishableKey,
	}, nil
}
