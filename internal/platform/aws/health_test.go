package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"nbox/internal/nbox"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeDynamoDB is a thread-safe fake for dynamoDescribeTableAPI.
type fakeDynamoDB struct {
	mu      sync.Mutex
	results map[string]error // tableName -> error to return
	calls   []string         // recorded call log
}

func (f *fakeDynamoDB) DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	f.mu.Lock()
	f.calls = append(f.calls, *params.TableName)
	err := f.results[*params.TableName]
	f.mu.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return &dynamodb.DescribeTableOutput{}, nil
}

// fakeS3 is a fake for s3HeadBucketAPI.
type fakeS3 struct {
	err error
}

func (f *fakeS3) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &s3.HeadBucketOutput{}, nil
}

// makeResponseError builds a *smithyhttp.ResponseError with the given HTTP status code,
// allowing tests to exercise the real classification branch in S3Checker.Check.
func makeResponseError(statusCode int) *smithyhttp.ResponseError {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{
			Response: &http.Response{StatusCode: statusCode},
		},
		Err: fmt.Errorf("http %d", statusCode),
	}
}

// makeConfig returns a *nbox.Config with the given table names.
func makeConfig(entry, tracking, box, prefix string) *nbox.Config {
	return &nbox.Config{
		EntryTableName:         entry,
		TrackingEntryTableName: tracking,
		BoxTableName:           box,
		ConfigTableName:        prefix,
	}
}

// ---------------------------------------------------------------------------
// DynamoDBChecker tests
// ---------------------------------------------------------------------------

func TestDynamoDBChecker(t *testing.T) {
	t.Parallel()

	allTables := []string{"entries", "tracking", "boxes", "prefixes"}

	t.Run("all tables OK returns nil", func(t *testing.T) {
		t.Parallel()
		fake := &fakeDynamoDB{results: map[string]error{}}
		checker := &DynamoDBChecker{
			client: fake,
			config: makeConfig("entries", "tracking", "boxes", "prefixes"),
		}
		if err := checker.Check(context.Background()); err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
		// All 4 tables were described.
		fake.mu.Lock()
		got := len(fake.calls)
		fake.mu.Unlock()
		if got != len(allTables) {
			t.Fatalf("expected %d calls, got %d", len(allTables), got)
		}
	})

	t.Run("one table returns error wraps ErrDynamoDBTableCheckFailed and names table", func(t *testing.T) {
		t.Parallel()
		tableErr := errors.New("resource not found")
		fake := &fakeDynamoDB{
			results: map[string]error{
				"tracking": tableErr,
			},
		}
		checker := &DynamoDBChecker{
			client: fake,
			config: makeConfig("entries", "tracking", "boxes", "prefixes"),
		}
		err := checker.Check(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrDynamoDBTableCheckFailed) {
			t.Errorf("expected error to wrap ErrDynamoDBTableCheckFailed, got: %v", err)
		}
		// The error message must contain the failing table name.
		if errMsg := err.Error(); !contains(errMsg, "tracking") {
			t.Errorf("expected error to mention table name 'tracking', got: %v", errMsg)
		}
	})

	t.Run("empty table name returns ErrDynamoDBTableNotConfigured before goroutines", func(t *testing.T) {
		t.Parallel()
		fake := &fakeDynamoDB{results: map[string]error{}}
		checker := &DynamoDBChecker{
			client: fake,
			// BoxTableName is empty
			config: makeConfig("entries", "tracking", "", "prefixes"),
		}
		err := checker.Check(context.Background())
		if !errors.Is(err, ErrDynamoDBTableNotConfigured) {
			t.Fatalf("expected ErrDynamoDBTableNotConfigured, got: %v", err)
		}
		// No goroutines should have been launched — no calls recorded.
		fake.mu.Lock()
		calls := len(fake.calls)
		fake.mu.Unlock()
		if calls != 0 {
			t.Errorf("expected 0 DescribeTable calls (config guard), got %d", calls)
		}
	})

	t.Run("already canceled context returns error", func(t *testing.T) {
		t.Parallel()
		fake := &fakeDynamoDB{results: map[string]error{}}
		checker := &DynamoDBChecker{
			client: fake,
			config: makeConfig("entries", "tracking", "boxes", "prefixes"),
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately
		err := checker.Check(ctx)
		if err == nil {
			t.Fatal("expected error for canceled context, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// S3Checker tests
// ---------------------------------------------------------------------------

func TestS3Checker(t *testing.T) {
	t.Parallel()

	bucketConfig := func(name string) *nbox.Config {
		return &nbox.Config{BucketName: name}
	}

	t.Run("HeadBucket nil returns nil", func(t *testing.T) {
		t.Parallel()
		checker := &S3Checker{
			client: &fakeS3{err: nil},
			config: bucketConfig("my-bucket"),
		}
		if err := checker.Check(context.Background()); err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
	})

	t.Run("HeadBucket 403 Forbidden returns nil (bucket is reachable)", func(t *testing.T) {
		t.Parallel()
		checker := &S3Checker{
			client: &fakeS3{err: makeResponseError(http.StatusForbidden)},
			config: bucketConfig("my-bucket"),
		}
		if err := checker.Check(context.Background()); err != nil {
			t.Fatalf("expected nil for 403 (bucket reachable), got: %v", err)
		}
	})

	t.Run("HeadBucket 404 NotFound returns error", func(t *testing.T) {
		t.Parallel()
		checker := &S3Checker{
			client: &fakeS3{err: makeResponseError(http.StatusNotFound)},
			config: bucketConfig("my-bucket"),
		}
		err := checker.Check(context.Background())
		if err == nil {
			t.Fatal("expected error for 404, got nil")
		}
		if !errors.Is(err, ErrS3BucketCheckFailed) {
			t.Errorf("expected ErrS3BucketCheckFailed, got: %v", err)
		}
	})

	t.Run("HeadBucket generic error returns error", func(t *testing.T) {
		t.Parallel()
		checker := &S3Checker{
			client: &fakeS3{err: errors.New("connection refused")},
			config: bucketConfig("my-bucket"),
		}
		err := checker.Check(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrS3BucketCheckFailed) {
			t.Errorf("expected ErrS3BucketCheckFailed, got: %v", err)
		}
	})

	t.Run("empty bucket name returns ErrS3BucketNotConfigured", func(t *testing.T) {
		t.Parallel()
		checker := &S3Checker{
			client: &fakeS3{},
			config: bucketConfig(""),
		}
		err := checker.Check(context.Background())
		if !errors.Is(err, ErrS3BucketNotConfigured) {
			t.Fatalf("expected ErrS3BucketNotConfigured, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
