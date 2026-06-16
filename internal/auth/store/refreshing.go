package store

import (
	"context"

	"nbox/internal/auth"
	"nbox/internal/config"
)

// RefreshingStore implements auth.Store by delegating to a config.Snapshot
// that hot-reloads the basic-auth store.
type RefreshingStore struct {
	snap *config.Snapshot[auth.Store]
}

func NewRefreshingStore(snap *config.Snapshot[auth.Store]) *RefreshingStore {
	return &RefreshingStore{snap: snap}
}

func (r *RefreshingStore) FindByUsername(ctx context.Context, username string) (*auth.User, error) {
	return r.snap.Current().FindByUsername(ctx, username)
}

func (r *RefreshingStore) ValidatePassword(ctx context.Context, username, password string) (*auth.User, error) {
	return r.snap.Current().ValidatePassword(ctx, username, password)
}
