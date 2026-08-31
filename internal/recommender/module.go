package recommender

import "go.uber.org/fx"

var Module = fx.Module("recommender", fx.Provide(New))
