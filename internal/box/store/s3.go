package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/norlis/httpgate/logging"
	"nbox/internal/application"
	"nbox/internal/box"
	"nbox/internal/box/store/spec"
	"nbox/internal/logfields"
	"nbox/internal/nbox"
)

// S3 implementa box.Store guardando templates en S3 y metadata en DynamoDB.
type S3 struct {
	s3             *s3.Client
	dynamodbClient *dynamodb.Client
	config         *nbox.Config
	logger         *slog.Logger
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
	logger *slog.Logger,
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
		return nil, "", spec.NewResult(spec.WithSyntaxError(err)), fmt.Errorf("TemplateProcessingFailed: %w", err)
	}

	format := strings.TrimPrefix(filepath.Ext(objectPath), ".")

	// Validamos contra el registro. Si no hay spec que coincida, ValidateByFilename retorna Valid=true (ignora)
	result, err := b.registry.ValidateByFilename(ctx, stage.Template.Name, content, format)
	if err != nil {
		return nil, "", spec.NewResult(spec.WithInternalError(err)), fmt.Errorf("validation engine failed: %w", err)
	}

	if !result.Valid {
		b.logger.WarnContext(ctx, "template validation failed",
			slog.String(logfields.KeyNboxTemplate, objectPath),
			slog.Any("errors", result.Errors),
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
			// Schema errors (user input) are already WARN-logged in store(); anything
			// else here is a genuine internal/engine fault that never reaches the
			// presenter as a log (it's only serialized as HTTP 400/500 JSON).
			if validResult.Kind != spec.KindSchemaError {
				b.logger.ErrorContext(ctx, "template processing failed",
					slog.String(logfields.KeyNboxService, bx.Service),
					slog.String(logfields.KeyNboxStage, stageName),
					slog.String(logfields.KeyNboxTemplate, s3path),
					logging.Err(err),
				)
			}
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
			b.logger.ErrorContext(ctx, "template store write failed",
				slog.String(logfields.KeyNboxTemplate, s3path),
				logging.Err(err),
			)
		}
	}
	return result
}

func (b *S3) List(ctx context.Context) ([]box.Box, error) {
	boxes := map[string]box.Box{}

	paginator := dynamodb.NewScanPaginator(b.dynamodbClient, &dynamodb.ScanInput{
		TableName: new(b.config.BoxTableName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan DynamoDB table: %w", err)
		}

		var records []boxRecord
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			continue
		}
		for _, record := range records {
			if _, ok := boxes[record.Service]; !ok {
				boxes[record.Service] = box.Box{Service: record.Service, Stage: map[string]box.Stage{}}
			}
			boxes[record.Service].Stage[record.Stage] = box.Stage{Template: record.Template, Metadata: record.Metadata}
		}
	}

	services := make([]string, 0, len(boxes))
	for svc := range boxes {
		services = append(services, svc)
	}
	sort.Strings(services)

	results := make([]box.Box, 0, len(boxes))
	for _, svc := range services {
		results = append(results, boxes[svc])
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
