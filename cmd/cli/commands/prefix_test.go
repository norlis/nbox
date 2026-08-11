package commands

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nbox/internal/config"
	"nbox/internal/prefix"
)

func TestBuildPrefixConfig(t *testing.T) {
	// Create sin overrides ⇒ default dynamodb, allowed normalizado = [dynamodb].
	c, err := buildPrefixConfig("global", nil, prefixOverrides{})
	require.NoError(t, err)
	assert.Equal(t, prefix.BackendDynamoDB, c.TypeDefault)
	assert.Equal(t, "global", c.Prefix)
	assert.Equal(t, []prefix.StorageBackendType{prefix.BackendDynamoDB}, c.TypeAllowed)

	// Merge: cambia solo --secure; allowed = default + secure + existentes, dedup.
	cur := &prefix.Config{Prefix: "global", TypeDefault: prefix.BackendParameterStore, TypeAllowed: []prefix.StorageBackendType{prefix.BackendDynamoDB}}
	sec := "parameterstore_secure"
	c, err = buildPrefixConfig("global", cur, prefixOverrides{typeSecure: &sec})
	require.NoError(t, err)
	assert.Equal(t, prefix.BackendParameterStore, c.TypeDefault)
	assert.Equal(t, prefix.BackendParameterStoreSecure, c.TypeSecure)
	assert.Equal(t, []prefix.StorageBackendType{prefix.BackendParameterStore, prefix.BackendParameterStoreSecure, prefix.BackendDynamoDB}, c.TypeAllowed)

	// --allowed lista: default (dynamodb) inyectado primero, dedup.
	al := []string{"parameterstore_secure", "dynamodb"}
	c, err = buildPrefixConfig("global", nil, prefixOverrides{allowed: &al})
	require.NoError(t, err)
	assert.Equal(t, []prefix.StorageBackendType{prefix.BackendDynamoDB, prefix.BackendParameterStoreSecure}, c.TypeAllowed)

	// Backend inválido ⇒ error.
	bad := "garbage"
	_, err = buildPrefixConfig("global", nil, prefixOverrides{typeDefault: &bad})
	assert.Error(t, err)
}

func TestApplyRegion(t *testing.T) {
	// Non-empty region ⇒ setenv called with ("AWS_REGION", region), even if already set (flag wins).
	var got [2]string
	set := func(k, v string) error { got[0], got[1] = k, v; return nil }
	require.NoError(t, applyRegion("us-east-1", set))
	assert.Equal(t, [2]string{"AWS_REGION", "us-east-1"}, got)

	// Flag wins even when AWS_REGION is already set (simulated by calling again with a different value).
	require.NoError(t, applyRegion("eu-west-1", set))
	assert.Equal(t, [2]string{"AWS_REGION", "eu-west-1"}, got)

	// Empty region is a no-op — setenv must NOT be called.
	require.NoError(t, applyRegion("",
		func(string, string) error { return errors.New("should not be called") }))
}

func TestFormatPrefixList(t *testing.T) {
	recs := []config.Record{
		{ID: "global", Data: `{"prefix":"global","typeDefault":"dynamodb","typeSecure":"parameterstore_secure","typeAllowed":["parameterstore"]}`},
	}

	table, err := formatPrefixList(recs, false)
	require.NoError(t, err)
	assert.Contains(t, table, "global")
	assert.Contains(t, table, "default=dynamodb")

	jsonOut, err := formatPrefixList(recs, true)
	require.NoError(t, err)
	var got []prefix.Config
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "global", got[0].Prefix)

	// Bad data surfaces an error, not a silent skip.
	_, err = formatPrefixList([]config.Record{{ID: "x", Data: "not-json"}}, false)
	assert.Error(t, err)
}

func TestConfirmDelete(t *testing.T) {
	// --force ⇒ proceed, no prompt.
	ok, err := confirmDelete(true, false, strings.NewReader(""), "x")
	require.NoError(t, err)
	assert.True(t, ok)

	// Non-interactive without --force ⇒ refused (fail-closed).
	_, err = confirmDelete(false, false, strings.NewReader(""), "x")
	require.Error(t, err)

	// Interactive: y ⇒ proceed, n ⇒ abort.
	ok, err = confirmDelete(false, true, strings.NewReader("y\n"), "x")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = confirmDelete(false, true, strings.NewReader("n\n"), "x")
	require.NoError(t, err)
	assert.False(t, ok)
}
