package aws

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	eventdrivenaws "github.com/norlis/event-driven/pkg/transport/aws"
)

// NewS3 creates a new S3 client from the given AWS config.
func NewS3(cfg *awssdk.Config) *s3.Client {
	return s3.NewFromConfig(*cfg)
}

// NewDynamoDB creates a new DynamoDB client from the given AWS config.
func NewDynamoDB(cfg *awssdk.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(*cfg)
}

// NewSSM creates a new SSM client from the given AWS config.
func NewSSM(cfg *awssdk.Config) *ssm.Client {
	return ssm.NewFromConfig(*cfg)
}

// NewSNS creates a new SNS client from the given AWS config.
func NewSNS(cfg *awssdk.Config) *sns.Client {
	return sns.NewFromConfig(*cfg)
}

// NewSQS creates a new SQS client from the given AWS config.
func NewSQS(cfg *awssdk.Config) *sqs.Client {
	return sqs.NewFromConfig(*cfg)
}

// NewIdentity returns a singleton aws.Identity bound to the AWS SDK config.
// AccountID is resolved lazily via sts:GetCallerIdentity and cached via
// sync.Once. Region is read from the SDK config (AWS_REGION env, shared
// config, or IMDS).
func NewIdentity(cfg *awssdk.Config) *eventdrivenaws.Identity {
	return &eventdrivenaws.Identity{Config: cfg}
}
