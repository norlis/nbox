package bootstrap

import (
	"nbox/internal/adapters/amazonaws"
	"nbox/internal/adapters/storage"
	"nbox/internal/usecases"
	"nbox/pkg/logger"

	"go.uber.org/fx"
)

var CommonModules = fx.Options(
	fx.NopLogger, // Silenciar logs de Fx
	fx.Provide(logger.NewLogger),
	fx.Provide(amazonaws.NewAwsConfig),
	fx.Provide(amazonaws.NewDynamodbClient),

	// Use case
	fx.Provide(usecases.NewPathUseCase),

	// Adapters
	fx.Provide(storage.NewDynamodbKit),
	fx.Provide(storage.NewDynamoPrefixConfigRepository),
)
