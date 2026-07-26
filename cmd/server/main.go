package main

import (
	"github.com/joho/godotenv"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/db"
	"github.com/sunba23/moonphase/internal/server"
)

func main() {
	_ = godotenv.Load() // no-op if .env doesn't exist, e.g. in production where vars are injected directly

	fx.New(config.Module, db.Module, server.Module).Run()
}
