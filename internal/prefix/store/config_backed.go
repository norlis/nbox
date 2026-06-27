package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nbox/internal/config"
	"nbox/internal/prefix"
)

// indexSource is the read side: config.Snapshot[*PrefixIndex] satisfies it
// (Current() *PrefixIndex). An interface so the store is unit-testable.
type indexSource interface {
	Current() *PrefixIndex
}

// ConfigBacked is a prefix.Store reading from the cached config Snapshot and
// writing through config.AdminStore (kind="prefix_config"). Service writes are
// rare (none on the hot path); readers observe them within NBOX_CONFIG_TTL.
type ConfigBacked struct {
	idx   indexSource
	admin *config.AdminStore
}

func NewConfigBacked(idx indexSource, admin *config.AdminStore) prefix.Store {
	return &ConfigBacked{idx: idx, admin: admin}
}

func (c *ConfigBacked) ByPrefix(_ context.Context, key string) (*prefix.Config, error) {
	return c.idx.Current().ByPrefix(key)
}

func (c *ConfigBacked) List(_ context.Context) ([]prefix.Config, error) {
	return c.idx.Current().List(), nil
}

func (c *ConfigBacked) Upsert(ctx context.Context, prefixes []prefix.Config) (prefix.UpsertStats, error) {
	stats := prefix.UpsertStats{}
	for _, p := range prefixes {
		data, err := json.Marshal(p)
		if err != nil {
			stats.Failed++
			continue
		}
		id := strings.Trim(p.Prefix, "/")
		if err := c.admin.Upsert(ctx, config.KeyPrefixConfig.Kind, id, data, "nbox"); err != nil {
			stats.Failed++
			continue
		}
		stats.Processed++
	}
	if stats.Failed > 0 {
		return stats, fmt.Errorf("prefix upsert: %d failed", stats.Failed)
	}
	return stats, nil
}
