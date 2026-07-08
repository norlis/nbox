package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nbox/internal/config"
	"nbox/internal/entry"
	"nbox/internal/prefix"
)

type fakeIndexSource struct{ ix *PrefixIndex }

func (f *fakeIndexSource) Current() *PrefixIndex { return f.ix }

// fakeDynamo satisfies config.DynamoAPI; captures PutItem.
// Set putErr to make every PutItem call return an error.
type fakeDynamo struct {
	put    *dynamodb.PutItemInput
	putErr error
}

func (f *fakeDynamo) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return &dynamodb.QueryOutput{}, nil
}

func (f *fakeDynamo) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.put = in
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeDynamo) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func (f *fakeDynamo) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func TestConfigBacked_ReadAndWrite(t *testing.T) {
	ix, err := ParseIndex([]byte(`[{"prefix":"global","typeDefault":"dynamodb","typeSecure":"parameterstore_secure","typeAllowed":[]}]`), entry.NewProcessor())
	require.NoError(t, err)
	fd := &fakeDynamo{}
	s := NewConfigBacked(&fakeIndexSource{ix: ix}, config.NewAdminStore(fd, "cfg"))

	got, err := s.ByPrefix(context.Background(), "global/x")
	require.NoError(t, err)
	assert.Equal(t, "global", got.Prefix)

	stats, err := s.Upsert(context.Background(), []prefix.Config{
		{Prefix: "global/serverless", TypeDefault: prefix.BackendParameterStore},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Processed)
	require.NotNil(t, fd.put)
	assert.Equal(t, "prefix_config", fd.put.Item["kind"].(*types.AttributeValueMemberS).Value)
	assert.Equal(t, "global/serverless", fd.put.Item["id"].(*types.AttributeValueMemberS).Value)
}

// TestConfigBacked_Upsert_PartialFailure verifies that DynamoDB write errors
// increment stats.Failed and cause Upsert to return a non-nil error.
func TestConfigBacked_Upsert_PartialFailure(t *testing.T) {
	ix, err := ParseIndex([]byte(`[{"prefix":"global","typeDefault":"dynamodb","typeSecure":"parameterstore_secure","typeAllowed":[]}]`), entry.NewProcessor())
	require.NoError(t, err)

	fd := &fakeDynamo{putErr: errors.New("boom")}
	s := NewConfigBacked(&fakeIndexSource{ix: ix}, config.NewAdminStore(fd, "cfg"))

	items := []prefix.Config{
		{Prefix: "global/a", TypeDefault: prefix.BackendDynamoDB},
		{Prefix: "global/b", TypeDefault: prefix.BackendDynamoDB},
	}
	stats, err := s.Upsert(context.Background(), items)

	require.Error(t, err)
	assert.Equal(t, len(items), stats.Failed)
	assert.Equal(t, 0, stats.Processed)
}
