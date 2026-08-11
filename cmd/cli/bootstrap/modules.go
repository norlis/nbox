package bootstrap

import (
	"go.uber.org/fx"
	"nbox/internal/entry"
	platformaws "nbox/internal/platform/aws"
	"nbox/pkg/logger"
)

var CommonModules = fx.Options(
	fx.NopLogger,
	fx.Provide(logger.NewLogger),
	platformaws.Module,
	fx.Provide(entry.NewProcessor),
)
