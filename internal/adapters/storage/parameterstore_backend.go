package storage

import (
	"context"
	"fmt"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"nbox/internal/usecases"
	"nbox/pkg/resiliency"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"go.uber.org/zap"
)

const (
	// MaxSSMConcurrency limita las peticiones simultáneas a AWS SSM.
	// El tier estándar soporta ~40 TPS. Con 5 hilos concurrentes nos mantenemos
	// en un margen seguro para evitar ThrottlingException.
	MaxSSMConcurrency = 5
)

type ParameterStoreBackend struct {
	client      *ssm.Client
	pathUseCase *usecases.PathUseCase
	logger      *zap.Logger
	config      *application.Config
	guard       *resiliency.Guard
}

func NewParameterStoreBackend(
	client *ssm.Client,
	pathUseCase *usecases.PathUseCase,
	logger *zap.Logger,
	config *application.Config,
) *ParameterStoreBackend {
	guard := resiliency.NewGuard(resiliency.GuardConfig{
		MaxConcurrency: MaxSSMConcurrency,
		MaxRetries:     3,
		BaseDelay:      200 * time.Millisecond,
		Name:           "ssm-upsert",
	})
	return &ParameterStoreBackend{
		client:      client,
		pathUseCase: pathUseCase,
		logger:      logger,
		config:      config,
		guard:       guard,
	}
}

func (p *ParameterStoreBackend) Upsert(ctx context.Context, entries []models.Entry) operations.Results {

	ch := make(chan operations.Result, len(entries))
	var wg sync.WaitGroup

	for _, entry := range entries {
		wg.Go(func() {
			err := p.guard.Execute(ctx, func() error {
				res := p.Send(ctx, entry)
				if res.Err != nil {
					return res.Err
				}

				ch <- res
				return nil
			})
			if err != nil {
				ch <- operations.Result{
					Key: entry.Key,
					Err: err,
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make(operations.Results)

	for res := range ch {
		if res.Err != nil {
			p.logger.Error("ErrSecureUpsert",
				zap.String("key", res.Key),
				zap.Error(res.Err),
			)
		}

		results.AddWithOutput(res.Key, res.Action, res.Err, res.Output)
	}

	return results
}

func (p *ParameterStoreBackend) Resolve(ctx context.Context, key string) ([]byte, error) {
	entry, err := p.Retrieve(ctx, key)
	if err != nil || entry == nil {
		return nil, err
	}
	return []byte(entry.Value), nil
}

func (p *ParameterStoreBackend) Retrieve(ctx context.Context, key string, opts ...domain.RetrieveOption) (*models.Entry, error) {
	key = p.pathUseCase.NormalizeKey(key)

	config := &domain.RetrieveConfig{
		Decrypt: false,
	}

	for _, opt := range opts {
		opt(config)
	}

	result, err := p.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(key),
		WithDecryption: aws.Bool(config.Decrypt),
	})

	if err != nil {
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return nil, fmt.Errorf("%w: %s", domain.ErrEntryNotFound, key)
		}
		return nil, fmt.Errorf("failed to get parameter from SSM: %w", err)
	}

	return &models.Entry{
		Key:    key,
		Value:  *result.Parameter.Value,
		Secure: result.Parameter.Type == types.ParameterTypeSecureString,
	}, nil
}

func (p *ParameterStoreBackend) Delete(ctx context.Context, key string) error {
	key = p.pathUseCase.NormalizeKey(key)

	err := p.guard.Execute(ctx, func() error {
		_, err := p.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(key),
		})
		return err
	})

	if err != nil {
		// ParameterNotFound is not an error for Delete (idempotent)
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return nil
		}
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

func (p *ParameterStoreBackend) Send(ctx context.Context, entry models.Entry) operations.Result {
	in := p.prepare(ctx, entry, p.config.ParameterStoreKeyId)
	out, err := p.client.PutParameter(ctx, in)
	if err != nil {
		return operations.Result{Key: entry.Key, Err: err}
	}

	opType := operations.Updated
	if out.Version == 1 {
		opType = operations.Created
		// Add tags after creation (can't use Tags + Overwrite together)
		p.addTags(ctx, in.Name, entry)
	}

	entryToIndex := entry
	entryToIndex.Value = p.getArn(entry.Key)

	if entryToIndex.Metadata == nil {
		entryToIndex.Metadata = &models.Metadata{}
	}
	entryToIndex.Metadata.StorageBackend = backend.BackendParameterStore
	if entry.Secure {
		entryToIndex.Metadata.StorageBackend = backend.BackendParameterStoreSecure
	}
	entryToIndex.Secure = entry.Secure

	return operations.Result{Key: entry.Key, Action: opType, Err: nil, Output: &entryToIndex}
}

func (p *ParameterStoreBackend) getArn(rawKey string) string {
	normalizedKey := p.pathUseCase.NormalizeKey(rawKey)

	if p.config.ParameterShortArn {
		if !strings.HasPrefix(normalizedKey, "/") {
			return "/" + normalizedKey
		}
		return normalizedKey
	}

	cleanName := strings.TrimPrefix(normalizedKey, "/")

	return fmt.Sprintf(
		"arn:aws:ssm:%s:%s:parameter/%s",
		p.config.RegionName,
		p.config.AccountId,
		cleanName,
	)
}

func (p *ParameterStoreBackend) prepare(ctx context.Context, entry models.Entry, parameterStoreKeyId string) *ssm.PutParameterInput {
	key := p.pathUseCase.NormalizeKey(entry.Key)

	// Default: plain text (String type)
	// Note: This backend CAN handle SecureString when entry.Secure=true,
	// but ParameterStoreSecureBackend wrapper should enforce Secure=true for vault prefixes
	parameterType := types.ParameterTypeString
	parameterTier := types.ParameterTierStandard

	if entry.Secure {
		parameterType = types.ParameterTypeSecureString
	}

	// Use Advanced tier for values > 4KB (Standard tier limit)
	if len(entry.Value) > 4096 {
		parameterTier = types.ParameterTierAdvanced
	}

	parameterInput := &ssm.PutParameterInput{
		Name:      aws.String(key),
		Value:     aws.String(entry.Value),
		Type:      parameterType,
		Tier:      parameterTier,
		Overwrite: aws.Bool(true),
		// Note: Can't use Tags with Overwrite=true
		// Tags will be added via AddTagsToResource after creation (when version==1)
	}

	// Only set KMS Key ID for SecureString type
	if entry.Secure && parameterStoreKeyId != "" {
		parameterInput.KeyId = aws.String(parameterStoreKeyId)
	}

	return parameterInput
}

// addTags adds tags to a parameter after creation.
// AWS doesn't allow Tags + Overwrite together in PutParameter.
func (p *ParameterStoreBackend) addTags(ctx context.Context, key *string, entry models.Entry) {
	updatedBy := "ghost"
	if user, ok := application.UserFromContext(ctx); ok {
		updatedBy = user.Name
	}

	_, err := p.client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
		ResourceId:   key,
		ResourceType: types.ResourceTypeForTaggingParameter,
		Tags: []types.Tag{
			{Key: aws.String("project"), Value: aws.String("nbox")},
			{Key: aws.String("username"), Value: aws.String(updatedBy)},
		},
	})

	if err != nil {
		p.logger.Warn("Failed to add tags to parameter",
			zap.String("key", *key),
			zap.Error(err))
	}
}
