package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"go.uber.org/fx"

	"github.com/sunba23/moonphase/internal/config"
)

func jwksURL(cfg config.Config) string {
	return strings.TrimRight(cfg.SupabaseURL, "/") + "/auth/v1/.well-known/jwks.json"
}

// NewKeyCache builds an auto-refreshing cache of the project's JWKS. The
// cache's background refresh loop is tied to an internally-owned context
// (not a per-request one) so it keeps running for the lifetime of the app;
// the OnStop hook cancels it on shutdown.
func NewKeyCache(lc fx.Lifecycle, cfg config.Config) (*jwk.Cache, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create jwk cache: %w", err)
	}

	url := jwksURL(cfg)

	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			if err := cache.Register(startCtx, url); err != nil {
				return fmt.Errorf("register jwks endpoint %q: %w", url, err)
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})

	return cache, nil
}
