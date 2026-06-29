package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/box"
	"nbox/internal/box/store/spec"
	"nbox/internal/nbox"
)

// S3 implementa box.Store guardando templates en S3 y metadata en DynamoDB.
type S3 struct {
	s3             *s3.Client
	dynamodbClient *dynamodb.Client
	config         *nbox.Config
	logger         *zap.Logger
	resolver       *box.StrategyResolver
	registry       *spec.SpecRegistry
}

type boxRecord struct {
	Service  string               `dynamodbav:"Service"`
	Stage    string               `dynamodbav:"Stage"`
	Template box.Template         `dynamodbav:"Template"`
	Metadata box.TemplateMetadata `dynamodbav:"Metadata"`
}

func NewS3(
	s3Client *s3.Client,
	config *nbox.Config,
	dynamodbClient *dynamodb.Client,
	logger *zap.Logger,
	resolver *box.StrategyResolver,
	registry *spec.SpecRegistry,
) box.Store {
	return &S3{
		s3:             s3Client,
		dynamodbClient: dynamodbClient,
		config:         config,
		logger:         logger,
		resolver:       resolver,
		registry:       registry,
	}
}

func (b *S3) store(ctx context.Context, objectPath string, stage box.Stage) (*s3.PutObjectOutput, string, spec.Result, error) {
	content, hash, err := b.resolver.Process(stage.Template.Name, stage.Template.Value)
	if err != nil {
		b.logger.Error("TemplateProcessingFailed", zap.String("path", objectPath), zap.Error(err))
		return nil, "", spec.NewResult(spec.WithSyntaxError(err)), fmt.Errorf("TemplateProcessingFailed: %w", err)
	}

	format := strings.TrimPrefix(filepath.Ext(objectPath), ".")

	// Validamos contra el registro. Si no hay spec que coincida, ValidateByFilename retorna Valid=true (ignora)
	result, err := b.registry.ValidateByFilename(ctx, stage.Template.Name, content, format)
	if err != nil {
		b.logger.Error("SpecValidationEngineError", zap.String("path", objectPath), zap.Error(err))
		return nil, "", spec.NewResult(spec.WithInternalError(err)), fmt.Errorf("validation engine failed: %w", err)
	}

	if !result.Valid {
		b.logger.Warn("TemplateValidationFailed",
			zap.String("path", objectPath),
			zap.Any("errors", result.Errors),
		)

		var msgs []string
		for _, ve := range result.Errors {
			msgs = append(msgs, fmt.Sprintf("[%s]: %s", ve.Path, ve.Message))
		}
		return nil, "", result, fmt.Errorf("template validation failed:\n%s", strings.Join(msgs, "\n"))
	}

	s3Result, err := b.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: new(b.config.BucketName),
		Key:    new(objectPath),
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		return nil, hash, result, fmt.Errorf("failed to put object to S3: %w", err)
	}
	return s3Result, hash, result, nil
}

func (b *S3) BoxExists(ctx context.Context, service, stage, template string) (bool, error) {
	s3path := path.Join(service, stage, template)

	_, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: new(b.config.BucketName),
		Key:    new(s3path),
	})
	if err != nil {
		return false, fmt.Errorf("failed to check S3 object existence: %w", err)
	}
	return true, nil
}

func (b *S3) RetrieveBox(ctx context.Context, service, stage, template string) ([]byte, error) {
	s3path := path.Join(service, stage, template)
	object, err := b.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: new(b.config.BucketName),
		Key:    new(s3path),
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

func (b *S3) RetrieveBoxVersion(ctx context.Context, service, stage, template, versionId string) ([]byte, error) {
	s3path := path.Join(service, stage, template)
	input := &s3.GetObjectInput{
		Bucket:    awssdk.String(b.config.BucketName),
		Key:       awssdk.String(s3path),
		VersionId: awssdk.String(versionId),
	}
	object, err := b.s3.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get object version from S3: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(object.Body)

	body, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}
	return body, nil
}

func (b *S3) ListVersions(ctx context.Context, service, stage, template string) ([]box.TemplateVersion, error) {
	s3path := path.Join(service, stage, template)
	resp, err := b.s3.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: awssdk.String(b.config.BucketName),
		Prefix: awssdk.String(s3path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 object versions: %w", err)
	}

	versions := make([]box.TemplateVersion, 0, len(resp.Versions))
	for _, v := range resp.Versions {
		if awssdk.ToString(v.Key) != s3path {
			continue
		}
		size := int64(0)
		if v.Size != nil {
			size = *v.Size
		}
		lastModified := time.Time{}
		if v.LastModified != nil {
			lastModified = *v.LastModified
		}
		versions = append(versions, box.TemplateVersion{
			VersionId:    awssdk.ToString(v.VersionId),
			LastModified: lastModified,
			Size:         size,
			IsLatest:     awssdk.ToBool(v.IsLatest),
		})
	}
	return versions, nil
}

func (b *S3) UpsertBox(ctx context.Context, bx *box.Box) map[string]spec.Result {
	result := make(map[string]spec.Result)

	var item map[string]types.AttributeValue

	for stageName, stage := range bx.Stage {
		name := stage.Template.Name
		s3path := path.Join(bx.Service, stageName, stage.Template.Name)

		stage.Template.Name = s3path
		bx.Stage[stageName] = stage

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

		UpdatedBy := application.ActorFromContext(ctx)

		item, _ = attributevalue.MarshalMap(boxRecord{
			Service: bx.Service,
			Stage:   stageName,
			Template: box.Template{
				Name:  s3path,
				Value: name,
			},
			Metadata: box.TemplateMetadata{
				Version:   versionId,
				UpdatedAt: time.Now().UTC(),
				UpdatedBy: UpdatedBy,
				Hash:      hash,
			},
		})
		_, err = b.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: new(b.config.BoxTableName), Item: item,
		})
		if err != nil {
			b.logger.Warn("ErrDbStoreTemplate", zap.String("path", s3path), zap.Error(err))
		}
	}
	return result
}

func (b *S3) List(ctx context.Context) ([]box.Box, error) {
	boxes := map[string]box.Box{}
	results := make([]box.Box, 0)

	scan, err := b.dynamodbClient.Scan(ctx, &dynamodb.ScanInput{
		TableName:              new(b.config.BoxTableName),
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan DynamoDB table: %w", err)
	}

	for _, i := range scan.Items {
		var record boxRecord
		err = attributevalue.UnmarshalMap(i, &record)
		if err != nil {
			continue
		}

		_, ok := boxes[record.Service]
		if !ok {
			boxes[record.Service] = box.Box{Service: record.Service, Stage: map[string]box.Stage{}}
		}
		boxes[record.Service].Stage[record.Stage] = box.Stage{Template: record.Template, Metadata: record.Metadata}
	}

	for _, bx := range boxes {
		results = append(results, bx)
	}

	return results, nil
}

func (b *S3) Detail(ctx context.Context, service, stage string) (*box.Stage, error) {
	p, _ := attributevalue.Marshal(service)
	k, _ := attributevalue.Marshal(stage)

	resp, err := b.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		Key:            map[string]types.AttributeValue{"Service": p, "Stage": k},
		TableName:      new(b.config.BoxTableName),
		ConsistentRead: new(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}
	if resp.Item == nil {
		return nil, nil // Not found
	}

	record := &boxRecord{}
	if err = attributevalue.UnmarshalMap(resp.Item, record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}
	return &box.Stage{
		Template: record.Template,
		Metadata: record.Metadata,
	}, nil
}
