package main

import (
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/server"
)

func main() {
	fx.New(config.Module, server.Module).Run()
}
