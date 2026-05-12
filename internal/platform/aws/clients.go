package aws

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
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
