package application

import (
	status "github.com/norlis/httpgate/pkg/application/health"
	"go.uber.org/fx"
)

var Module = fx.Module("application",
	fx.Provide(NewConfigFromEnv),
	fx.Provide(func() *status.Status {
		version := GitHash
		if version == "" {
			version = "unknown.dev"
		}
		return status.NewStatus(version)
	}),
)
