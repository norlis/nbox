package nbox

import "go.uber.org/fx"

// Module provides *Config to the FX graph. Imported only by cmd/nbox.
var Module = fx.Module("nbox.config",
	fx.Provide(LoadConfig),
)
