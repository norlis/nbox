package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
)

// maxRetryAttempts is the maximum number of SDK retryer attempts.
// Covers ThrottlingException, ProvisionedThroughputExceededException,
// RequestLimitExceeded, TooManyRequestsException, network errors and 5xx,
// with exponential backoff, jitter, and adaptive client-side rate limiting.
const maxRetryAttempts = 8

// NewConfig loads the default AWS configuration with adaptive retry.
func NewConfig() (*awssdk.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithDefaultRegion("us-east-1"),
		config.WithRetryer(func() awssdk.Retryer {
			return retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
				o.StandardOptions = append(o.StandardOptions, func(so *retry.StandardOptions) {
					so.MaxAttempts = maxRetryAttempts
				})
			})
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &cfg, nil
}
