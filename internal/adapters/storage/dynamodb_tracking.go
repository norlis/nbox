package storage

import (
	"context"
	"fmt"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

type DynamoDBTracking struct {
	client      *dynamodb.Client
	tableName   string
	dynamodbKit *DynamodbKit
	logger      *zap.Logger
}

func NewDynamoDBTracking(
	client *dynamodb.Client,
	config *application.Config,
	dynamodbKit *DynamodbKit,
	logger *zap.Logger,
) domain.TrackingRepository {
	return &DynamoDBTracking{
		client:      client,
		tableName:   config.TrackingEntryTableName,
		dynamodbKit: dynamodbKit,
		logger:      logger,
	}
}

func (d *DynamoDBTracking) CreateBatch(ctx context.Context, tracks []models.Tracking) error {
	if len(tracks) == 0 {
		return nil
	}

	requests := make([]types.WriteRequest, 0, len(tracks))

	for _, track := range tracks {
		// Direct mapping: Struct -> DynamoDB Item
		// The 'dynamodbav:"Timestamp,unixtime"' tag automatically converts time.Time to a number.
		item, err := attributevalue.MarshalMap(track)
		if err != nil {
			d.logger.Error("Failed to marshal tracking", zap.String("key", track.Key), zap.Error(err))
			continue
		}

		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})
	}

	if len(requests) == 0 {
		return nil
	}

	failed, err := d.dynamodbKit.BatchWrite(ctx, d.tableName, requests)
	if err != nil {
		return fmt.Errorf("tracking batch error: %w", err)
	}
	if len(failed) > 0 {
		d.logger.Warn("Partial tracking failure", zap.Int("failed_count", len(failed)))
	}

	return nil
}

func (d *DynamoDBTracking) History(ctx context.Context, key string, opts ...domain.HistoryOption) ([]models.Tracking, error) {
	config := &domain.HistoryConfig{
		Since: time.Now().Add(-24 * time.Hour), // Default: Last 24 hours
		To:    time.Now().UTC(),                // Default: Until now
		Limit: 100,                             // Default: Maximum 100 records
	}

	for _, opt := range opts {
		opt(config)
	}

	keyCond := expression.Key("Key").Equal(expression.Value(key))

	if !config.Since.IsZero() {
		fromStr := config.Since.Format(time.RFC3339)
		toStr := config.To.Format(time.RFC3339)

		// Range: Timestamp BETWEEN :from AND :to
		keyCond = keyCond.And(expression.Key("Timestamp").Between(
			expression.Value(fromStr),
			expression.Value(toStr),
		))
	}

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, err
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		ConsistentRead:            aws.Bool(true), // Recommended to see the latest write immediately
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(false),         // Descending order (most recent first)
		Limit:                     aws.Int32(config.Limit), // Apply limit
	}

	// Pagination
	paginator := dynamodb.NewQueryPaginator(d.client, queryInput)
	var history []models.Tracking

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("History query failed", zap.String("key", key), zap.Error(err))
			return nil, err
		}

		// Pagination fix:
		// Deserialize into a temporary slice and append to the main one.
		// This prevents overwriting or subtle pointer errors.
		var pageItems []models.Tracking
		if err = attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return nil, fmt.Errorf("unmarshal history failed: %w", err)
		}

		history = append(history, pageItems...)

		if config.Limit > 0 && int32(len(history)) >= config.Limit {
			history = history[:config.Limit]
			break
		}
	}

	return history, nil
}
