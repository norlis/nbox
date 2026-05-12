package bootstrap

import (
	"go.uber.org/fx"
	"nbox/internal/entry"
	platformaws "nbox/internal/platform/aws"
	prefixstore "nbox/internal/prefix/store"
	"nbox/pkg/logger"
)

var CommonModules = fx.Options(
	fx.NopLogger, // Silenciar logs de Fx
	fx.Provide(logger.NewLogger),

	// platform: AWS config, clients, kit, health
	platformaws.Module,

	// Use case
	fx.Provide(entry.NewProcessor),

	// Adapters
	fx.Provide(prefixstore.NewDynamoDB),
)
