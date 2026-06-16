package awssts

import (
	"context"
	"encoding/json"
	"fmt"
)

// Store resolves an ARN mapping by exact ARN match.
// Implementations must be safe for concurrent use.
type Store interface {
	LookupByARN(ctx context.Context, arn string) (*ARNMapping, error)
}

// InMemory is a Store backed by an in-memory map seeded from JSON at
// construction. Immutable after NewInMemory returns — safe for
// concurrent reads without locking.
type InMemory struct {
	byARN map[string]ARNMapping
}

// NewInMemory parses a JSON array of ARNMapping objects from rawJSON.
// Empty input (nil or zero-length) produces an empty store; LookupByARN
// returns ErrUnknownARN for any input. If the same ARN appears more
// than once, the last occurrence wins.
//
// Example payload:
//
//	[
//	  {
//	    "arn": "arn:aws:iam::123456789012:role/entrypushd-watcher",
//	    "name": "production-watcher",
//	    "roles": ["entrypushd"],
//	    "status": "active"
//	  }
//	]
func NewInMemory(rawJSON []byte) (*InMemory, error) {
	if len(rawJSON) == 0 {
		return &InMemory{byARN: map[string]ARNMapping{}}, nil
	}
	var mappings []ARNMapping
	if err := json.Unmarshal(rawJSON, &mappings); err != nil {
		return nil, fmt.Errorf("awssts: parse arn map json: %w", err)
	}
	s := &InMemory{byARN: make(map[string]ARNMapping, len(mappings))}
	for _, m := range mappings {
		if m.ARN == "" {
			return nil, fmt.Errorf("awssts: mapping with empty arn (name=%q)", m.Name)
		}
		if m.Status == "" {
			m.Status = StatusActive
		}
		s.byARN[m.ARN] = m
	}
	return s, nil
}

func (s *InMemory) LookupByARN(_ context.Context, arn string) (*ARNMapping, error) {
	m, ok := s.byARN[arn]
	if !ok {
		return nil, ErrUnknownARN
	}
	return &m, nil
}
