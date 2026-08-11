package entry

import "go.uber.org/fx"

// Module provides entry domain business logic.
// Stores (entrystore.*) and entry.Manager are wired by the caller due to import cycles.
// HTTP handler is registered by the transport module.
var Module = fx.Module("entry",
	fx.Provide(NewProcessor),
)
