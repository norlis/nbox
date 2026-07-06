package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoAPI is the subset of *dynamodb.Client used here. Decouples from the
// concrete client (DIP) and allows mocking.
type DynamoAPI interface {
	Query(ctx context.Context, in *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// NewSourceChain builds the env->DynamoDB chain. Empty table => env only.
// Centralized so each binary doesn't duplicate it (DRY).
func NewSourceChain(api DynamoAPI, table string) Source {
	sources := []Source{EnvSource{}}
	if table != "" {
		sources = append(sources, NewDynamoSource(api, table))
	}
	return NewChain(sources...)
}

// DynamoSource reads a kind's items via Query (NEVER Scan) and assembles
// them into the JSON shape the domain parser expects.
type DynamoSource struct {
	client DynamoAPI
	table  string
}

func NewDynamoSource(client DynamoAPI, table string) *DynamoSource {
	return &DynamoSource{client: client, table: table}
}

func (d *DynamoSource) Get(ctx context.Context, k Key) (value []byte, found bool, err error) {
	recs, err := query(ctx, d.client, d.table, k.Kind)
	if err != nil {
		return nil, false, err
	}
	// A successful (even empty) query counts as found: an empty array/object
	// is a valid config (store with no entries).
	return assemble(k, recs), true, nil
}

// query returns all Records for a kind, paginating. Never Scan.
// client is DynamoAPI: NewQueryPaginator only needs Query.
func query(ctx context.Context, client DynamoAPI, table, kind string) ([]Record, error) {
	keyCond := expression.Key("kind").Equal(expression.Value(kind))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("config: build query expr: %w", err)
	}

	var out []Record
	p := dynamodb.NewQueryPaginator(client, &dynamodb.QueryInput{
		TableName:                 new(table),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("config: query %q: %w", kind, err)
		}
		var chunk []Record
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &chunk); err != nil {
			return nil, fmt.Errorf("config: unmarshal %q: %w", kind, err)
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// EnvValue wraps one entity's JSON into the collection shape of the key's env
// var (object for ShapeObject, array for ShapeArray) — the value you paste into
// NBOX_*. Reuses assemble so the shape logic lives in one place.
func EnvValue(k Key, id string, data []byte) []byte {
	return assemble(k, []Record{{ID: id, Data: string(data)}})
}

// assemble joins each Record's Data into the array/object the parser wants.
func assemble(k Key, recs []Record) []byte {
	parts := make([]string, 0, len(recs))
	for _, r := range recs {
		if k.Shape == ShapeObject {
			parts = append(parts, fmt.Sprintf("%q:%s", r.ID, r.Data))
		} else {
			parts = append(parts, r.Data)
		}
	}
	joined := strings.Join(parts, ",")
	if k.Shape == ShapeObject {
		return []byte("{" + joined + "}")
	}
	return []byte("[" + joined + "]")
}
