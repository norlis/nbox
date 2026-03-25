package amazonaws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"nbox/internal/usecases"
)

const (
	DynamoDBLockPrefix        = "_"
	DefaultParallelOperations = 128
)

var ErrBackendTimeout = errors.New("dynamodb: timeout handling Unprocessed Items")

type BatchResult models.Exchange[map[string][]types.WriteRequest, error]

type PermitPool struct {
	sem chan int
}

type RecordBase struct {
	Key      string          `dynamodbav:"Key"`
	Value    []byte          `dynamodbav:"Value"`
	Metadata models.Metadata `dynamodbav:"Metadata"`
}

type Record struct {
	Path string `dynamodbav:"Path"`
	*RecordBase
}

type RecordTracking struct {
	Timestamp string `dynamodbav:"Timestamp"`
	*RecordBase
}

func NewPermitPool(permits int) *PermitPool {
	if permits < 1 {
		permits = DefaultParallelOperations
	}
	return &PermitPool{
		sem: make(chan int, permits),
	}
}

// Acquire returns when a permit has been acquired.
func (c *PermitPool) Acquire() {
	c.sem <- 1
}

// Release returns a permit to the pool.
func (c *PermitPool) Release() {
	<-c.sem
}

// CurrentPermits Get number of requests in the permit pool.
func (c *PermitPool) CurrentPermits() int {
	return len(c.sem)
}

// dynamodbBackend aws docs https://aws.github.io/aws-sdk-go-v2/docs/
// Deprecated.
type dynamodbBackend struct {
	client      *dynamodb.Client
	config      *application.Config
	permitPool  *PermitPool
	pathUseCase *usecases.PathUseCase
	logger      *zap.Logger
}

// NewDynamodbBackend TODO eliminar.
func NewDynamodbBackend(client *dynamodb.Client, config *application.Config, pathUseCase *usecases.PathUseCase, logger *zap.Logger) domain.EntryAdapter {
	return &dynamodbBackend{
		client:      client,
		config:      config,
		permitPool:  NewPermitPool(0),
		pathUseCase: pathUseCase,
		logger:      logger,
	}
}

func (d *dynamodbBackend) cleanedKey(key string) string {
	for _, prefix := range d.config.AllowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return key
		}
	}
	return fmt.Sprintf(
		"%s/%s", strings.Trim(d.config.DefaultPrefix, "/"), key,
	)
}

func (d *dynamodbBackend) sanitize(key string) string {
	key = strings.ToLower(key)
	key = strings.TrimSpace(key)
	key = strings.Trim(key, "/")

	key = d.cleanedKey(key)

	return key
}

// Upsert is used to insert or update an entry.
func (d *dynamodbBackend) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	records := map[string]Record{}
	tracking := map[string]RecordTracking{}
	results := make(operations.Results, len(entries))

	updatedBy := "ghost"
	action := "upsert"

	user, ok := application.UserFromContext(ctx)
	if ok {
		updatedBy = user.Name
	}

	for _, entry := range entries {
		now := time.Now().UTC()

		entryKey := d.sanitize(entry.Key)

		results[entryKey] = operations.Result{
			Key:    entryKey,
			Action: operations.Updated,
			Err:    nil,
		}

		path := d.pathUseCase.PathWithoutKey(entryKey)
		key := d.pathUseCase.BaseKey(entryKey)

		metadata := models.Metadata{
			UpdatedAt: now,
			UpdatedBy: updatedBy,
			Secure:    entry.Secure,
		}

		records[fmt.Sprintf("%s/%s", path, key)] = Record{
			Path: path,
			RecordBase: &RecordBase{
				Key:      key,
				Value:    []byte(entry.Value),
				Metadata: metadata,
			},
		}

		tracking[entryKey] = RecordTracking{
			Timestamp: strconv.FormatInt(now.Unix(), 10),
			RecordBase: &RecordBase{
				Key:   entryKey,
				Value: []byte(entry.Value),
				Metadata: models.Metadata{
					UpdatedAt: now,
					UpdatedBy: updatedBy,
					Secure:    entry.Secure,
					Action:    action,
				},
			},
		}

		for _, prefix := range d.pathUseCase.Prefixes(entryKey) {
			path = d.pathUseCase.PathWithoutKey(prefix)
			key = d.pathUseCase.BaseKey(prefix) + "/"
			records[fmt.Sprintf("%s%s", path, key)] = Record{
				Path: path,
				RecordBase: &RecordBase{
					Key: key,
					Metadata: models.Metadata{
						UpdatedAt: now,
						UpdatedBy: updatedBy,
					},
				},
			}
		}
	}

	ch := make(chan BatchResult)

	go func(channel chan BatchResult) {
		channel <- d.writeReqsBatch(ctx, d.config.EntryTableName, prepareWriteRequest(records))
		channel <- d.writeReqsBatch(ctx, d.config.TrackingEntryTableName, prepareWriteRequest(tracking))
	}(ch)

	result1 := <-ch
	result2 := <-ch

	if result2.Err != nil {
		d.logger.Error("ErrSaveTracking", zap.Error(result2.Err))
	}

	for _, req := range result1.Out[d.config.EntryTableName] {
		key := req.PutRequest.Item["Key"]
		path := req.PutRequest.Item["Path"]

		k := fmt.Sprintf("%s/%s", path, key)

		results[k] = operations.Result{
			Key:    k,
			Action: operations.Updated,
			Err:    result1.Err,
		}
	}

	return results
}

func (d *dynamodbBackend) writeReqsBatch(ctx context.Context, tableName string, requests []types.WriteRequest) BatchResult {
	for len(requests) > 0 {
		var err error
		var output *dynamodb.BatchWriteItemOutput
		unprocessed := map[string][]types.WriteRequest{}

		batchSize := int(math.Min(float64(len(requests)), 25))
		batch := map[string][]types.WriteRequest{tableName: requests[:batchSize]}
		requests = requests[batchSize:]

		d.permitPool.Acquire()
		boff := backoff.NewExponentialBackOff()
		boff.MaxElapsedTime = 600 * time.Second

		for len(batch) > 0 {
			output, err = d.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: batch,
			})
			if err != nil {
				break
			}

			if len(output.UnprocessedItems) == 0 {
				break
			}

			duration := boff.NextBackOff()
			unprocessed = output.UnprocessedItems
			if duration != backoff.Stop {
				batch = output.UnprocessedItems
				time.Sleep(duration)
			} else {
				err = ErrBackendTimeout
				break
			}
		}
		d.permitPool.Release()
		if err != nil {
			return BatchResult{Out: unprocessed, Err: err}
		}
	}
	return BatchResult{Out: nil, Err: nil}
}

// Retrieve Get is used to fetch an entry.
func (d *dynamodbBackend) Retrieve(ctx context.Context, key string) (*models.Entry, error) {
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
		//nolint:nilnil
		return nil, nil
	}
	record := &Record{}
	err = attributevalue.UnmarshalMap(resp.Item, record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal DynamoDB item: %w", err)
	}

	return &models.Entry{
		Key:    d.pathUseCase.Concat(record.Path, record.Key), // vaultKey(record),
		Value:  string(record.Value),
		Secure: record.Metadata.Secure,
	}, nil
}

// List is used to list all the keys under a given
// prefix, up to the next prefix.
func (d *dynamodbBackend) List(ctx context.Context, prefix string) ([]models.Entry, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	entries := make([]models.Entry, 0)
	prefix = d.pathUseCase.EscapeEmptyPath(prefix)

	keyEx := expression.Key("Path").Equal(expression.Value(prefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		d.logger.Error("ErrExpressionBuilder", zap.Error(err))
		return nil, fmt.Errorf("failed to build DynamoDB expression: %w", err)
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(d.config.EntryTableName),
		ConsistentRead:            aws.Bool(true),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	}
	queryPaginator := dynamodb.NewQueryPaginator(d.client, queryInput)
	var response *dynamodb.QueryOutput
	for queryPaginator.HasMorePages() {
		response, err = queryPaginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("ErrQueryPaginator", zap.Error(err), zap.String("prefix", prefix))
			return nil, fmt.Errorf("failed to get next page from DynamoDB: %w", err)
		}
		var records []Record
		err = attributevalue.UnmarshalListOfMaps(response.Items, &records)
		if err != nil {
			d.logger.Error("ErrUnmarshalListOfMaps", zap.Error(err), zap.String("prefix", prefix))
			return nil, fmt.Errorf("failed to unmarshal DynamoDB items: %w", err)
		}

		for _, record := range records {
			if !strings.HasPrefix(record.Key, DynamoDBLockPrefix) {
				entries = append(entries, models.Entry{
					Key:    record.Key,
					Value:  string(record.Value),
					Path:   record.Path,
					Secure: record.Metadata.Secure,
				})
			}
		}
	}

	return entries, nil
}

func (d *dynamodbBackend) Delete(ctx context.Context, key string) error {
	p, _ := attributevalue.Marshal(d.pathUseCase.PathWithoutKey(key))
	k, _ := attributevalue.Marshal(d.pathUseCase.BaseKey(key))

	requests := []types.WriteRequest{
		{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{"Path": p, "Key": k},
			},
		},
	}

	entries, _ := d.List(ctx, key)

	// children
	for _, e := range entries {
		pEntry, _ := attributevalue.Marshal(e.Path)
		kEntry, _ := attributevalue.Marshal(e.Key)

		requests = append(requests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					"Path": pEntry, "Key": kEntry,
				},
			},
		})
	}

	result := d.writeReqsBatch(ctx, d.config.EntryTableName, requests)
	return result.Err
}

func (d *dynamodbBackend) Tracking(ctx context.Context, key string) ([]models.Tracking, error) {
	entries := make([]models.Tracking, 0)
	keyEx := expression.Key("Key").Equal(expression.Value(key))
	expr, err := expression.NewBuilder().
		WithKeyCondition(keyEx).
		Build()
	if err != nil {
		d.logger.Error("ErrExpressionBuilder", zap.Error(err))
		return nil, fmt.Errorf("failed to build DynamoDB expression: %w", err)
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(d.config.TrackingEntryTableName),
		ConsistentRead:            aws.Bool(true),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ScanIndexForward:          aws.Bool(false),
	}
	queryPaginator := dynamodb.NewQueryPaginator(d.client, queryInput)
	var response *dynamodb.QueryOutput
	for queryPaginator.HasMorePages() {
		response, err = queryPaginator.NextPage(ctx)
		if err != nil {
			d.logger.Error("ErrQueryPaginator", zap.Error(err), zap.String("key", key))
			return nil, fmt.Errorf("failed to get next page from DynamoDB: %w", err)
		}
		var records []Record
		err = attributevalue.UnmarshalListOfMaps(response.Items, &records)
		if err != nil {
			d.logger.Error("ErrUnmarshalListOfMaps", zap.Error(err), zap.String("key", key))
			return nil, fmt.Errorf("failed to unmarshal DynamoDB items: %w", err)
		}

		for _, record := range records {
			if !strings.HasPrefix(record.Key, DynamoDBLockPrefix) {
				entries = append(entries, models.Tracking{
					Key:       record.Key,
					Value:     string(record.Value),
					Secure:    record.Metadata.Secure,
					UpdatedAt: record.Metadata.UpdatedAt,
					UpdatedBy: record.Metadata.UpdatedBy,
				})
			}
		}
	}

	return entries, nil
}

func prepareWriteRequest[T any](items map[string]T) []types.WriteRequest {
	writeReqs := make([]types.WriteRequest, 0, len(items))
	var item map[string]types.AttributeValue
	var err error

	for _, r := range items {
		item, err = attributevalue.MarshalMap(r)
		if err != nil {
			log.Printf("Err could not convert Record to DynamoDB item: %v", err)
			continue
		}
		writeReqs = append(
			writeReqs, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}},
		)
	}

	return writeReqs
}
