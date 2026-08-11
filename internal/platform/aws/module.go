package aws

import "go.uber.org/fx"

// CoreModule provides the AWS SDK config + Identity helper. Required by
// every binary that needs any AWS client.
var CoreModule = fx.Module("platform.aws.core",
	fx.Provide(NewConfig, NewIdentity),
)

// SQSModule provides the SQS client. Required by SQS consumers and publishers.
var SQSModule = fx.Module("platform.aws.sqs",
	fx.Provide(NewSQS),
)

// SNSModule provides the SNS client. Required by SNS publishers.
var SNSModule = fx.Module("platform.aws.sns",
	fx.Provide(NewSNS),
)

// S3Module provides the S3 client and its health checker.
var S3Module = fx.Module("platform.aws.s3",
	fx.Provide(NewS3, NewS3Checker),
)

// DynamoDBModule provides the DynamoDB client, the batch kit, and the
// health checker.
var DynamoDBModule = fx.Module("platform.aws.dynamodb",
	fx.Provide(NewDynamoDB, NewDynamoDBKit, NewDynamoDBChecker),
)

// SSMModule provides the SSM (Parameter Store) client and its health checker.
var SSMModule = fx.Module("platform.aws.ssm",
	fx.Provide(NewSSM, NewSSMChecker),
)

// Module is the "all-in" composition kept for cmd/nbox compatibility.
// New binaries should compose only the sub-modules they actually consume.
var Module = fx.Options(
	CoreModule,
	S3Module,
	DynamoDBModule,
	SSMModule,
	SNSModule,
	SQSModule,
)
