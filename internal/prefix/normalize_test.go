package prefix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAllowed(t *testing.T) {
	// El ejemplo del usuario: default inyectado, secure ya presente → dedup.
	c := Config{
		TypeDefault: BackendParameterStore,
		TypeSecure:  BackendParameterStoreSecure,
		TypeAllowed: []StorageBackendType{BackendParameterStoreSecure},
	}
	c.NormalizeAllowed()
	assert.Equal(t, []StorageBackendType{BackendParameterStore, BackendParameterStoreSecure}, c.TypeAllowed)

	// Secure vacío se omite; default siempre primero; dedup de extras.
	c = Config{
		TypeDefault: BackendDynamoDB,
		TypeAllowed: []StorageBackendType{BackendParameterStore, BackendDynamoDB, BackendParameterStore},
	}
	c.NormalizeAllowed()
	assert.Equal(t, []StorageBackendType{BackendDynamoDB, BackendParameterStore}, c.TypeAllowed)

	// Sin allowed ni secure → solo el default.
	c = Config{TypeDefault: BackendDynamoDB}
	c.NormalizeAllowed()
	assert.Equal(t, []StorageBackendType{BackendDynamoDB}, c.TypeAllowed)
}

func TestBackendIsValid(t *testing.T) {
	assert.True(t, BackendDynamoDB.IsValid())
	assert.True(t, BackendParameterStore.IsValid())
	assert.True(t, BackendParameterStoreSecure.IsValid())
	assert.False(t, StorageBackendType("garbage").IsValid())
	assert.False(t, StorageBackendType("").IsValid())
}

func TestConfigValidate(t *testing.T) {
	require.NoError(t, (&Config{
		TypeDefault: BackendParameterStore,
		TypeSecure:  BackendParameterStoreSecure,
		TypeAllowed: []StorageBackendType{BackendDynamoDB},
	}).Validate())

	// Secure vacío es válido (opcional).
	require.NoError(t, (&Config{TypeDefault: BackendDynamoDB}).Validate())

	assert.Error(t, (&Config{TypeDefault: "garbage"}).Validate())
	assert.Error(t, (&Config{TypeDefault: BackendDynamoDB, TypeSecure: "nope"}).Validate())
	assert.Error(t, (&Config{TypeDefault: BackendDynamoDB, TypeAllowed: []StorageBackendType{"bad"}}).Validate())
}
