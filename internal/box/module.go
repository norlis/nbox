package box

import (
	"go.uber.org/fx"
	boxspec "nbox/internal/box/store/spec"
)

// Module provides box domain business logic.
// box.Store (boxstore.NewS3) is wired by the caller due to import cycles.
// HTTP handler is registered by the transport module.
var Module = fx.Module("box",
	boxspec.Module,
	fx.Provide(NewStrategyResolver),
	fx.Provide(NewRenderer),
)
