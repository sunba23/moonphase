package config

import (
	"errors"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	AppEnv      string
	SupabaseURL string
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

	return Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AppEnv:      appEnv,
		SupabaseURL: supabaseURL,
	}, nil
}
