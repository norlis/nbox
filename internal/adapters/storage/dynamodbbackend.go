package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"nbox/internal/usecases"
)

const (
	DynamoDBLockPrefix = "_"
	BatchSize          = 25
)

// Internal structures for DynamoDB mapping.
type RecordBase struct {
	Key      string          `dynamodbav:"Key"`
	Value    []byte          `dynamodbav:"Value"`
	Metadata models.Metadata `dynamodbav:"Metadata"`
}

type Record struct {
	Path string `dynamodbav:"Path"`
	*RecordBase
}

// DynamoDBBackend implements EntryFullStore (Upsert, Retrieve, List, Delete)
// Note: Tracking is no longer the direct responsibility of this struct.
type DynamoDBBackend struct {
	client      *dynamodb.Client
	config      *application.Config
	pathUseCase *usecases.PathUseCase
	logger      *zap.Logger
	dynamodbKit *DynamodbKit
}

func NewDynamoDBBackend(
	client *dynamodb.Client,
	config *application.Config,
	pathUseCase *usecases.PathUseCase,
	logger *zap.Logger,
	dynamodbKit *DynamodbKit,
) *DynamoDBBackend {
	return &DynamoDBBackend{
		client:      client,
		config:      config,
		pathUseCase: pathUseCase,
		logger:      logger,
		dynamodbKit: dynamodbKit,
	}
}

func (d *DynamoDBBackend) BackendType() backend.StorageBackendType {
	return backend.BackendDynamoDB
}

func (d *DynamoDBBackend) RetrieveMany(ctx context.Context, keys []string) (map[string]*models.Entry, error) {
	if len(keys) == 0 {
		return make(map[string]*models.Entry), nil
	}

	attributes := make([]map[string]types.AttributeValue, 0, len(keys))

	for _, fullKey := range keys {
		// Use pathUseCase to split "folder/variable" into "folder" and "variable"
		path, err := attributevalue.Marshal(d.pathUseCase.PathWithoutKey(fullKey))
		if err != nil {
			d.logger.Error("Failed to marshal path", zap.String("key", fullKey), zap.Error(err))
			continue
		}

		key, err := attributevalue.Marshal(d.pathUseCase.BaseKey(fullKey))
		if err != nil {
			d.logger.Error("Failed to marshal key", zap.String("key", fullKey), zap.Error(err))
			continue
		}

		attributes = append(attributes, map[string]types.AttributeValue{
			"Path": path,
			"Key":  key,
		})
	}
	items, err := d.dynamodbKit.BatchGet(ctx, d.config.EntryTableName, attributes)
	if err != nil {
		// If the entire batch fails (after retries), return error.
		// The Gateway will decide whether to degrade the service or fail.
		return nil, fmt.Errorf("retrieve many failed: %w", err)
	}

	results := make(map[string]*models.Entry)

	for _, item := range items {
		var record Record
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			d.logger.Error("Failed to unmarshal record", zap.Any("item", item), zap.Error(err))
			continue
		}

		fullKey := d.pathUseCase.Concat(record.Path, record.Key)

		results[fullKey] = &models.Entry{
			Key:      fullKey,
			Value:    string(record.Value),
			Secure:   record.Metadata.Secure,
			Metadata: &record.Metadata,
		}
	}

	return results, nil
}

// Upsert inserts or updates entries in DynamoDB.
// It only cares about the CURRENT state. Tracking is handled externally.
func (d *DynamoDBBackend) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	results := make(operations.Results, len(entries))

	uniqueRequests := make(map[string]types.WriteRequest)

	// requests := make([]types.WriteRequest, 0, len(entries))

	updatedBy := "ghost"
	if user, ok := application.UserFromContext(ctx); ok {
		updatedBy = user.Name
	}
	now := time.Now().UTC()

	for _, entry := range entries {
		sanitizedKey := d.sanitize(entry.Key)

		path := d.pathUseCase.PathWithoutKey(sanitizedKey)
		key := d.pathUseCase.BaseKey(sanitizedKey)

		storageType := backend.BackendDynamoDB
		if entry.Metadata != nil && entry.Metadata.StorageBackend != "" {
			storageType = entry.Metadata.StorageBackend
		}

		record := Record{
			Path: path,
			RecordBase: &RecordBase{
				Key:   key,
				Value: []byte(entry.Value),
				Metadata: models.Metadata{
					UpdatedAt:      now,
					UpdatedBy:      updatedBy,
					Secure:         entry.Secure,
					Action:         "upsert",
					StorageBackend: storageType,
					Fingerprint:    entry.Metadata.Fingerprint,
				},
			},
		}

		// Individual marshal with immediate error handling
		item, err := attributevalue.MarshalMap(record)
		if err != nil {
			// Mark THIS specific entry as failed and don't add it to the batch
			d.logger.Error("DynamoDB Marshal failed", zap.String("key", sanitizedKey), zap.Error(err))
			results.Add(sanitizedKey, operations.Failed, fmt.Errorf("marshal error: %w", err))
			continue
		}

		uniqueRequests[fmt.Sprintf("%s/%s", path, key)] = types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		}

		// Assume success by default (Pending)
		// results.Add(sanitizedKey, operations.Updated, nil)
		results.AddWithOutput(sanitizedKey, operations.Updated, nil, &entry)

		for _, prefix := range d.pathUseCase.Prefixes(sanitizedKey) {
			parentPath := d.pathUseCase.PathWithoutKey(prefix)
			parentKey := d.pathUseCase.BaseKey(prefix) + "/"
			parentRecordKey := fmt.Sprintf("%s%s", parentPath, parentKey)

			if _, exists := uniqueRequests[parentRecordKey]; !exists {
				parentRecord := Record{
					Path: parentPath,
					RecordBase: &RecordBase{
						Key: parentKey,
						Metadata: models.Metadata{
							UpdatedAt: now,
							UpdatedBy: updatedBy,
						},
					},
				}
				pItem, parentErr := attributevalue.MarshalMap(parentRecord)
				if parentErr != nil {
					continue
				}
				uniqueRequests[parentRecordKey] = types.WriteRequest{
					PutRequest: &types.PutRequest{Item: pItem},
				}
			}
		}
	}

	requests := make([]types.WriteRequest, 0, len(uniqueRequests))
	for _, req := range uniqueRequests {
		requests = append(requests, req)
	}

	if len(requests) == 0 {
		return results
	}

	failedItems, err := d.dynamodbKit.BatchWrite(ctx, d.config.EntryTableName, requests)

	if len(failedItems) > 0 {
		// Only mark as failed those returned by dynamodbKit.BatchWrite
		// The rest keep the "Updated" status we set initially (optimistic success)
		commonErr := err
		if commonErr == nil {
			commonErr = errors.New("write operation failed after retries")
		}

		for _, req := range failedItems {
			key := d.extractKeyFromRequest(req)
			results.Add(key, operations.Failed, commonErr)
		}
	}

	return results
}

func (d *DynamoDBBackend) Resolve(ctx context.Context, key string) (*models.Entry, error) {
	entry, err := d.Retrieve(ctx, key)
	if err != nil || entry == nil {
		return nil, err
	}
	return entry, nil
}

func (d *DynamoDBBackend) Retrieve(ctx context.Context, key string, _ ...domain.RetrieveOption) (*models.Entry, error) {
	p, _ := attributevalue.Marshal(d.pathUseCase.PathWithoutKey(key))
	k, _ := attributevalue.Marshal(d.pathUseCase.BaseKey(key))

	resp, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		Key:            map[string]types.AttributeValue{"Path": p, "Key": k},
		TableName:      aws.String(d.config.EntryTableName),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}
	if resp.Item == nil {
		return nil, nil // Not found
	}

	record := &Record{}
	if err := attributevalue.UnmarshalMap(resp.Item, record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &models.Entry{
		Key:      d.pathUseCase.Concat(record.Path, record.Key),
		Value:    string(record.Value),
		Secure:   record.Metadata.Secure,
		Metadata: &record.Metadata,
	}, nil
}

func (d *DynamoDBBackend) List(ctx context.Context, prefix string) ([]models.Entry, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	cleanPrefix := d.pathUseCase.EscapeEmptyPath(prefix)

	keyEx := expression.Key("Path").Equal(expression.Value(cleanPrefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, fmt.Errorf("expression builder: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 aws.String(d.config.EntryTableName),
		ConsistentRead:            aws.Bool(true),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})

	var entries []models.Entry
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("dynamo query: %w", err)
		}

		var records []Record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			return nil, fmt.Errorf("unmarshal list: %w", err)
		}

		for _, r := range records {
			if !strings.HasPrefix(r.Key, DynamoDBLockPrefix) {
				entries = append(entries, models.Entry{
					Key:    d.pathUseCase.Concat(r.Path, r.Key),
					Value:  string(r.Value),
					Path:   r.Path,
					Secure: r.Metadata.Secure,
				})
			}
		}
	}
	return entries, nil
}

func (d *DynamoDBBackend) Delete(ctx context.Context, key string) error {
	// Delete the exact item (if it exists)
	// It's cheap to try deleting it directly
	p := d.pathUseCase.PathWithoutKey(key)
	k := d.pathUseCase.BaseKey(key)

	rootDelete := types.WriteRequest{
		DeleteRequest: &types.DeleteRequest{
			Key: map[string]types.AttributeValue{
				"Path": mustMarshal(p),
				"Key":  mustMarshal(k),
			},
		},
	}

	// Try to delete the root first
	if _, err := d.dynamodbKit.BatchWrite(ctx, d.config.EntryTableName, []types.WriteRequest{rootDelete}); err != nil {
		return fmt.Errorf("failed to delete root item: %w", err)
	}

	// Early return check: Verificar si hay hijos antes de paginar
	prefix := d.pathUseCase.EscapeEmptyPath(strings.TrimSuffix(key, "/"))

	keyEx := expression.Key("Path").Equal(expression.Value(prefix))
	expr, _ := expression.NewBuilder().WithKeyCondition(keyEx).Build()

	// Quick peek: Query con Limit=1 para verificar existencia de hijos
	peekResult, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.config.EntryTableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		Limit:                     aws.Int32(1),
		Select:                    types.SelectCount,
	})
	if err != nil {
		// If the check fails, continue with the normal process (not fatal)
		d.logger.Warn("Failed to check for children, proceeding with full deletion", zap.Error(err))
	} else if peekResult.Count == 0 {
		// No hay hijos, retorno temprano
		return nil
	}

	// Delete children (Efficient pagination with ProjectionExpression)
	proj := expression.NamesList(expression.Name("Path"), expression.Name("Key"))
	expr, _ = expression.NewBuilder().
		WithKeyCondition(keyEx).
		WithProjection(proj).
		Build() // Error ignorado seguro por estructura fija

	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 aws.String(d.config.EntryTableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list children for deletion: %w", err)
		}

		// Convert current page to Batch Delete Requests
		batchReqs := make([]types.WriteRequest, 0, len(page.Items))

		var records []Record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			return fmt.Errorf("unmarshal error during delete: %w", err)
		}

		for _, r := range records {
			// Reconstruct keys for DeleteRequest
			pAv, _ := attributevalue.Marshal(r.Path)
			kAv, _ := attributevalue.Marshal(r.Key)

			batchReqs = append(batchReqs, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{"Path": pAv, "Key": kAv},
				},
			})
		}

		// Execute deletion of THIS page before requesting the next one
		if len(batchReqs) > 0 {
			failed, err := d.dynamodbKit.BatchWrite(ctx, d.config.EntryTableName, batchReqs)
			if err != nil {
				return fmt.Errorf("failed to delete batch of children (failed: %d): %w", len(failed), err)
			}
		}
	}

	return nil
}

func mustMarshal(v string) types.AttributeValue {
	av, _ := attributevalue.Marshal(v)
	return av
}

// Private helpers

func (d *DynamoDBBackend) sanitize(key string) string {
	key = strings.ToLower(key)
	key = strings.TrimSpace(key)
	key = strings.Trim(key, "/")
	return key
}

// extractKeyFromRequest reconstruye la key original (app/foo) desde un WriteRequest de Dynamo.
func (d *DynamoDBBackend) extractKeyFromRequest(req types.WriteRequest) string {
	var item map[string]types.AttributeValue

	if req.PutRequest != nil {
		item = req.PutRequest.Item
	} else if req.DeleteRequest != nil {
		item = req.DeleteRequest.Key
	}

	if item == nil {
		return "unknown"
	}

	var r Record
	// Unmarshal only key fields for efficiency
	if err := attributevalue.UnmarshalMap(item, &r); err != nil {
		d.logger.Warn("Failed to extract key from failed request",
			zap.Error(err),
			zap.Any("request_type", map[bool]string{true: "PutRequest", false: "DeleteRequest"}[req.PutRequest != nil]))
		return "unknown"
	}

	// Reconstruct using pathUseCase (Path + BaseKey)
	return d.pathUseCase.Concat(r.Path, r.Key)
}
