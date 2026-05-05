package amazonaws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// awsMaxRetryAttempts is the maximum number of SDK retryer attempts.
// Covers ThrottlingException, ProvisionedThroughputExceededException,
// RequestLimitExceeded, TooManyRequestsException, network errors and 5xx,
// with exponential backoff, jitter, and adaptive client-side rate limiting.
const awsMaxRetryAttempts = 8

func NewAwsConfig() (*aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithDefaultRegion("us-east-1"),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
				o.StandardOptions = append(o.StandardOptions, func(so *retry.StandardOptions) {
					so.MaxAttempts = awsMaxRetryAttempts
				})
			})
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &cfg, nil
}

func NewS3Client(cfg *aws.Config) *s3.Client {
	return s3.NewFromConfig(*cfg)
}

func NewDynamodbClient(cfg *aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(*cfg)
}

func NewSsmClient(cfg *aws.Config) *ssm.Client {
	return ssm.NewFromConfig(*cfg)
}

// func NewSnsClient(cfg *aws.Config) *sns.Client {
//	return sns.NewFromConfig(*cfg)
//}
