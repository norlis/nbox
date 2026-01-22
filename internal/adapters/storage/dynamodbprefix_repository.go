package storage

import (
	"context"
	"fmt"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/usecases"
	"nbox/pkg/resiliency"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

type dynamoPrefixConfigRepository struct {
	client      *dynamodb.Client
	config      *application.Config
	logger      *zap.Logger
	dynamodbKit *DynamodbKit
	pathUseCase *usecases.PathUseCase
	guard       *resiliency.Guard
	tableName   string
}

// NewDynamoPrefixConfigRepository creates a new DynamoDB-backed prefix configuration repository
// with resilience patterns (retry, circuit breaker) for read operations.
func NewDynamoPrefixConfigRepository(
	client *dynamodb.Client,
	logger *zap.Logger,
	dynamodbKit *DynamodbKit,
	pathUseCase *usecases.PathUseCase,
	config *application.Config,
) domain.PrefixConfigRepository {

	guard := resiliency.NewGuard(resiliency.GuardConfig{
		MaxConcurrency:           50, // High concurrency allowed for reads
		MaxRetries:               3,
		BaseDelay:                50 * time.Millisecond,
		Name:                     "prefix-repo-reader",
		CBMaxConsecutiveFailures: 10,
	})

	return &dynamoPrefixConfigRepository{
		client:      client,
		logger:      logger,
		dynamodbKit: dynamodbKit,
		pathUseCase: pathUseCase,
		guard:       guard,
		tableName:   config.PrefixConfigTableName,
	}
}

// GetByPrefix implements hierarchical prefix matching using Longest Prefix Match (LPM).
//
// For input "development/myapp/db/password":
// 1. Generates candidates: ["development", "development/myapp", "development/myapp/db", "development/myapp/db/password"]
// 2. Fetches all candidates in single BatchGetItem
// 3. Returns the most specific match (longest prefix)
//
// Example: If configs exist for "development" and "development/myapp",
// it will return "development/myapp" as it's more specific.
func (d *dynamoPrefixConfigRepository) GetByPrefix(ctx context.Context, prefix string) (*backend.PrefixConfig, error) {
	// Validate and clean input
	cleanKey := strings.Trim(prefix, "/")
	if cleanKey == "" {
		return nil, fmt.Errorf("prefix cannot be empty")
	}

	// 1. Generate hierarchical candidates
	candidates := d.pathUseCase.Prefixes(cleanKey)
	candidates = append(candidates, cleanKey)

	d.logger.Debug("Searching prefix config",
		zap.String("prefix", prefix),
		zap.String("cleanKey", cleanKey),
		zap.Strings("candidates", candidates))

	// 2. Prepare DynamoDB keys
	keys := make([]map[string]types.AttributeValue, 0, len(candidates))
	failedMarshals := 0

	for _, c := range candidates {
		k, err := attributevalue.Marshal(c)
		if err != nil {
			d.logger.Warn("Failed to marshal candidate",
				zap.String("candidate", c),
				zap.Error(err))
			failedMarshals++
			continue
		}
		keys = append(keys, map[string]types.AttributeValue{"Prefix": k})
	}

	// Early return if all marshals failed
	if len(keys) == 0 {
		d.logger.Error("All candidate marshals failed",
			zap.String("prefix", cleanKey),
			zap.Int("failed_count", failedMarshals))
		return nil, fmt.Errorf("failed to marshal any candidates")
	}

	// 3. Execute BatchGetItem with retry/circuit breaker
	//items, err := d.batchGet(ctx, keys)
	items, err := d.dynamodbKit.BatchGet(ctx, d.tableName, keys)

	if err != nil {
		d.logger.Error("BatchGetItem failed",
			zap.String("prefix", cleanKey),
			zap.Int("key_count", len(keys)),
			zap.Error(err))
		return nil, fmt.Errorf("failed to fetch prefix configs: %w", err)
	}

	if len(items) == 0 {
		d.logger.Debug("No prefix config found",
			zap.String("prefix", cleanKey),
			zap.Strings("searched", candidates))
		return nil, domain.ErrEntryNotFound
	}

	// 4. Longest Prefix Match (LPM) with deterministic tie-breaking
	var bestMatch *backend.PrefixConfig
	maxLen := -1

	for _, item := range items {
		var config backend.PrefixConfig
		if err := attributevalue.UnmarshalMap(item, &config); err != nil {
			d.logger.Error("Failed to unmarshal config",
				zap.Any("item", item),
				zap.Error(err))
			continue
		}

		// Select longest prefix, with lexicographic tie-breaking
		// Tie-breaking ensures deterministic behavior when multiple configs
		// have the same length (e.g., "app/db" vs "app/ui")
		if len(config.Prefix) > maxLen ||
			(len(config.Prefix) == maxLen && bestMatch != nil && config.Prefix < bestMatch.Prefix) {
			bestMatch = &config
			maxLen = len(config.Prefix)
		}
	}

	if bestMatch == nil {
		d.logger.Error("All unmarshal operations failed",
			zap.String("prefix", cleanKey),
			zap.Int("item_count", len(items)))
		return nil, domain.ErrEntryNotFound
	}

	d.logger.Debug("Found best match",
		zap.String("requested", cleanKey),
		zap.String("matched", bestMatch.Prefix),
		zap.String("backend", string(bestMatch.TypeDefault)),
		zap.Int("candidates_searched", len(candidates)),
		zap.Int("configs_found", len(items)))

	return bestMatch, nil
}

// List retrieves all existing prefix configurations.
// Use cases: CLI tools, backups, cache warming.
func (d *dynamoPrefixConfigRepository) List(ctx context.Context) ([]backend.PrefixConfig, error) {
	start := time.Now()

	d.logger.Info("Scanning all prefix configs", zap.String("table", d.tableName))

	configs := make([]backend.PrefixConfig, 0)

	paginator := dynamodb.NewScanPaginator(d.client, &dynamodb.ScanInput{
		TableName: aws.String(d.tableName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("Scan page failed",
				zap.String("table", d.tableName),
				zap.Error(err))
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		var chunk []backend.PrefixConfig
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &chunk); err != nil {
			d.logger.Error("Unmarshal page failed",
				zap.String("table", d.tableName),
				zap.Int("item_count", len(page.Items)),
				zap.Error(err))
			return nil, fmt.Errorf("unmarshal failed: %w", err)
		}
		configs = append(configs, chunk...)
	}

	// Alphabetical sort for consistent output
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Prefix < configs[j].Prefix
	})

	d.logger.Info("Scan completed",
		zap.Int("count", len(configs)),
		zap.Duration("elapsed", time.Since(start)))

	return configs, nil
}

// Upsert creates or updates prefix configurations in DynamoDB using BatchWrite.
// Returns statistics about processed, failed, and skipped items.
func (d *dynamoPrefixConfigRepository) Upsert(ctx context.Context, prefixes []backend.PrefixConfig) (backend.UpsertStats, error) {
	if len(prefixes) == 0 {
		d.logger.Warn("Upsert called with empty prefixes list")
		return backend.UpsertStats{}, nil
	}

	stats := backend.UpsertStats{}
	start := time.Now()

	d.logger.Info("Upserting prefix configs",
		zap.Int("count", len(prefixes)),
		zap.String("table", d.tableName))

	requests := make([]types.WriteRequest, 0, len(prefixes))
	now := time.Now().UTC()

	for _, p := range prefixes {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = now
		}
		p.UpdatedAt = now

		item, err := attributevalue.MarshalMap(p)
		if err != nil {
			d.logger.Error("Failed to marshal prefix config",
				zap.String("prefix", p.Prefix),
				zap.Error(err))
			stats.Failed++
			continue
		}

		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})
	}

	failedItems, err := d.dynamodbKit.BatchWrite(ctx, d.tableName, requests)

	stats.Processed = len(requests) - len(failedItems)
	stats.Failed += len(failedItems)

	d.logger.Info("Upsert completed",
		zap.Int("processed", stats.Processed),
		zap.Int("failed", stats.Failed),
		zap.Int("skipped", stats.Skipped),
		zap.Duration("elapsed", time.Since(start)))

	if len(failedItems) > 0 || err != nil {
		return stats, fmt.Errorf("write operation failed: %w", err)
	}

	return stats, nil
}

// batchGet handles retry and throttling logic for DynamoDB BatchGetItem operations.
// It processes unprocessed keys across multiple iterations until all items are retrieved
// or the Guard exhausts retries.
//func (d *dynamoPrefixConfigRepository) batchGet(ctx context.Context, keys []map[string]types.AttributeValue) ([]map[string]types.AttributeValue, error) {
//
//	var results []map[string]types.AttributeValue
//	var unprocessed = keys
//
//	// Outer loop: responsible for advancing work until nothing is pending
//	for len(unprocessed) > 0 {
//		// Capture current batch for this attempt
//		// If Guard retries internally, it will use this same currentBatch
//		currentBatch := unprocessed
//
//		err := d.guard.Execute(ctx, func() error {
//			input := &dynamodb.BatchGetItemInput{
//				RequestItems: map[string]types.KeysAndAttributes{
//					d.tableName: {
//						Keys:           currentBatch,
//						ConsistentRead: aws.Bool(true),
//					},
//				},
//			}
//
//			out, err := d.client.BatchGetItem(ctx, input)
//			if err != nil {
//				return err // Guard will decide if retryable (network error, 5xx, etc.)
//			}
//
//			// Accumulate successful responses
//			if list, ok := out.Responses[d.tableName]; ok {
//				results = append(results, list...)
//			}
//
//			// Check for pending items for next outer loop iteration
//			// Note: We don't return error here. Partial success from DynamoDB is valid.
//			// Update external 'unprocessed' variable so the for loop continues with remaining items.
//			if len(out.UnprocessedKeys) > 0 {
//				unprocessed = out.UnprocessedKeys[d.tableName].Keys
//			} else {
//				unprocessed = nil
//			}
//
//			return nil
//		})
//
//		if err != nil {
//			// If Guard fails (exhausted retries or circuit breaker open), fail entire operation
//			return nil, fmt.Errorf("batch get failed after retries: %w", err)
//		}
//	}
//
//	return results, nil
//}
