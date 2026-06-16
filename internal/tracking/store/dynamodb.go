package store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
	"nbox/internal/nbox"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/tracking"
)

// DynamoDB es un tracking.Store respaldado por DynamoDB.
type DynamoDB struct {
	client      *dynamodb.Client
	tableName   string
	dynamodbKit *platformaws.DynamoDBKit
	logger      *zap.Logger
}

func NewDynamoDB(
	client *dynamodb.Client,
	config *nbox.Config,
	dynamodbKit *platformaws.DynamoDBKit,
	logger *zap.Logger,
) tracking.Store {
	return &DynamoDB{
		client:      client,
		tableName:   config.TrackingEntryTableName,
		dynamodbKit: dynamodbKit,
		logger:      logger,
	}
}

func (d *DynamoDB) CreateBatch(ctx context.Context, tracks []tracking.Record) error {
	if len(tracks) == 0 {
		return nil
	}

	requests := make([]types.WriteRequest, 0, len(tracks))

	for _, track := range tracks {
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

func (d *DynamoDB) History(ctx context.Context, key string, opts ...tracking.HistoryOption) ([]tracking.Record, error) {
	config := &tracking.HistoryConfig{
		Since: time.Now().Add(-24 * time.Hour),
		To:    time.Now().UTC(),
		Limit: 100,
	}

	for _, opt := range opts {
		opt(config)
	}

	keyCond := expression.Key("Key").Equal(expression.Value(key))

	if !config.Since.IsZero() {
		fromStr := config.Since.Format(time.RFC3339)
		toStr := config.To.Format(time.RFC3339)

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
		TableName:                 new(d.tableName),
		ConsistentRead:            new(true),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          new(false),
		Limit:                     new(config.Limit),
	}

	paginator := dynamodb.NewQueryPaginator(d.client, queryInput)
	var history []tracking.Record

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("History query failed", zap.String("key", key), zap.Error(err))
			return nil, err
		}

		var pageItems []tracking.Record
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
