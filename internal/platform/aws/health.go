package aws

import (
	"context"
	"errors"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"nbox/internal/application"
)

var (
	ErrS3BucketNotConfigured      = errors.New("s3 bucket name is not configured (NBOX_BUCKET_NAME)")
	ErrS3BucketCheckFailed        = errors.New("s3 bucket check failed")
	ErrDynamoDBTableNotConfigured = errors.New("dynamoDB table name is not configured")
	ErrDynamoDBTableCheckFailed   = errors.New("dynamoDB table check failed")
	ErrSSMCheckFailed             = errors.New("ssm check failed")
)

// S3Checker verifica que el bucket S3 esté accesible.
type S3Checker struct {
	Client *s3.Client
	config *application.Config
}

func (c *S3Checker) Check() error {
	bucketName := c.config.BucketName
	if bucketName == "" {
		return ErrS3BucketNotConfigured
	}

	_, err := c.Client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: awssdk.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("%w: %s -> %w", ErrS3BucketCheckFailed, bucketName, err)
	}
	return nil
}

// DynamoDBChecker verifica que las tablas DynamoDB existan.
type DynamoDBChecker struct {
	Client *dynamodb.Client
	config *application.Config
}

func (c *DynamoDBChecker) Check() error {
	tableNames := []string{
		c.config.EntryTableName,
		c.config.TrackingEntryTableName,
		c.config.BoxTableName,
		c.config.PrefixConfigTableName,
	}

	for _, tableName := range tableNames {
		if tableName == "" {
			return ErrDynamoDBTableNotConfigured
		}

		_, err := c.Client.DescribeTable(context.TODO(), &dynamodb.DescribeTableInput{
			TableName: awssdk.String(tableName),
		})
		if err != nil {
			return fmt.Errorf("%w: %s -> %w", ErrDynamoDBTableCheckFailed, tableName, err)
		}
	}
	return nil
}

// SSMChecker verifica que SSM esté accesible.
type SSMChecker struct {
	Client *ssm.Client
	config *application.Config
}

func (c *SSMChecker) Check() error {
	_, err := c.Client.DescribeParameters(context.TODO(), &ssm.DescribeParametersInput{MaxResults: awssdk.Int32(1)})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSSMCheckFailed, err)
	}
	return nil
}

func NewS3Checker(client *s3.Client, config *application.Config) *S3Checker {
	return &S3Checker{Client: client, config: config}
}

func NewDynamoDBChecker(client *dynamodb.Client, config *application.Config) *DynamoDBChecker {
	return &DynamoDBChecker{Client: client, config: config}
}

func NewSSMChecker(client *ssm.Client, config *application.Config) *SSMChecker {
	return &SSMChecker{Client: client, config: config}
}
