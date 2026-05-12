package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/prefix"
)

const (
	DynamoDBLockPrefix = "_"
	BatchSize          = 25
)

// recordBase es la estructura interna para mapear filas DynamoDB.
type recordBase struct {
	Key      string         `dynamodbav:"Key"`
	Value    []byte         `dynamodbav:"Value"`
	Metadata entry.Metadata `dynamodbav:"Metadata"`
}

type record struct {
	Path string `dynamodbav:"Path"`
	*recordBase
}

// DynamoDB implementa entry.Store (Upsert, Retrieve, List, Delete) usando DynamoDB.
type DynamoDB struct {
	client      *dynamodb.Client
	config      *application.Config
	pathUseCase *entry.Processor
	logger      *zap.Logger
	dynamodbKit *platformaws.DynamoDBKit
}

func NewDynamoDB(
	client *dynamodb.Client,
	config *application.Config,
	pathUseCase *entry.Processor,
	logger *zap.Logger,
	dynamodbKit *platformaws.DynamoDBKit,
) *DynamoDB {
	return &DynamoDB{
		client:      client,
		config:      config,
		pathUseCase: pathUseCase,
		logger:      logger,
		dynamodbKit: dynamodbKit,
	}
}

func (d *DynamoDB) BackendType() prefix.StorageBackendType {
	return prefix.BackendDynamoDB
}

func (d *DynamoDB) RetrieveMany(ctx context.Context, keys []string) (map[string]*entry.Entry, error) {
	if len(keys) == 0 {
		return make(map[string]*entry.Entry), nil
	}

	attributes := make([]map[string]types.AttributeValue, 0, len(keys))

	for _, fullKey := range keys {
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
		return nil, fmt.Errorf("retrieve many failed: %w", err)
	}

	results := make(map[string]*entry.Entry)

	for _, item := range items {
		var r record
		if err := attributevalue.UnmarshalMap(item, &r); err != nil {
			d.logger.Error("Failed to unmarshal record", zap.Any("item", item), zap.Error(err))
			continue
		}

		fullKey := d.pathUseCase.Concat(r.Path, r.Key)

		results[fullKey] = &entry.Entry{
			Key:      fullKey,
			Value:    string(r.Value),
			Secure:   r.Metadata.Secure,
			Metadata: &r.Metadata,
		}
	}

	return results, nil
}

func (d *DynamoDB) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	results := make(entry.Results, len(entries))

	uniqueRequests := make(map[string]types.WriteRequest)

	updatedBy := "ghost"
	if user, ok := application.UserFromContext(ctx); ok {
		updatedBy = user.Name
	}
	now := time.Now().UTC()

	for _, en := range entries {
		sanitizedKey := d.sanitize(en.Key)

		path := d.pathUseCase.PathWithoutKey(sanitizedKey)
		key := d.pathUseCase.BaseKey(sanitizedKey)

		storageType := prefix.BackendDynamoDB
		if en.Metadata != nil && en.Metadata.StorageBackend != "" {
			storageType = en.Metadata.StorageBackend
		}

		rec := record{
			Path: path,
			recordBase: &recordBase{
				Key:   key,
				Value: []byte(en.Value),
				Metadata: entry.Metadata{
					UpdatedAt:      now,
					UpdatedBy:      updatedBy,
					Secure:         en.Secure,
					Action:         "upsert",
					StorageBackend: storageType,
					Fingerprint:    en.Metadata.Fingerprint,
					Version:        en.Metadata.Version,
				},
			},
		}

		item, err := attributevalue.MarshalMap(rec)
		if err != nil {
			d.logger.Error("DynamoDB Marshal failed", zap.String("key", sanitizedKey), zap.Error(err))
			results.Add(sanitizedKey, entry.Failed, fmt.Errorf("marshal error: %w", err))
			continue
		}

		uniqueRequests[fmt.Sprintf("%s/%s", path, key)] = types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		}

		results.AddWithOutput(sanitizedKey, entry.Updated, nil, &en)

		for _, p := range d.pathUseCase.Prefixes(sanitizedKey) {
			parentPath := d.pathUseCase.PathWithoutKey(p)
			parentKey := d.pathUseCase.BaseKey(p) + "/"
			parentRecordKey := fmt.Sprintf("%s%s", parentPath, parentKey)

			if _, exists := uniqueRequests[parentRecordKey]; !exists {
				parentRecord := record{
					Path: parentPath,
					recordBase: &recordBase{
						Key: parentKey,
						Metadata: entry.Metadata{
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
		commonErr := err
		if commonErr == nil {
			commonErr = errors.New("write operation failed after retries")
		}

		for _, req := range failedItems {
			key := d.extractKeyFromRequest(req)
			results.Add(key, entry.Failed, commonErr)
		}
	}

	return results
}

func (d *DynamoDB) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	e, err := d.Retrieve(ctx, key)
	if err != nil || e == nil {
		return nil, err
	}
	return e, nil
}

func (d *DynamoDB) Retrieve(ctx context.Context, key string, _ ...entry.RetrieveOption) (*entry.Entry, error) {
	p, _ := attributevalue.Marshal(d.pathUseCase.PathWithoutKey(key))
	k, _ := attributevalue.Marshal(d.pathUseCase.BaseKey(key))

	resp, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		Key:            map[string]types.AttributeValue{"Path": p, "Key": k},
		TableName:      awssdk.String(d.config.EntryTableName),
		ConsistentRead: awssdk.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}
	if resp.Item == nil {
		return nil, nil
	}

	r := &record{}
	if err := attributevalue.UnmarshalMap(resp.Item, r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &entry.Entry{
		Key:      d.pathUseCase.Concat(r.Path, r.Key),
		ShortKey: r.Key,
		Value:    string(r.Value),
		Secure:   r.Metadata.Secure,
		Metadata: &r.Metadata,
	}, nil
}

func (d *DynamoDB) List(ctx context.Context, prefix string) ([]entry.Entry, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	cleanPrefix := d.pathUseCase.EscapeEmptyPath(prefix)

	keyEx := expression.Key("Path").Equal(expression.Value(cleanPrefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, fmt.Errorf("expression builder: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 awssdk.String(d.config.EntryTableName),
		ConsistentRead:            awssdk.Bool(true),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})

	var entries []entry.Entry
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("dynamo query: %w", err)
		}

		var records []record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			return nil, fmt.Errorf("unmarshal list: %w", err)
		}

		for _, r := range records {
			if !strings.HasPrefix(r.Key, DynamoDBLockPrefix) {
				entries = append(entries, entry.Entry{
					Key:      d.pathUseCase.Concat(r.Path, r.Key),
					ShortKey: r.Key,
					Value:    string(r.Value),
					Path:     r.Path,
					Secure:   r.Metadata.Secure,
					Metadata: &r.Metadata,
				})
			}
		}
	}
	return entries, nil
}

func (d *DynamoDB) Delete(ctx context.Context, key string) error {
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

	if _, err := d.dynamodbKit.BatchWrite(ctx, d.config.EntryTableName, []types.WriteRequest{rootDelete}); err != nil {
		return fmt.Errorf("failed to delete root item: %w", err)
	}

	prefix := d.pathUseCase.EscapeEmptyPath(strings.TrimSuffix(key, "/"))

	keyEx := expression.Key("Path").Equal(expression.Value(prefix))
	expr, _ := expression.NewBuilder().WithKeyCondition(keyEx).Build()

	peekResult, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 awssdk.String(d.config.EntryTableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		Limit:                     awssdk.Int32(1),
		Select:                    types.SelectCount,
	})
	if err != nil {
		d.logger.Warn("Failed to check for children, proceeding with full deletion", zap.Error(err))
	} else if peekResult.Count == 0 {
		return nil
	}

	proj := expression.NamesList(expression.Name("Path"), expression.Name("Key"))
	expr, _ = expression.NewBuilder().
		WithKeyCondition(keyEx).
		WithProjection(proj).
		Build()

	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 awssdk.String(d.config.EntryTableName),
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

		batchReqs := make([]types.WriteRequest, 0, len(page.Items))

		var records []record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			return fmt.Errorf("unmarshal error during delete: %w", err)
		}

		for _, r := range records {
			pAv, _ := attributevalue.Marshal(r.Path)
			kAv, _ := attributevalue.Marshal(r.Key)

			batchReqs = append(batchReqs, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{"Path": pAv, "Key": kAv},
				},
			})
		}

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

func (d *DynamoDB) sanitize(key string) string {
	key = strings.ToLower(key)
	key = strings.TrimSpace(key)
	key = strings.Trim(key, "/")
	return key
}

// extractKeyFromRequest reconstruye la key original (app/foo) desde un WriteRequest de Dynamo.
func (d *DynamoDB) extractKeyFromRequest(req types.WriteRequest) string {
	var item map[string]types.AttributeValue

	if req.PutRequest != nil {
		item = req.PutRequest.Item
	} else if req.DeleteRequest != nil {
		item = req.DeleteRequest.Key
	}

	if item == nil {
		return "unknown"
	}

	var r record
	if err := attributevalue.UnmarshalMap(item, &r); err != nil {
		d.logger.Warn("Failed to extract key from failed request",
			zap.Error(err),
			zap.Any("request_type", map[bool]string{true: "PutRequest", false: "DeleteRequest"}[req.PutRequest != nil]))
		return "unknown"
	}

	return d.pathUseCase.Concat(r.Path, r.Key)
}
