package config

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// AdminStore is the write path to the config table (used by the CLI).
// Reads via Query (List), writes via PutItem, removes via DeleteItem. Never Scan.
type AdminStore struct {
	client DynamoAPI
	table  string
}

func NewAdminStore(client DynamoAPI, table string) *AdminStore {
	return &AdminStore{client: client, table: table}
}

// Upsert writes one entity. data is the JSON of one domain entity.
// Bumps version and stamps audit fields.
func (a *AdminStore) Upsert(ctx context.Context, kind, id string, data []byte, by string) error {
	cur, _ := a.Get(ctx, kind, id)
	rec := Record{
		Kind:      kind,
		ID:        id,
		Data:      string(data),
		Version:   1,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedBy: by,
	}
	if cur != nil {
		rec.Version = cur.Version + 1
	}
	item, err := attributevalue.MarshalMap(rec)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if _, err := a.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: new(a.table),
		Item:      item,
	}); err != nil {
		return fmt.Errorf("config: put %s/%s: %w", kind, id, err)
	}
	return nil
}

func (a *AdminStore) List(ctx context.Context, kind string) ([]Record, error) {
	return query(ctx, a.client, a.table, kind)
}

func (a *AdminStore) Delete(ctx context.Context, kind, id string) error {
	if _, err := a.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: new(a.table),
		Key:       keyOf(kind, id),
	}); err != nil {
		return fmt.Errorf("config: delete %s/%s: %w", kind, id, err)
	}
	return nil
}

// Get returns one record, or (nil, nil) when it does not exist. Used by the
// CLI to merge partial updates onto the existing entity.
func (a *AdminStore) Get(ctx context.Context, kind, id string) (*Record, error) {
	out, err := a.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: new(a.table),
		Key:       keyOf(kind, id),
	})
	if err != nil || out.Item == nil {
		return nil, err
	}
	var rec Record
	if err := attributevalue.UnmarshalMap(out.Item, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func keyOf(kind, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"kind": &types.AttributeValueMemberS{Value: kind},
		"id":   &types.AttributeValueMemberS{Value: id},
	}
}
