package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"nbox/internal/entry"
	"nbox/internal/prefix"
)

// PrefixIndex is an immutable, in-memory view of all prefix configs, built once
// per config Snapshot refresh. ByPrefix does longest-prefix-match locally — no
// DynamoDB round-trip per resolve (unlike the legacy BatchGet store).
type PrefixIndex struct {
	proc     *entry.Processor
	byPrefix map[string]prefix.Config // keyed by trimmed prefix
}

// ParseIndex builds the index from the assembled config array ([cfg,cfg,...]).
func ParseIndex(raw []byte, proc *entry.Processor) (*PrefixIndex, error) {
	var configs []prefix.Config
	if err := json.Unmarshal(raw, &configs); err != nil {
		return nil, fmt.Errorf("prefix index parse: %w", err)
	}
	m := make(map[string]prefix.Config, len(configs))
	for _, c := range configs {
		m[strings.Trim(c.Prefix, "/")] = c
	}
	return &PrefixIndex{proc: proc, byPrefix: m}, nil
}

// ByPrefix returns the most specific matching config (LPM), or
// entry.ErrEntryNotFound when none of the prefix candidates match.
func (ix *PrefixIndex) ByPrefix(key string) (*prefix.Config, error) {
	cleanKey := strings.Trim(key, "/")
	if cleanKey == "" {
		return nil, errors.New("prefix cannot be empty")
	}
	candidates := append(ix.proc.Prefixes(cleanKey), cleanKey)

	var best *prefix.Config
	maxLen := -1
	for _, c := range candidates {
		cfg, ok := ix.byPrefix[c]
		if !ok {
			continue
		}
		if len(cfg.Prefix) > maxLen ||
			(len(cfg.Prefix) == maxLen && cfg.Prefix < best.Prefix) {
			cp := cfg
			best = &cp
			maxLen = len(cfg.Prefix)
		}
	}
	if best == nil {
		return nil, entry.ErrEntryNotFound
	}
	return best, nil
}

// List returns all configs sorted by prefix.
func (ix *PrefixIndex) List() []prefix.Config {
	out := make([]prefix.Config, 0, len(ix.byPrefix))
	for _, c := range ix.byPrefix {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })
	return out
}
