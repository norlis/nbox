package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/prefix"
)

// DynamoDB es un Store de configuración de prefijos respaldado por DynamoDB
// con resilience patterns (retry + circuit breaker) para lecturas.
type DynamoDB struct {
	client      *dynamodb.Client
	config      *application.Config
	logger      *zap.Logger
	dynamodbKit *platformaws.DynamoDBKit
	pathUseCase *entry.Processor
	tableName   string
}

// NewDynamoDB crea un Store DynamoDB para configuraciones de prefijo.
func NewDynamoDB(
	client *dynamodb.Client,
	logger *zap.Logger,
	dynamodbKit *platformaws.DynamoDBKit,
	pathUseCase *entry.Processor,
	config *application.Config,
) prefix.Store {
	return &DynamoDB{
		client:      client,
		logger:      logger,
		dynamodbKit: dynamodbKit,
		pathUseCase: pathUseCase,
		tableName:   config.PrefixConfigTableName,
	}
}

// ByPrefix implementa Longest Prefix Match (LPM) jerárquico.
//
// Para input "development/myapp/db/password":
// 1. Genera candidates: ["development", "development/myapp", "development/myapp/db", "development/myapp/db/password"]
// 2. Hace fetch de todos los candidates en un solo BatchGetItem
// 3. Retorna el match más específico (prefijo más largo).
func (d *DynamoDB) ByPrefix(ctx context.Context, key string) (*prefix.Config, error) {
	cleanKey := strings.Trim(key, "/")
	if cleanKey == "" {
		return nil, errors.New("prefix cannot be empty")
	}

	candidates := d.pathUseCase.Prefixes(cleanKey)
	candidates = append(candidates, cleanKey)

	d.logger.Debug("Searching prefix config",
		zap.String("prefix", key),
		zap.String("cleanKey", cleanKey),
		zap.Strings("candidates", candidates))

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

	if len(keys) == 0 {
		d.logger.Error("All candidate marshals failed",
			zap.String("prefix", cleanKey),
			zap.Int("failed_count", failedMarshals))
		return nil, errors.New("failed to marshal any candidates")
	}

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
		return nil, entry.ErrEntryNotFound
	}

	// Longest Prefix Match (LPM) con desempate determinístico.
	var bestMatch *prefix.Config
	maxLen := -1

	for _, item := range items {
		var config prefix.Config
		if err := attributevalue.UnmarshalMap(item, &config); err != nil {
			d.logger.Error("Failed to unmarshal config",
				zap.Any("item", item),
				zap.Error(err))
			continue
		}

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
		return nil, entry.ErrEntryNotFound
	}

	d.logger.Debug("Found best match",
		zap.String("requested", cleanKey),
		zap.String("matched", bestMatch.Prefix),
		zap.String("backend", string(bestMatch.TypeDefault)),
		zap.Int("candidates_searched", len(candidates)),
		zap.Int("configs_found", len(items)))

	return bestMatch, nil
}

// List recupera todas las configuraciones de prefijo existentes.
func (d *DynamoDB) List(ctx context.Context) ([]prefix.Config, error) {
	start := time.Now()

	d.logger.Info("Scanning all prefix configs", zap.String("table", d.tableName))

	configs := make([]prefix.Config, 0)

	paginator := dynamodb.NewScanPaginator(d.client, &dynamodb.ScanInput{
		TableName: awssdk.String(d.tableName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("Scan page failed",
				zap.String("table", d.tableName),
				zap.Error(err))
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		var chunk []prefix.Config
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &chunk); err != nil {
			d.logger.Error("Unmarshal page failed",
				zap.String("table", d.tableName),
				zap.Int("item_count", len(page.Items)),
				zap.Error(err))
			return nil, fmt.Errorf("unmarshal failed: %w", err)
		}
		configs = append(configs, chunk...)
	}

	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Prefix < configs[j].Prefix
	})

	d.logger.Info("Scan completed",
		zap.Int("count", len(configs)),
		zap.Duration("elapsed", time.Since(start)))

	return configs, nil
}

// Upsert crea o actualiza configuraciones de prefijo en DynamoDB usando BatchWrite.
func (d *DynamoDB) Upsert(ctx context.Context, prefixes []prefix.Config) (prefix.UpsertStats, error) {
	if len(prefixes) == 0 {
		d.logger.Warn("Upsert called with empty prefixes list")
		return prefix.UpsertStats{}, nil
	}

	stats := prefix.UpsertStats{}
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
