package main

import (
	"github.com/joho/godotenv"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/db"
	"github.com/sunba23/moonphase/internal/logging"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/internal/server"
)

func main() {
	_ = godotenv.Load() // no-op if .env doesn't exist, e.g. in production where vars are injected directly

	fx.New(config.Module, logging.Module, db.Module, auth.Module, profile.Module, server.Module).Run()
}
