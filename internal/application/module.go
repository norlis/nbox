package application

import (
	"github.com/norlis/httpgate/health"
	"go.uber.org/fx"
	"nbox/internal/version"
)

var Module = fx.Module("application",
	fx.Provide(func() *health.Status {
		v := version.GitHash
		if v == "" {
			v = "unknown.dev"
		}
		return health.NewStatus(v)
	}),
)
