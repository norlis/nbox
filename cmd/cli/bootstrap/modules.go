package bootstrap

import (
	"log/slog"

	"go.uber.org/fx"
	"nbox/internal/entry"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/version"
	"nbox/pkg/logger"
)

var CommonModules = fx.Options(
	fx.NopLogger,
	fx.Provide(logger.LoadConfig),
	fx.Provide(func(cfg logger.Config) *slog.Logger {
		return logger.New(cfg, "nbox-cli", version.OrDev())
	}),
	platformaws.Module,
	fx.Provide(entry.NewProcessor),
)
