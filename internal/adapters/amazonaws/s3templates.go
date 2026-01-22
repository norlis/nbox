package amazonaws

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type s3TemplateStore struct {
	s3             *s3.Client
	dynamodbClient *dynamodb.Client
	config         *application.Config
	logger         *zap.Logger
}

type BoxRecord struct {
	Service  string          `dynamodbav:"Service"`
	Stage    string          `dynamodbav:"Stage"`
	Template models.Template `dynamodbav:"Template"`
}

func NewS3TemplateStore(s3Client *s3.Client, config *application.Config, dynamodbClient *dynamodb.Client, logger *zap.Logger) domain.TemplateAdapter {
	return &s3TemplateStore{
		s3:             s3Client,
		dynamodbClient: dynamodbClient,
		config:         config,
		logger:         logger,
	}
}

func (b *s3TemplateStore) store(ctx context.Context, objectPath string, stage models.Stage) (*s3.PutObjectOutput, error) {
	var out bytes.Buffer

	decoded, err := base64.StdEncoding.DecodeString(stage.Template.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	err = json.Indent(&out, decoded, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format JSON: %w", err)
	}

	result, err := b.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(objectPath),
		Body:   bytes.NewReader(out.Bytes()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put object to S3: %w", err)
	}
	return result, nil
}

func (b *s3TemplateStore) BoxExists(ctx context.Context, service, stage, template string) (bool, error) {
	// path := fmt.Sprintf("%s/%s/%s", service, stage, template)
	s3path := path.Join(service, stage, template)

	_, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(s3path),
	})
	if err != nil {
		return false, fmt.Errorf("failed to check S3 object existence: %w", err)
	}
	return true, nil
}

func (b *s3TemplateStore) RetrieveBox(ctx context.Context, service, stage, template string) ([]byte, error) {
	// path := fmt.Sprintf("%s/%s/%s", service, stage, template)
	s3path := path.Join(service, stage, template)
	object, err := b.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(s3path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(object.Body)

	body, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}

	return body, nil
}

func (b *s3TemplateStore) UpsertBox(ctx context.Context, box *models.Box) []string {
	result := make([]string, 0)
	var item map[string]types.AttributeValue

	for stageName, stage := range box.Stage {
		name := stage.Template.Name
		// path := fmt.Sprintf("%s/%s/%s", box.Service, stageName, stage.Template.Name)
		s3path := path.Join(box.Service, stageName, stage.Template.Name)

		stage.Template.Name = s3path
		box.Stage[stageName] = stage
		_, err := b.store(ctx, s3path, stage)
		if err != nil {
			b.logger.Error("ErrStoreTemplate", zap.String("path", s3path), zap.Error(err))
		}

		if err == nil {
			item, _ = attributevalue.MarshalMap(BoxRecord{
				Service: box.Service,
				Stage:   stageName,
				Template: models.Template{
					Name:  s3path,
					Value: name,
				},
			})
			_, err = b.dynamodbClient.PutItem(context.TODO(), &dynamodb.PutItemInput{
				TableName: aws.String(b.config.BoxTableName), Item: item,
			})
			if err != nil {
				b.logger.Warn("ErrDbStoreTemplate", zap.String("path", s3path), zap.Error(err))
			}
		}

		if err == nil {
			result = append(result, s3path)
		}
	}
	return result
}

func (b *s3TemplateStore) List(ctx context.Context) ([]models.Box, error) {
	boxes := map[string]models.Box{}
	results := make([]models.Box, 0)

	scan, err := b.dynamodbClient.Scan(ctx, &dynamodb.ScanInput{
		TableName:              aws.String(b.config.BoxTableName),
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan DynamoDB table: %w", err)
	}

	for _, i := range scan.Items {
		var record BoxRecord
		err = attributevalue.UnmarshalMap(i, &record)
		if err != nil {
			continue
		}

		_, ok := boxes[record.Service]
		if !ok {
			boxes[record.Service] = models.Box{Service: record.Service, Stage: map[string]models.Stage{}}
		}
		boxes[record.Service].Stage[record.Stage] = models.Stage{Template: record.Template}
	}

	for _, box := range boxes {
		results = append(results, box)
	}

	return results, nil
}
