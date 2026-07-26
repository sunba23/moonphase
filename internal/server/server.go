package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/config"
)

const indexHTML = `<!DOCTYPE html>
<html>
<head><title>MoonPhase</title></head>
<body><h1>MoonPhase v0</h1></body>
</html>`

func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(indexHTML))
	})

	return r
}

func registerHooks(lc fx.Lifecycle, cfg config.Config, router *chi.Mux) {
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				_ = srv.ListenAndServe()
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}

var Module = fx.Module("server",
	fx.Provide(NewRouter),
	fx.Invoke(registerHooks),
)
