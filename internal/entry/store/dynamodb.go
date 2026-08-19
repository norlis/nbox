package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/norlis/httpgate/logging"
	"nbox/internal/application"
	"nbox/internal/entry"
	"nbox/internal/logfields"
	"nbox/internal/nbox"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/prefix"
)

const (
	DynamoDBLockPrefix = "_"
	BatchSize          = 25
)

// dynamodbClientAPI is the minimal interface over *dynamodb.Client that DynamoDB uses
// directly. It satisfies dynamodb.QueryAPIClient and dynamodb.ScanAPIClient so the
// SDK paginators can accept it.
type dynamodbClientAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

// Compile-time assertion: *dynamodb.Client satisfies dynamodbClientAPI.
var _ dynamodbClientAPI = (*dynamodb.Client)(nil)

// dynamodbKitAPI is the minimal interface over *platformaws.DynamoDBKit that
// DynamoDB uses for batch reads and writes.
type dynamodbKitAPI interface {
	BatchWrite(ctx context.Context, tableName string, requests []types.WriteRequest) ([]types.WriteRequest, error)
	BatchGet(ctx context.Context, tableName string, keys []map[string]types.AttributeValue) ([]map[string]types.AttributeValue, error)
}

// Compile-time assertion: *platformaws.DynamoDBKit satisfies dynamodbKitAPI.
var _ dynamodbKitAPI = (*platformaws.DynamoDBKit)(nil)

// recordBase es la estructura interna para mapear filas DynamoDB.
type recordBase struct {
	Key      string         `dynamodbav:"Key"`
	Value    []byte         `dynamodbav:"Value"`
	Metadata entry.Metadata `dynamodbav:"Metadata"`
}

type record struct {
	Path string `dynamodbav:"Path"`
	recordBase
}

// DynamoDB implementa entry.Store (Upsert, Retrieve, List, Delete) usando DynamoDB.
type DynamoDB struct {
	client      dynamodbClientAPI
	config      *nbox.Config
	pathUseCase *entry.Processor
	logger      *slog.Logger
	dynamodbKit dynamodbKitAPI
}

func NewDynamoDB(
	client *dynamodb.Client,
	config *nbox.Config,
	pathUseCase *entry.Processor,
	logger *slog.Logger,
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

	seen := make(map[string]struct{}, len(keys))
	attributes := make([]map[string]types.AttributeValue, 0, len(keys))

	var marshalFailed int
	for _, fullKey := range keys {
		if _, dup := seen[fullKey]; dup {
			continue
		}
		seen[fullKey] = struct{}{}

		path, err := attributevalue.Marshal(d.pathUseCase.PathWithoutKey(fullKey))
		if err != nil {
			marshalFailed++
			d.logger.DebugContext(ctx, "marshal path failed", slog.String(logfields.KeyNboxKey, fullKey), logging.Err(err))
			continue
		}

		key, err := attributevalue.Marshal(d.pathUseCase.BaseKey(fullKey))
		if err != nil {
			marshalFailed++
			d.logger.DebugContext(ctx, "marshal key failed", slog.String(logfields.KeyNboxKey, fullKey), logging.Err(err))
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

	var unmarshalFailed int
	for _, item := range items {
		var r record
		if err := attributevalue.UnmarshalMap(item, &r); err != nil {
			unmarshalFailed++
			d.logger.DebugContext(ctx, "unmarshal record failed", logging.Err(err))
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

	if failed := marshalFailed + unmarshalFailed; failed > 0 {
		d.logger.InfoContext(ctx, "retrieve many completed with failures",
			slog.Int(logfields.KeyEntriesTotal, len(keys)),
			slog.Int(logfields.KeyEntriesFailed, failed))
	}

	return results, nil
}

func (d *DynamoDB) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	results := make(entry.Results, len(entries))

	uniqueRequests := make(map[string]types.WriteRequest)

	updatedBy := application.ActorFromContext(ctx)
	now := time.Now().UTC()

	var marshalFailed int
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
			recordBase: recordBase{
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
			marshalFailed++
			d.logger.DebugContext(ctx, "marshal failed", slog.String(logfields.KeyNboxKey, sanitizedKey), logging.Err(err))
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
					recordBase: recordBase{
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

	if marshalFailed > 0 {
		d.logger.InfoContext(ctx, "upsert completed with failures",
			slog.Int(logfields.KeyEntriesTotal, len(entries)),
			slog.Int(logfields.KeyEntriesFailed, marshalFailed))
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
		TableName:      new(d.config.EntryTableName),
		ConsistentRead: new(true),
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

func (d *DynamoDB) listLevel(ctx context.Context, pfx string) ([]entry.Entry, error) {
	pfx = strings.TrimSuffix(pfx, "/")
	cleanPrefix := d.pathUseCase.EscapeEmptyPath(pfx)

	keyEx := expression.Key("Path").Equal(expression.Value(cleanPrefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, fmt.Errorf("expression builder: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 new(d.config.EntryTableName),
		ConsistentRead:            new(true),
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

		if cap(entries)-len(entries) < len(records) {
			grown := make([]entry.Entry, len(entries), len(entries)+len(records))
			copy(grown, entries)
			entries = grown
		}

		for i := range records {
			r := &records[i]
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

// List returns entries under pfx. Default: a single level, folders included
// (what the UI uses to navigate). With entry.LeavesOnly(): every real key
// under pfx, flat — folder markers and empty-value records are filtered out.
// At root, LeavesOnly falls back to the level listing: a flat dump of the
// whole table is never served from "/".
func (d *DynamoDB) List(ctx context.Context, pfx string, opts ...entry.ListOption) ([]entry.Entry, error) {
	if entry.NewListOptions(opts...).LeavesOnly && strings.Trim(strings.TrimSpace(pfx), "/") != "" {
		return d.listLeaves(ctx, pfx)
	}
	return d.listLevel(ctx, pfx)
}

// listLeaves returns every real key under pfx (flat subtree) via ONE paginated,
// eventually consistent Scan filtered in memory: folder markers, empty-value
// records and lock entries are dropped, with an exact segment boundary on pfx.
// Each row stores its full directory in Path, so no tree walk is needed — the
// old per-folder BFS issued O(folders) sequential Queries and took seconds.
func (d *DynamoDB) listLeaves(ctx context.Context, pfx string) ([]entry.Entry, error) {
	pfx = strings.Trim(strings.TrimSpace(pfx), "/")

	paginator := dynamodb.NewScanPaginator(d.client, &dynamodb.ScanInput{
		TableName: new(d.config.EntryTableName),
	})

	leaves := make([]entry.Entry, 0)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("dynamo scan: %w", err)
		}

		var records []record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
			return nil, fmt.Errorf("unmarshal list: %w", err)
		}

		for i := range records {
			r := &records[i]
			if strings.HasSuffix(r.Key, "/") || strings.HasPrefix(r.Key, DynamoDBLockPrefix) {
				continue // folder marker / lock entry
			}
			if strings.TrimSpace(string(r.Value)) == "" {
				continue // empty value (the feature: real keys only)
			}
			if pfx != "" && r.Path != pfx && !strings.HasPrefix(r.Path, pfx+"/") {
				continue // outside the requested subtree (exact segment boundary)
			}
			leaves = append(leaves, entry.Entry{
				Key:      d.pathUseCase.Concat(r.Path, r.Key),
				ShortKey: r.Key,
				Value:    string(r.Value),
				Path:     r.Path,
				Secure:   r.Metadata.Secure,
				Metadata: &r.Metadata,
			})
		}
	}
	return leaves, nil
}

// Delete removes the target item and its entire subtree from DynamoDB.
//
// Because Path is the partition key, a begins_with query is not possible.
// Instead we perform a BFS walk: for each directory level queried, items whose
// Key ends in "/" are child directories — we enqueue their full path
// (Concat(item.Path, item.Key)) and continue until the queue is empty.
// All collected (Path, Key) pairs are batch-deleted along with the root item.
func (d *DynamoDB) Delete(ctx context.Context, key string) error {
	p := d.pathUseCase.PathWithoutKey(key)
	k := d.pathUseCase.BaseKey(key)

	// Always delete the root item first.
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

	// BFS queue of Path values to scan for children.
	// Start from the root's path level, which is the key itself stripped of trailing slash.
	queue := []string{d.pathUseCase.EscapeEmptyPath(strings.TrimSuffix(key, "/"))}

	proj := expression.NamesList(expression.Name("Path"), expression.Name("Key"))

	for len(queue) > 0 {
		// Dequeue.
		currentPath := queue[0]
		queue = queue[1:]

		keyEx := expression.Key("Path").Equal(expression.Value(currentPath))
		expr, err := expression.NewBuilder().
			WithKeyCondition(keyEx).
			WithProjection(proj).
			Build()
		if err != nil {
			return fmt.Errorf("expression builder for path %q: %w", currentPath, err)
		}

		paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
			TableName:                 new(d.config.EntryTableName),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
			ProjectionExpression:      expr.Projection(),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("failed to list children for path %q: %w", currentPath, err)
			}
			if len(page.Items) == 0 {
				continue
			}

			var records []record
			if err := attributevalue.UnmarshalListOfMaps(page.Items, &records); err != nil {
				return fmt.Errorf("unmarshal error during delete for path %q: %w", currentPath, err)
			}

			batchReqs := make([]types.WriteRequest, 0, len(records))
			for _, r := range records {
				pAv, _ := attributevalue.Marshal(r.Path)
				kAv, _ := attributevalue.Marshal(r.Key)
				batchReqs = append(batchReqs, types.WriteRequest{
					DeleteRequest: &types.DeleteRequest{
						Key: map[string]types.AttributeValue{"Path": pAv, "Key": kAv},
					},
				})

				// If the Key ends in "/" this item is a directory marker; enqueue its
				// subtree path so we descend into grandchildren.
				if strings.HasSuffix(r.Key, "/") {
					childPath := d.pathUseCase.EscapeEmptyPath(
						strings.TrimSuffix(d.pathUseCase.Concat(r.Path, r.Key), "/"),
					)
					queue = append(queue, childPath)
				}
			}

			if len(batchReqs) > 0 {
				failed, err := d.dynamodbKit.BatchWrite(ctx, d.config.EntryTableName, batchReqs)
				if err != nil {
					return fmt.Errorf("failed to delete batch for path %q (failed: %d): %w", currentPath, len(failed), err)
				}
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
		requestType := "DeleteRequest"
		if req.PutRequest != nil {
			requestType = "PutRequest"
		}
		d.logger.Warn("failed to extract key from failed request",
			logging.Err(err),
			slog.String("request_type", requestType))
		return "unknown"
	}

	return d.pathUseCase.Concat(r.Path, r.Key)
}
