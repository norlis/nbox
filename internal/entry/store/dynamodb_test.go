package store

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nbox/internal/entry"
	"nbox/internal/nbox"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeQueryClient implements dynamodbClientAPI using an in-memory item table.
// It only supports Query (used by Delete and List) and a no-op GetItem.
type fakeQueryClient struct {
	mu    sync.Mutex
	items []fakeItem // all items in the "table"
}

type fakeItem struct {
	Path  string
	Key   string
	Value string
}

// Query filters items by Path equality.  The real code always builds its key
// condition expression as "#0 = :0" where #0 → "Path" and :0 → the path value.
// We decode ":0" from ExpressionAttributeValues to obtain the target path.
func (f *fakeQueryClient) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	// Decode the equality value from ExpressionAttributeValues[":0"].
	av, ok := params.ExpressionAttributeValues[":0"]
	if !ok {
		return &dynamodb.QueryOutput{}, nil
	}
	var targetPath string
	if err := attributevalue.Unmarshal(av, &targetPath); err != nil {
		return &dynamodb.QueryOutput{}, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var out []map[string]types.AttributeValue
	for _, it := range f.items {
		if it.Path == targetPath {
			r := record{
				Path: it.Path,
				recordBase: recordBase{
					Key:   it.Key,
					Value: []byte(it.Value),
				},
			}
			av, _ := attributevalue.MarshalMap(r)
			out = append(out, av)
		}
	}
	// Return all matching items in a single page (no LastEvaluatedKey).
	return &dynamodb.QueryOutput{Items: out}, nil
}

// Scan returns every item in a single page (no LastEvaluatedKey), marshalled
// through the same record type production unmarshals.
func (f *fakeQueryClient) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []map[string]types.AttributeValue
	for _, it := range f.items {
		r := record{
			Path: it.Path,
			recordBase: recordBase{
				Key:   it.Key,
				Value: []byte(it.Value),
			},
		}
		av, _ := attributevalue.MarshalMap(r)
		out = append(out, av)
	}
	return &dynamodb.ScanOutput{Items: out}, nil
}

// GetItem is required by the interface but not exercised in Delete tests.
func (f *fakeQueryClient) GetItem(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

// ---------------------------------------------------------------------------

// fakeBatchWriter implements dynamodbKitAPI.
// BatchWrite deletes items from the in-memory set; BatchGet is a no-op.
type fakeBatchWriter struct {
	client *fakeQueryClient // shared reference so deletes affect the query results
}

func (f *fakeBatchWriter) BatchWrite(_ context.Context, _ string, requests []types.WriteRequest) ([]types.WriteRequest, error) {
	f.client.mu.Lock()
	defer f.client.mu.Unlock()

	for _, req := range requests {
		if req.DeleteRequest == nil {
			continue
		}
		var path, key string
		_ = attributevalue.Unmarshal(req.DeleteRequest.Key["Path"], &path)
		_ = attributevalue.Unmarshal(req.DeleteRequest.Key["Key"], &key)

		kept := f.client.items[:0]
		for _, it := range f.client.items {
			if it.Path == path && it.Key == key {
				continue // remove this item
			}
			kept = append(kept, it)
		}
		f.client.items = kept
	}
	return nil, nil
}

func (f *fakeBatchWriter) BatchGet(_ context.Context, _ string, _ []map[string]types.AttributeValue) ([]map[string]types.AttributeValue, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestDynamoDB(items []fakeItem) (*DynamoDB, *fakeQueryClient) {
	client := &fakeQueryClient{items: items}
	kit := &fakeBatchWriter{client: client}
	proc := entry.NewProcessor()
	cfg := &nbox.Config{EntryTableName: "test-table"}
	logger, _ := zap.NewDevelopment()

	db := &DynamoDB{
		client:      client,
		config:      cfg,
		pathUseCase: proc,
		logger:      logger,
		dynamodbKit: kit,
	}
	return db, client
}

// keys returns the (Path, Key) pairs still present in the fake, sorted for
// stable comparison.
func remaining(client *fakeQueryClient) []fakeItem {
	client.mu.Lock()
	defer client.mu.Unlock()
	out := make([]fakeItem, len(client.items))
	copy(out, client.items)
	return out
}

func containsItem(items []fakeItem, path, key string) bool {
	for _, it := range items {
		if it.Path == path && it.Key == key {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Seed a tree that reflects actual DynamoDB storage layout.
//
// When Upsert("svc/bar/baz") is called, the following items are written:
//   (Path=" ",        Key="svc/")      ← dir marker for "svc" in root partition
//   (Path="svc",      Key="bar/")      ← dir marker for svc/bar
//   (Path="svc/bar",  Key="baz")       ← the actual leaf entry
//
// When Upsert("svc/bar/qux/deep") is called, additionally:
//   (Path="svc/bar",  Key="qux/")      ← dir marker for svc/bar/qux
//   (Path="svc/bar/qux", Key="deep")   ← the actual leaf entry (GRANDCHILD)
//
// Delete("svc/bar") targets:
//   root item:  PathWithoutKey("svc/bar")="svc", BaseKey("svc/bar")="bar"
//               → (Path="svc", Key="bar")   — a leaf at svc/bar if it exists
//   children:   items with Path = EscapeEmptyPath("svc/bar") = "svc/bar"
//               → (svc/bar, baz) and (svc/bar, qux/)
//   grandchildren: items with Path = "svc/bar/qux"  ← MISSED by old non-recursive code
//               → (svc/bar/qux, deep)
//
// Note: the dir markers (svc, bar/) are in a DIFFERENT partition ("svc") and are
// found/deleted during the BFS scan of Path="svc" only when that level is queued.
// For the Delete("svc/bar") call, we test that the GRANDCHILD is not orphaned.

func seedTree() []fakeItem {
	return []fakeItem{
		// Root item that Delete("svc/bar") targets (a real leaf at path svc/bar).
		// PathWithoutKey("svc/bar") = "svc", BaseKey("svc/bar") = "bar".
		{Path: "svc", Key: "bar"},
		// Direct children of svc/bar.
		{Path: "svc/bar", Key: "baz"},
		// Directory marker for svc/bar/qux — Key ends in "/" so BFS will enqueue "svc/bar/qux".
		{Path: "svc/bar", Key: "qux/"},
		// Grandchild: lives in partition "svc/bar/qux". Old code orphaned this.
		{Path: "svc/bar/qux", Key: "deep"},
		// Unrelated item — must survive.
		{Path: "other", Key: "x"},
	}
}

func TestDelete_Subtree_Recursive(t *testing.T) {
	// Delete("svc/bar") must remove the root item AND all descendants,
	// including the grandchild that the old non-recursive code left orphaned.
	//
	// PathWithoutKey("svc/bar") = "svc", BaseKey("svc/bar") = "bar"
	//
	// Must be deleted:
	//   (svc, bar)          — root item
	//   (svc/bar, baz)      — direct child leaf
	//   (svc/bar, qux/)     — direct child directory marker
	//   (svc/bar/qux, deep) — grandchild leaf (orphaned by old code)
	//
	// Must survive:
	//   (other, x)          — unrelated entry

	db, client := newTestDynamoDB(seedTree())

	err := db.Delete(context.Background(), "svc/bar")
	if err != nil {
		t.Fatalf("Delete returned unexpected error: %v", err)
	}

	left := remaining(client)

	// Must be deleted.
	deleted := []fakeItem{
		{Path: "svc", Key: "bar"},
		{Path: "svc/bar", Key: "baz"},
		{Path: "svc/bar", Key: "qux/"},
		{Path: "svc/bar/qux", Key: "deep"},
	}
	for _, it := range deleted {
		if containsItem(left, it.Path, it.Key) {
			t.Errorf("item (%q, %q) should have been deleted but still exists", it.Path, it.Key)
		}
	}

	// Must survive.
	survived := []fakeItem{
		{Path: "other", Key: "x"},
	}
	for _, it := range survived {
		if !containsItem(left, it.Path, it.Key) {
			t.Errorf("item (%q, %q) should have survived but was deleted", it.Path, it.Key)
		}
	}
}

func TestDelete_Leaf_SingleItem(t *testing.T) {
	// Deleting a leaf key should remove exactly that item and nothing else.
	// Delete("svc/bar/baz"):
	//   PathWithoutKey("svc/bar/baz") = "svc/bar", BaseKey("svc/bar/baz") = "baz"
	//   Root delete: (svc/bar, baz)
	//   Children scan: Path = "svc/bar/baz" → nothing
	db, client := newTestDynamoDB(seedTree())

	err := db.Delete(context.Background(), "svc/bar/baz")
	if err != nil {
		t.Fatalf("Delete returned unexpected error: %v", err)
	}

	left := remaining(client)

	if containsItem(left, "svc/bar", "baz") {
		t.Error("leaf item (svc/bar, baz) should have been deleted")
	}

	// Everything else must survive.
	mustSurvive := []fakeItem{
		{Path: "svc", Key: "bar"},
		{Path: "svc/bar", Key: "qux/"},
		{Path: "svc/bar/qux", Key: "deep"},
		{Path: "other", Key: "x"},
	}
	for _, it := range mustSurvive {
		if !containsItem(left, it.Path, it.Key) {
			t.Errorf("item (%q, %q) should have survived but was deleted", it.Path, it.Key)
		}
	}
}

func TestDelete_Grandchild_OrphanNotLeft(t *testing.T) {
	// Regression: the old non-recursive Delete would leave (svc/bar/qux, deep) orphaned
	// because it only scanned Path="svc/bar" (direct children) and never descended into
	// the "qux/" subdirectory. This test pins the exact previously-failing case.
	db, client := newTestDynamoDB(seedTree())

	_ = db.Delete(context.Background(), "svc/bar")

	left := remaining(client)

	orphan := fakeItem{Path: "svc/bar/qux", Key: "deep"}
	if containsItem(left, orphan.Path, orphan.Key) {
		t.Errorf(
			"grandchild (%q, %q) was orphaned — recursive delete did not descend into sub-directories",
			orphan.Path, orphan.Key,
		)
	}
}

func TestDelete_DirectoryConvention(t *testing.T) {
	// Verify that directory markers follow the Key-ends-in-"/" convention
	// as produced by Upsert's Prefixes loop, which this implementation relies on.
	proc := entry.NewProcessor()
	key := "svc/bar/qux/deep"
	prefixes := proc.Prefixes(key)
	for _, p := range prefixes {
		parentKey := proc.BaseKey(p) + "/"
		if !strings.HasSuffix(parentKey, "/") {
			t.Errorf("prefix %q yielded parentKey %q which does not end in /", p, parentKey)
		}
	}
}

// Tree (real DynamoDB layout):
//
//	(" ",            "passbox/")      ← root folder marker
//	("passbox",      "av")            ← leaf
//	("passbox",      "av2/")          ← folder marker
//	("passbox/av2",  "account-pepe")  ← nested leaf
//	(" ",            "rootleaf")      ← root-level leaf
func seedVaultTree() []fakeItem {
	return []fakeItem{
		{Path: " ", Key: "passbox/", Value: ""},                  // root folder marker
		{Path: "passbox", Key: "av", Value: "secret"},            // real leaf
		{Path: "passbox", Key: "av2/", Value: ""},                // folder marker
		{Path: "passbox/av2", Key: "account-pepe", Value: "tok"}, // nested leaf
		{Path: " ", Key: "rootleaf", Value: "v"},                 // root-level leaf
	}
}

// TestList_LeavesOnly_SubtreeRealKeys: LeavesOnly returns every real key under
// pfx (flat subtree, via one Scan) — folder markers and empty values excluded,
// exact segment boundary on the prefix.
func TestList_LeavesOnly_SubtreeRealKeys(t *testing.T) {
	db, _ := newTestDynamoDB([]fakeItem{
		{Path: "qa", Key: "website-frontend/", Value: ""},                               // folder marker → excluded
		{Path: "qa", Key: "flag", Value: "on"},                                          // real key → included
		{Path: "qa", Key: "empty", Value: ""},                                           // empty value → excluded
		{Path: "qa/website-frontend", Key: "next-public-instagram-token", Value: "tok"}, // nested → included (flat)
		{Path: "qa2", Key: "other", Value: "x"},                                         // sibling prefix → excluded (boundary)
	})

	got, err := db.List(context.Background(), "qa", entry.LeavesOnly())
	require.NoError(t, err)

	keys := make([]string, 0, len(got))
	for _, e := range got {
		keys = append(keys, e.Key)
	}
	require.ElementsMatch(t, []string{"qa/flag", "qa/website-frontend/next-public-instagram-token"}, keys)
}

// TestList_LeavesOnly_RootFallsBackToLevel: at "/" LeavesOnly is ignored and
// the plain level listing is served — a flat dump of the whole table is never
// exposed from root.
func TestList_LeavesOnly_RootFallsBackToLevel(t *testing.T) {
	db, _ := newTestDynamoDB(seedVaultTree())

	got, err := db.List(context.Background(), "/", entry.LeavesOnly())
	require.NoError(t, err)

	keys := make([]string, 0, len(got))
	for _, e := range got {
		keys = append(keys, e.Key)
	}
	// Same as the default level listing at root: markers included, no recursion.
	require.ElementsMatch(t, []string{"passbox/", "rootleaf"}, keys)
}

func TestList_LeavesOnly_EmptyIsNonNilSlice(t *testing.T) {
	db, _ := newTestDynamoDB(nil)

	entries, err := db.List(context.Background(), "passbox/", entry.LeavesOnly())
	require.NoError(t, err)
	require.NotNil(t, entries, "vacío debe ser [] no nil")
	require.Empty(t, entries)
}

func TestList_Default_StillReturnsFolders(t *testing.T) {
	db, _ := newTestDynamoDB(seedVaultTree())

	entries, err := db.List(context.Background(), "passbox/") // sin option
	require.NoError(t, err)

	var keys []string
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	require.ElementsMatch(t, []string{"passbox/av", "passbox/av2/"}, keys,
		"default = un nivel, carpetas incluidas (lo que usa el front)")
}
