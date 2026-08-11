package export

import "go.uber.org/fx"

// Module provides export domain business logic.
// HTTP handler is registered by the transport module.
var Module = fx.Module("export",
	fx.Provide(NewGenerator),
)
