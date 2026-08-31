package session

import "go.uber.org/fx"

var Module = fx.Module("session", fx.Provide(NewStore))
