package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"nbox/internal/nbox"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"golang.org/x/sync/errgroup"
)

var (
	ErrS3BucketNotConfigured      = errors.New("s3 bucket name is not configured (NBOX_BUCKET_NAME)")
	ErrS3BucketCheckFailed        = errors.New("s3 bucket check failed")
	ErrDynamoDBTableNotConfigured = errors.New("dynamoDB table name is not configured")
	ErrDynamoDBTableCheckFailed   = errors.New("dynamoDB table check failed")
	ErrSSMCheckFailed             = errors.New("ssm check failed")
)

// s3HeadBucketAPI is the minimal interface over *s3.Client used by S3Checker.
type s3HeadBucketAPI interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// dynamoDescribeTableAPI is the minimal interface over *dynamodb.Client used by DynamoDBChecker.
type dynamoDescribeTableAPI interface {
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

// Compile-time assertions: concrete SDK clients satisfy the interfaces.
var (
	_ s3HeadBucketAPI        = (*s3.Client)(nil)
	_ dynamoDescribeTableAPI = (*dynamodb.Client)(nil)
)

// S3Checker verifica que el bucket S3 esté accesible.
type S3Checker struct {
	client s3HeadBucketAPI
	config *nbox.Config
}

func (c *S3Checker) Check(ctx context.Context) error {
	bucketName := c.config.BucketName
	if bucketName == "" {
		return ErrS3BucketNotConfigured
	}
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: new(bucketName),
	})
	if err != nil {
		// 403 Forbidden: bucket is reachable but HeadBucket (s3:ListBucket) is denied.
		// nbox only needs object-level access, so treat this as healthy.
		var respErr *smithyhttp.ResponseError
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusForbidden {
			return nil
		}
		return fmt.Errorf("%w: %s -> %w", ErrS3BucketCheckFailed, bucketName, err)
	}
	return nil
}

// DynamoDBChecker verifica que las tablas DynamoDB existan.
type DynamoDBChecker struct {
	client dynamoDescribeTableAPI
	config *nbox.Config
}

func (c *DynamoDBChecker) Check(ctx context.Context) error {
	tableNames := []string{
		c.config.EntryTableName,
		c.config.TrackingEntryTableName,
		c.config.BoxTableName,
		c.config.ConfigTableName,
	}

	// Config guard: check before launching goroutines — it's a config error, cheap.
	if slices.Contains(tableNames, "") {
		return ErrDynamoDBTableNotConfigured
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, tableName := range tableNames {
		g.Go(func() error {
			_, err := c.client.DescribeTable(gctx, &dynamodb.DescribeTableInput{
				TableName: new(tableName),
			})
			if err != nil {
				return fmt.Errorf("%w: %s -> %w", ErrDynamoDBTableCheckFailed, tableName, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// SSMChecker verifica que SSM esté accesible.
type SSMChecker struct {
	Client *ssm.Client
	config *nbox.Config
}

func (c *SSMChecker) Check(ctx context.Context) error {
	_, err := c.Client.DescribeParameters(ctx, &ssm.DescribeParametersInput{MaxResults: new(int32(1))})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSSMCheckFailed, err)
	}
	return nil
}

func NewS3Checker(client *s3.Client, config *nbox.Config) *S3Checker {
	return &S3Checker{client: client, config: config}
}

func NewDynamoDBChecker(client *dynamodb.Client, config *nbox.Config) *DynamoDBChecker {
	return &DynamoDBChecker{client: client, config: config}
}

func NewSSMChecker(client *ssm.Client, config *nbox.Config) *SSMChecker {
	return &SSMChecker{Client: client, config: config}
}
