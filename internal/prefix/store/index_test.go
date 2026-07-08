package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nbox/internal/entry"
	"nbox/internal/prefix"
)

func TestPrefixIndex_LPMAndNotFound(t *testing.T) {
	raw := []byte(`[
		{"prefix":"development","typeDefault":"dynamodb","typeSecure":"parameterstore_secure","typeAllowed":[]},
		{"prefix":"development/myapp","typeDefault":"parameterstore","typeSecure":"parameterstore_secure","typeAllowed":[]}
	]`)
	ix, err := ParseIndex(raw, entry.NewProcessor())
	require.NoError(t, err)

	// Longest match wins.
	got, err := ix.ByPrefix("development/myapp/db/password")
	require.NoError(t, err)
	assert.Equal(t, "development/myapp", got.Prefix)
	assert.Equal(t, prefix.BackendParameterStore, got.TypeDefault)

	// Falls back to the shorter prefix.
	got, err = ix.ByPrefix("development/other/key")
	require.NoError(t, err)
	assert.Equal(t, "development", got.Prefix)

	// No candidate matches.
	_, err = ix.ByPrefix("qa/whatever")
	require.ErrorIs(t, err, entry.ErrEntryNotFound)

	// Empty key.
	_, err = ix.ByPrefix("/")
	require.Error(t, err)

	// List sorted.
	list := ix.List()
	require.Len(t, list, 2)
	assert.Equal(t, "development", list[0].Prefix)
	assert.Equal(t, "development/myapp", list[1].Prefix)
}

// TestPrefixIndex_TieBreak verifies the alphabetical tie-break when two
// candidate prefixes have the same length. "aaa" < "aab", so a key under
// "aaa" must resolve to the "aaa" config, exercising the equal-length branch.
func TestPrefixIndex_TieBreak(t *testing.T) {
	raw := []byte(`[
		{"prefix":"aaa","typeDefault":"dynamodb","typeSecure":"parameterstore_secure","typeAllowed":[]},
		{"prefix":"aab","typeDefault":"parameterstore","typeSecure":"parameterstore_secure","typeAllowed":[]}
	]`)
	ix, err := ParseIndex(raw, entry.NewProcessor())
	require.NoError(t, err)

	// Key starts with "aaa" — only "aaa" matches; confirm it wins (not "aab").
	got, err := ix.ByPrefix("aaa/mykey")
	require.NoError(t, err)
	assert.Equal(t, "aaa", got.Prefix)
	assert.Equal(t, prefix.BackendDynamoDB, got.TypeDefault)

	// Symmetry: "aab" key resolves to "aab", not "aaa".
	got, err = ix.ByPrefix("aab/mykey")
	require.NoError(t, err)
	assert.Equal(t, "aab", got.Prefix)
	assert.Equal(t, prefix.BackendParameterStore, got.TypeDefault)
}

// TestParseIndex_BadJSON confirms that malformed JSON produces an error
// whose message wraps "prefix index parse".
func TestParseIndex_BadJSON(t *testing.T) {
	_, err := ParseIndex([]byte(`{not valid json`), entry.NewProcessor())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prefix index parse")
}
