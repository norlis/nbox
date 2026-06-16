package approle

import "encoding/json"

// BuildCredential is a helper for clients (agents) constructing the body
// they send via gRPC metadata. Returns the JSON bytes ready to base64-encode
// and attach to the "authorization" metadata as "AppRole <base64>".
//
// Equivalent to building the JSON by hand; provided for ergonomics and
// to keep the wire format defined in one place.
func BuildCredential(roleID, secretID string) ([]byte, error) {
	return json.Marshal(credential{RoleID: roleID, SecretID: secretID})
}
