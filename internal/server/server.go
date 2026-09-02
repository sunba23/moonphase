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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/auth"
	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/profile"
	"github.com/sunba23/moonphase/internal/recommender"
	"github.com/sunba23/moonphase/internal/session"
	"github.com/sunba23/moonphase/static"
)

func NewRouter(verifier *auth.Verifier, authClient *auth.AuthClient, profileStore *profile.Store, sessionStore *session.Store, rec *recommender.Recommender, pool *pgxpool.Pool, cfg config.Config, logger *zerolog.Logger) *chi.Mux {
	r := chi.NewRouter()

	secure := cfg.AppEnv != "development"

	r.Use(requestLogger(logger))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	// Browsers request /favicon.ico unprompted; answer with No Content so it
	// does not fall through to a 404.
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ap := newAuthPages(authClient, secure, logger)
	op := newOnboardingPages(pool, profileStore, logger)
	pp := newProfilePages(pool, profileStore, logger)
	hp := newHubPages(profileStore, logger)
	sp := newSessionPages(pool, profileStore, sessionStore, rec, logger)

	r.Get("/signup", ap.handleSignupPage)
	r.Post("/signup", ap.handleSignupSubmit)
	r.Get("/signin", ap.handleSigninPage)
	r.Post("/signin", ap.handleSigninSubmit)

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(verifier, authClient, secure))

		// Auth-only tier: needs a resolved user id but must NOT be gated by
		// onboarding completeness.

		r.Post("/signout", ap.handleSignout)
		r.Get("/onboarding", op.handlePage)
		r.Post("/onboarding", op.handleSubmit)

		r.Group(func(r chi.Router) {
			r.Use(OnboardingGate(profileStore))

			r.Get("/", hp.handlePage)

			r.Get("/api/me", handleMe)

			r.Get("/profile", pp.handlePage)
			r.Post("/profile", pp.handleSubmit)

			r.Post("/session", sp.handleStart)
			r.Get("/session/{sessionID}", sp.handleView)
			r.Post("/session/{sessionID}/result", sp.handleResult)
			r.Post("/session/{sessionID}/end", sp.handleEnd)
		})
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
