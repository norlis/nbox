package storage

import (
	"context"
	"errors"
	"fmt"
	_ "nbox/internal/domain/models"
	"nbox/pkg/resiliency"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
)

const (
	DynamodbBatchWriteSize = 25
	BatchGetSize           = 100
)

type DynamodbKit struct {
	client *dynamodb.Client
	logger *zap.Logger
	guard  *resiliency.Guard
}

func NewDynamodbKit(client *dynamodb.Client, logger *zap.Logger) *DynamodbKit {
	guard := resiliency.NewGuard(resiliency.GuardConfig{
		MaxConcurrency:           50, // High concurrency allowed for reads
		MaxRetries:               3,
		BaseDelay:                50 * time.Millisecond,
		Name:                     "dynamodb-kit",
		CBMaxConsecutiveFailures: 10,
	})
	return &DynamodbKit{client: client, logger: logger, guard: guard}
}

func (k *DynamodbKit) BatchWrite(ctx context.Context, tableName string, requests []types.WriteRequest) ([]types.WriteRequest, error) {
	var totalFailed []types.WriteRequest

	for i := 0; i < len(requests); i += DynamodbBatchWriteSize {
		end := i + DynamodbBatchWriteSize
		if end > len(requests) {
			end = len(requests)
		}

		chunk := requests[i:end]

		// Backoff configuration for this specific chunk
		b := backoff.NewExponentialBackOff()
		b.MaxElapsedTime = 15 * time.Second // Max time retrying this chunk
		startTime := time.Now()

		// Track unprocessed items in this chunk
		var unprocessed = chunk

		operation := func() error {
			if len(unprocessed) == 0 {
				return nil
			}

			input := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: unprocessed,
				},
			}

			out, err := k.client.BatchWriteItem(ctx, input)
			if err != nil {
				return err // Network/AWS error, Backoff will retry
			}

			// If there are unprocessed items (Throttling), update list and force error to retry
			if len(out.UnprocessedItems) > 0 {
				unprocessed = out.UnprocessedItems[tableName]
				return errors.New("dynamodb throttling: partial batch processed")
			}

			// Full chunk success
			unprocessed = nil
			return nil
		}

		// Execute retry
		err := backoff.Retry(operation, backoff.WithContext(b, ctx))

		// If items remain after all retries, add them to the final failure list
		if err != nil || len(unprocessed) > 0 {
			k.logger.Warn("BatchWriteItem partial failure after retries",
				zap.String("table", tableName),
				zap.Int("original_count", len(chunk)),
				zap.Int("failed_count", len(unprocessed)),
				zap.Duration("elapsed", time.Since(startTime)),
				zap.Error(err))
			totalFailed = append(totalFailed, unprocessed...)
		}
	}

	if len(totalFailed) > 0 {
		return totalFailed, fmt.Errorf("failed to process %d items", len(totalFailed))
	}

	return nil, nil
}

// BatchGet uses Guard to handle both network errors and UnprocessedKeys (partial batch)
func (k *DynamodbKit) BatchGet(
	ctx context.Context,
	tableName string,
	keys []map[string]types.AttributeValue,
) ([]map[string]types.AttributeValue, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	results := make([]map[string]types.AttributeValue, 0, len(keys))

	// Chunking: DynamoDB hard limit (100 items)
	for i := 0; i < len(keys); i += BatchGetSize {
		end := i + BatchGetSize
		if end > len(keys) {
			end = len(keys)
		}

		chunk := keys[i:end]
		chunkResults, err := k.processGetChunk(ctx, tableName, chunk)
		if err != nil {
			return nil, err
		}
		results = append(results, chunkResults...)
	}

	return results, nil
}

func (k *DynamodbKit) processGetChunk(
	ctx context.Context,
	tableName string,
	keys []map[string]types.AttributeValue,
) ([]map[string]types.AttributeValue, error) {

	// Pre-allocate with expected capacity
	chunkResults := make([]map[string]types.AttributeValue, 0, len(keys))

	// Mutable state for Guard closure
	currentKeys := keys

	// Guard handles both network errors and partial batch retries
	err := k.guard.Execute(ctx, func() error {
		if len(currentKeys) == 0 {
			return nil
		}

		input := &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				tableName: {
					Keys:           currentKeys,
					ConsistentRead: aws.Bool(true),
				},
			},
		}

		out, err := k.client.BatchGetItem(ctx, input)
		if err != nil {
			return err
		}

		if items, found := out.Responses[tableName]; found {
			chunkResults = append(chunkResults, items...)
		}

		// Partial batch - Guard will retry with backoff
		if len(out.UnprocessedKeys) > 0 {
			currentKeys = out.UnprocessedKeys[tableName].Keys
			return fmt.Errorf("partial batch: %d keys remaining", len(currentKeys))
		}

		currentKeys = nil
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("batch get failed: %w", err)
	}

	return chunkResults, nil
}
