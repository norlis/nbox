package config

import (
	"context"
	"os"
)

// Shape is how a kind's items combine into the JSON the domain parser expects.
type Shape int

const (
	// ShapeArray: [d1,d2,...] — app_role, arn_map (NewInMemory wants []T).
	ShapeArray Shape = iota
	// ShapeObject: {"id":d,...} — basic_auth (NewInMemory wants a map by username).
	ShapeObject
)

// Key identifies one config domain across the resolution chain.
type Key struct {
	Kind   string // DynamoDB partition value, e.g. "app_role"
	EnvVar string // env var checked first, e.g. "NBOX_APPROLE_ROLES"
	Shape  Shape
}

// Empty returns the empty JSON literal for the shape, so the parser always
// gets valid JSON even with no data.
func (k Key) Empty() []byte {
	if k.Shape == ShapeObject {
		return []byte("{}")
	}
	return []byte("[]")
}

// Predefined keys for the three auth configs.
var (
	KeyBasicAuth = Key{Kind: "basic_auth", EnvVar: "NBOX_BASIC_AUTH_CREDENTIALS", Shape: ShapeObject}
	KeyAppRole   = Key{Kind: "app_role", EnvVar: "NBOX_APPROLE_ROLES", Shape: ShapeArray}
	KeyARNMap    = Key{Kind: "arn_map", EnvVar: "NBOX_AWS_ARN_MAP", Shape: ShapeArray}

	// KeyPrefixConfig is the storage-routing config domain. id = the prefix
	// string; data = JSON of one prefix.Config. EnvVar lets local dev inject the
	// whole array; empty table+env ⇒ no prefixes (ByPrefix → not-found).
	KeyPrefixConfig = Key{Kind: "prefix_config", EnvVar: "NBOX_PREFIX_CONFIG_JSON", Shape: ShapeArray}
)

// Source resolves the raw JSON payload for a key.
type Source interface {
	// Get returns the payload; found=false (nil err) means "try the next source".
	Get(ctx context.Context, k Key) (value []byte, found bool, err error)
}

// EnvSource reads the key's EnvVar. Found only when non-empty.
type EnvSource struct{}

func (EnvSource) Get(_ context.Context, k Key) (value []byte, found bool, err error) {
	v := os.Getenv(k.EnvVar)
	if v == "" {
		return nil, false, nil
	}
	return []byte(v), true, nil
}

// Chain tries sources in order, returning the first found. None found =>
// found=false (the Snapshot then uses Key.Empty()).
type Chain struct {
	sources []Source
}

func NewChain(sources ...Source) *Chain { return &Chain{sources: sources} }

func (c *Chain) Get(ctx context.Context, k Key) (value []byte, found bool, err error) {
	for _, s := range c.sources {
		v, ok, e := s.Get(ctx, k)
		if e != nil {
			return nil, false, e
		}
		if ok {
			return v, true, nil
		}
	}
	return nil, false, nil
}
