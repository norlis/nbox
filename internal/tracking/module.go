package tracking

import "go.uber.org/fx"

// Module provides tracking domain components.
// Note: tracking.Store (trackingstore.NewDynamoDB) must be provided by the caller
// because tracking/store imports this package, which would create an import cycle.
var Module = fx.Module("tracking",
	fx.Provide(NewHandler),
)
