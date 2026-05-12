package prefix

import "go.uber.org/fx"

// Module provides prefix domain components.
// Note: prefix.Store (prefixstore.NewDynamoDB) must be provided by the caller
// because prefix/store imports this package, which would create an import cycle.
var Module = fx.Module("prefix",
	fx.Provide(NewHandler),
)
