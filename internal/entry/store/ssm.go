package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	eventdrivenaws "github.com/norlis/event-driven/pkg/transport/aws"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	"nbox/internal/nbox"
	"nbox/internal/prefix"
	"nbox/pkg/resiliency"
)

const (
	// MaxSSMConcurrency limits concurrent requests to AWS SSM.
	// The standard tier supports ~40 TPS. With 5 concurrent threads we stay
	// within a safe margin to avoid ThrottlingException.
	MaxSSMConcurrency = 5
)

// SSM implementa entry.PartialStore usando AWS Systems Manager Parameter Store.
type SSM struct {
	client      *ssm.Client
	pathUseCase *entry.Processor
	logger      *zap.Logger
	config      *nbox.Config
	identity    *eventdrivenaws.Identity
	guard       *resiliency.Guard
}

func NewSSM(
	client *ssm.Client,
	pathUseCase *entry.Processor,
	logger *zap.Logger,
	config *nbox.Config,
	identity *eventdrivenaws.Identity,
) *SSM {
	guard := resiliency.NewGuard(resiliency.GuardConfig{
		MaxConcurrency: MaxSSMConcurrency,
		MaxRetries:     3,
		BaseDelay:      200 * time.Millisecond,
		Name:           "ssm-upsert",
	})
	return &SSM{
		client:      client,
		pathUseCase: pathUseCase,
		logger:      logger,
		config:      config,
		identity:    identity,
		guard:       guard,
	}
}

func (p *SSM) BackendType() prefix.StorageBackendType {
	return prefix.BackendParameterStore
}

func (p *SSM) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	ch := make(chan entry.Result, len(entries))
	var wg sync.WaitGroup

	for _, en := range entries {
		wg.Go(func() {
			err := p.guard.Execute(ctx, func() error {
				res := p.Send(ctx, en)
				if res.Err != nil {
					return res.Err
				}

				ch <- res
				return nil
			})
			if err != nil {
				ch <- entry.Result{
					Key: en.Key,
					Err: err,
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make(entry.Results)

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

func (p *SSM) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	e, err := p.Retrieve(ctx, key)
	if err != nil || e == nil {
		return nil, err
	}
	return e, nil
}

func (p *SSM) Retrieve(ctx context.Context, key string, opts ...entry.RetrieveOption) (*entry.Entry, error) {
	key = p.pathUseCase.NormalizeKey(key)

	cfg := &entry.RetrieveConfig{
		Decrypt: false,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	result, err := p.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           new(key),
		WithDecryption: new(cfg.Decrypt),
	})
	if err != nil {
		var notFound *types.ParameterNotFound
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%w: %s", entry.ErrEntryNotFound, key)
		}
		return nil, fmt.Errorf("failed to get parameter from SSM: %w", err)
	}

	return &entry.Entry{
		Key:    key,
		Value:  *result.Parameter.Value,
		Secure: result.Parameter.Type == types.ParameterTypeSecureString,
	}, nil
}

func (p *SSM) Delete(ctx context.Context, key string) error {
	key = p.pathUseCase.NormalizeKey(key)

	err := p.guard.Execute(ctx, func() error {
		_, err := p.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: new(key),
		})
		return err
	})
	if err != nil {
		// ParameterNotFound is not an error for Delete (idempotent)
		var notFound *types.ParameterNotFound
		if errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

func (p *SSM) Send(ctx context.Context, en entry.Entry) entry.Result {
	in := p.prepare(ctx, en, p.config.ParameterStoreKeyId)
	out, err := p.client.PutParameter(ctx, in)
	if err != nil {
		return entry.Result{Key: en.Key, Err: err}
	}

	opType := entry.Updated
	if out.Version == 1 {
		opType = entry.Created
		// Add tags after creation (can't use Tags + Overwrite together)
		p.addTags(ctx, in.Name, en)
	}

	entryToIndex := en
	entryToIndex.Value = p.getArn(en.Key)

	if entryToIndex.Metadata == nil {
		entryToIndex.Metadata = &entry.Metadata{}
	}
	entryToIndex.Metadata.Version = out.Version
	entryToIndex.Metadata.StorageBackend = prefix.BackendParameterStore
	if en.Secure {
		entryToIndex.Metadata.StorageBackend = prefix.BackendParameterStoreSecure
	}
	entryToIndex.Secure = en.Secure

	return entry.Result{Key: en.Key, Action: opType, Err: nil, Output: &entryToIndex}
}

func (p *SSM) getArn(rawKey string) string {
	normalizedKey := strings.ToLower(p.pathUseCase.NormalizeKey(rawKey))

	if p.config.ParameterShortArn {
		if !strings.HasPrefix(normalizedKey, "/") {
			return "/" + normalizedKey
		}
		return normalizedKey
	}

	cleanName := strings.TrimPrefix(normalizedKey, "/")

	region, _ := p.identity.Region()
	accountID, _ := p.identity.AccountID(context.Background())
	return fmt.Sprintf(
		"arn:aws:ssm:%s:%s:parameter/%s",
		region,
		accountID,
		cleanName,
	)
}

func (p *SSM) prepare(_ context.Context, en entry.Entry, parameterStoreKeyId string) *ssm.PutParameterInput {
	key := strings.ToLower(p.pathUseCase.NormalizeKey(en.Key))

	parameterType := types.ParameterTypeString
	parameterTier := types.ParameterTierStandard

	if en.Secure {
		parameterType = types.ParameterTypeSecureString
	}

	if len(en.Value) > 4096 {
		parameterTier = types.ParameterTierAdvanced
	}

	parameterInput := &ssm.PutParameterInput{
		Name:      new(key),
		Value:     new(en.Value),
		Type:      parameterType,
		Tier:      parameterTier,
		Overwrite: new(true),
	}

	if en.Secure && parameterStoreKeyId != "" {
		parameterInput.KeyId = new(parameterStoreKeyId)
	}

	return parameterInput
}

// addTags adds tags to a parameter after creation.
// AWS doesn't allow Tags + Overwrite together in PutParameter.
func (p *SSM) addTags(ctx context.Context, key *string, _ entry.Entry) {
	updatedBy := application.ActorFromContext(ctx)

	_, err := p.client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
		ResourceId:   key,
		ResourceType: types.ResourceTypeForTaggingParameter,
		Tags: []types.Tag{
			{Key: new("project"), Value: new("nbox")},
			{Key: new("username"), Value: new(updatedBy)},
		},
	})
	if err != nil {
		p.logger.Warn("Failed to add tags to parameter",
			zap.String("key", *key),
			zap.Error(err))
	}
}
