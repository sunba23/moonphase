package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
)

const indexHTML = `<!DOCTYPE html>
<html>
<head><title>MoonPhase</title></head>
<body><h1>MoonPhase v0</h1></body>
</html>`

func NewRouter(verifier *auth.Verifier, logger *zerolog.Logger) *chi.Mux {
	r := chi.NewRouter()

	r.Use(requestLogger(logger))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(verifier))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(indexHTML))
		})

		r.Get("/api/me", handleMe)
	})

	return r
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "internal error: no user id in context", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
}

func registerHooks(lc fx.Lifecycle, cfg config.Config, router *chi.Mux, logger *zerolog.Logger) {
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Bind synchronously so a taken port fails app boot instead of
			// silently leaving the server unreachable.
			var listenCfg net.ListenConfig
			ln, err := listenCfg.Listen(ctx, "tcp", srv.Addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", srv.Addr, err)
			}

			go func() {
				if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error().Err(err).Msg("server stopped unexpectedly")
				}
			}()

			logger.Info().Str("addr", srv.Addr).Msg("server started")

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
