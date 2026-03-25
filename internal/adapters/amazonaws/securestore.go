package amazonaws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
)

var ErrParameterNotFound = errors.New("parameter not found or has no value")

type Result models.Exchange[*ssm.PutParameterOutput, *models.Entry]

// Deprecated.
type secureParameterStore struct {
	client *ssm.Client
	config *application.Config
	logger *zap.Logger
}

// NewSecureParameterStore
// Deprecated.
func NewSecureParameterStore(client *ssm.Client, config *application.Config, logger *zap.Logger) domain.SecretAdapter {
	return &secureParameterStore{client: client, config: config, logger: logger.Named("secure_parameter_store")}
}

func (s *secureParameterStore) RetrieveSecretValue(ctx context.Context, key string) (*models.Entry, error) {
	output, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(key),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter from SSM: %w", err)
	}

	if output.Parameter == nil {
		s.logger.Error("ErrParameterNotFound", zap.String("key", key), zap.Error(err))
		return nil, ErrParameterNotFound
	}

	return &models.Entry{
		Key:    key,
		Value:  *output.Parameter.Value,
		Secure: true,
	}, nil
}

func (s *secureParameterStore) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	ch := make(chan operations.Result)
	wg := sync.WaitGroup{}
	wg.Add(len(entries))

	for _, entry := range entries {
		go func(c chan operations.Result, g *sync.WaitGroup, e models.Entry, x context.Context) {
			defer g.Done()
			c <- s.Send(x, e)
		}(ch, &wg, entry, ctx)
	}

	go func(g *sync.WaitGroup, c chan operations.Result) {
		g.Wait()
		defer close(c)
	}(&wg, ch)

	results := make(operations.Results, len(entries))

	for result := range ch {
		if result.Err != nil {
			s.logger.Error("ErrSecureUpsert",
				zap.String("key", result.Key),
				zap.Error(result.Err),
			)
		}
		results[result.Key] = result
	}

	return results
}

func (s *secureParameterStore) Send(ctx context.Context, entry models.Entry) operations.Result {
	in := prepareSecret(entry, s.config.ParameterStoreKeyId)
	out, err := s.client.PutParameter(ctx, in)
	if err != nil {
		return operations.Result{Key: entry.Key, Err: err}
	}

	opType := operations.Updated
	if out.Version == 1 {
		opType = operations.Created
		s.AddTags(ctx, in.Name)
	}

	return operations.Result{Key: entry.Key, Action: opType, Err: nil}
}

func (s *secureParameterStore) AddTags(ctx context.Context, key *string) {
	_, err := s.client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
		ResourceId:   key,
		ResourceType: types.ResourceTypeForTaggingParameter,
		Tags:         []types.Tag{{Key: aws.String("project"), Value: aws.String("nbox")}},
	})
	if err != nil {
		s.logger.Warn("ErrSecureAddingTags", zap.Error(err), zap.String("key", *key))
	}
}

func prepareSecret(entry models.Entry, parameterStoreKeyId string) *ssm.PutParameterInput {
	key := entry.Key

	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}

	parameterInput := &ssm.PutParameterInput{
		Name:      aws.String(key),
		Value:     aws.String(entry.Value),
		Type:      types.ParameterTypeSecureString,
		Tier:      types.ParameterTierStandard,
		Overwrite: aws.Bool(true),
	}

	// when the value exceeds 4 KB
	if len(entry.Value) > 4096 {
		parameterInput.Tier = types.ParameterTierAdvanced
	}

	if parameterStoreKeyId != "" {
		parameterInput.KeyId = aws.String(parameterStoreKeyId)
	}

	return parameterInput
}
