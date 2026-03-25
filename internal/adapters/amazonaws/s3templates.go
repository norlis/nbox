package amazonaws

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/boxspec"
	"nbox/internal/domain/models"
	"nbox/internal/domain/strategies"
	"nbox/internal/domain/validation"
)

type s3TemplateStore struct {
	s3             *s3.Client
	dynamodbClient *dynamodb.Client
	config         *application.Config
	logger         *zap.Logger
	resolver       *strategies.StrategyResolver
	registry       *boxspec.SpecRegistry
}

type BoxRecord struct {
	Service  string                  `dynamodbav:"Service"`
	Stage    string                  `dynamodbav:"Stage"`
	Template models.Template         `dynamodbav:"Template"`
	Metadata models.TemplateMetadata `dynamodbav:"Metadata"`
}

func NewS3TemplateStore(
	s3Client *s3.Client,
	config *application.Config,
	dynamodbClient *dynamodb.Client,
	logger *zap.Logger,
	resolver *strategies.StrategyResolver,
	registry *boxspec.SpecRegistry,
) domain.TemplateAdapter {
	return &s3TemplateStore{
		s3:             s3Client,
		dynamodbClient: dynamodbClient,
		config:         config,
		logger:         logger,
		resolver:       resolver,
		registry:       registry,
	}
}

func (b *s3TemplateStore) store(ctx context.Context, objectPath string, stage models.Stage) (*s3.PutObjectOutput, string, validation.Result, error) {
	content, hash, err := b.resolver.Process(stage.Template.Name, stage.Template.Value)
	if err != nil {
		b.logger.Error("TemplateProcessingFailed", zap.String("path", objectPath), zap.Error(err))
		return nil, "", validation.NewResult(validation.WithSyntaxError(err)), fmt.Errorf("TemplateProcessingFailed: %w", err)
	}

	format := strings.TrimPrefix(filepath.Ext(objectPath), ".")

	// Validamos contra el registro. Si no hay spec que coincida, ValidateByFilename retorna Valid=true (ignora)
	result, err := b.registry.ValidateByFilename(ctx, stage.Template.Name, content, format)
	if err != nil {
		// Error técnico al intentar validar (ej. problema con el motor CUE)
		b.logger.Error("SpecValidationEngineError", zap.String("path", objectPath), zap.Error(err))
		return nil, "", validation.NewResult(validation.WithInternalError(err)), fmt.Errorf("validation engine failed: %w", err)
	}

	if !result.Valid {
		// La validación de negocio falló (ej. falta campo 'family' en ECS)
		b.logger.Warn("TemplateValidationFailed",
			zap.String("path", objectPath),
			zap.Any("errors", result.Errors),
		)

		// Construimos un mensaje de error legible
		var msgs []string
		for _, ve := range result.Errors {
			msgs = append(msgs, fmt.Sprintf("[%s]: %s", ve.Path, ve.Message))
		}
		return nil, "", result, fmt.Errorf("template validation failed:\n%s", strings.Join(msgs, "\n"))
	}

	s3Result, err := b.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(objectPath),
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		return nil, hash, result, fmt.Errorf("failed to put object to S3: %w", err)
	}
	return s3Result, hash, result, nil
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

func (b *s3TemplateStore) UpsertBox(ctx context.Context, box *models.Box) map[string]validation.Result {
	// result := make([]string, 0)
	result := make(map[string]validation.Result)

	var item map[string]types.AttributeValue

	for stageName, stage := range box.Stage {
		name := stage.Template.Name
		// path := fmt.Sprintf("%s/%s/%s", box.Service, stageName, stage.Template.Name)
		s3path := path.Join(box.Service, stageName, stage.Template.Name)

		stage.Template.Name = s3path
		box.Stage[stageName] = stage

		s3Result, hash, validResult, err := b.store(ctx, s3path, stage)
		result[s3path] = validResult
		if err != nil {
			b.logger.Error("ErrStoreTemplate", zap.String("path", s3path), zap.Error(err))
			continue
		}

		if !validResult.Valid {
			continue
		}

		versionId := ""
		if s3Result != nil && s3Result.VersionId != nil {
			versionId = *s3Result.VersionId
		}

		UpdatedBy := "ghost"
		user, ok := application.UserFromContext(ctx)

		if ok {
			UpdatedBy = user.Name
		}

		if err == nil {
			item, _ = attributevalue.MarshalMap(BoxRecord{
				Service: box.Service,
				Stage:   stageName,
				Template: models.Template{
					Name:  s3path,
					Value: name,
				},
				Metadata: models.TemplateMetadata{
					Version:   versionId,
					UpdatedAt: time.Now().UTC(),
					UpdatedBy: UpdatedBy,
					Hash:      hash,
				},
			})
			_, err = b.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String(b.config.BoxTableName), Item: item,
			})
			if err != nil {
				b.logger.Warn("ErrDbStoreTemplate", zap.String("path", s3path), zap.Error(err))
			}
		}

		// if err == nil {
		//	//result[s3path] = append(result, s3path)
		//	result = append(result, s3path)
		//}
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
