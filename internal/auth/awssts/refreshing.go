package awssts

import (
	"context"

	"nbox/internal/config"
)

// RefreshingStore implements Store by delegating to a config.Snapshot that
// hot-reloads the underlying *InMemory.
type RefreshingStore struct {
	snap *config.Snapshot[*InMemory]
}

func NewRefreshingStore(snap *config.Snapshot[*InMemory]) *RefreshingStore {
	return &RefreshingStore{snap: snap}
}

func (r *RefreshingStore) LookupByARN(ctx context.Context, arn string) (*ARNMapping, error) {
	return r.snap.Current().LookupByARN(ctx, arn)
}
