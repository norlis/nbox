package aws

import "go.uber.org/fx"

// Module expone los providers AWS (config, clients, kit, health checkers) para fx.
var Module = fx.Module("platform.aws",
	fx.Provide(
		NewConfig,
		NewS3,
		NewDynamoDB,
		NewSSM,
		NewDynamoDBKit,
		NewS3Checker,
		NewDynamoDBChecker,
		NewSSMChecker,
	),
)
