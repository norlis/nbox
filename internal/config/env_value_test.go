package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvValue(t *testing.T) {
	// ShapeObject (basic_auth): {"<id>": <data>}
	obj := EnvValue(KeyBasicAuth, "admin", []byte(`{"password":"h","roles":["admin"],"status":"active"}`))
	require.JSONEq(t, `{"admin":{"password":"h","roles":["admin"],"status":"active"}}`, string(obj))

	// ShapeArray (app_role / arn_map): [<data>]
	arr := EnvValue(KeyAppRole, "id1", []byte(`{"id":"id1","name":"x"}`))
	require.JSONEq(t, `[{"id":"id1","name":"x"}]`, string(arr))
}
