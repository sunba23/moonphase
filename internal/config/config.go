package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	AppEnv      string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	return Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AppEnv:      appEnv,
	}
}
