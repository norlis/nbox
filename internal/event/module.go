package event

import "go.uber.org/fx"

// Module provides event domain components.
// Note: bus.NewMemory must be provided by the caller
// because event/bus imports this package, which would create an import cycle.
var Module = fx.Module("event",
	fx.Provide(NewDispatcher),
)
