package awssts

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

	"nbox/internal/auth"
)

// Authenticator implements auth.Authenticator for the AWS STS method.
type Authenticator struct {
	store        Store
	httpClient   *http.Client
	trustedHosts []string // AWS STS hosts allowed for forwarding (anti-SSRF)
}

// New builds an Authenticator with the default AWS STS host whitelist.
// httpClient SHOULD have a 10s timeout — STS roundtrip is in the
// critical path of every agent connect.
func New(store Store, httpClient *http.Client) *Authenticator {
	return &Authenticator{
		store:        store,
		httpClient:   httpClient,
		trustedHosts: DefaultTrustedSTSHosts(),
	}
}

// NewWithTrustedHosts is a test seam — lets tests point at httptest.Server.
// Production code uses New.
func NewWithTrustedHosts(store Store, httpClient *http.Client, hosts []string) *Authenticator {
	return &Authenticator{store: store, httpClient: httpClient, trustedHosts: hosts}
}

func (a *Authenticator) Method() string { return MethodName }

// Authenticate forwards the agent's presigned GetCallerIdentity to AWS
// STS, parses the response, and matches the returned ARN against the
// Store. Returns Identity on success or one of the sentinel errors:
// ErrInvalidCredential, ErrDecodeFailed, ErrUntrustedHost, ErrSTSRejected,
// ErrSTSUnavailable, ErrUnknownARN, ErrARNDisabled.
func (a *Authenticator) Authenticate(ctx context.Context, raw []byte) (*auth.Identity, error) {
	var wire WireCredential
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("awssts: invalid credential format: %w", err)
	}
	if wire.Method == "" || wire.URLB64 == "" || wire.BodyB64 == "" || wire.HeadersB64 == "" {
		return nil, ErrInvalidCredential
	}

	caller, err := callSTS(ctx, a.httpClient, a.trustedHosts, &wire)
	if err != nil {
		return nil, err
	}

	normalized := NormalizeARN(caller.ARN)
	mapping, err := a.store.LookupByARN(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if mapping.Status != StatusActive {
		return nil, ErrARNDisabled
	}

	meta := make(map[string]string, len(mapping.Attributes)+3)
	maps.Copy(meta, mapping.Attributes)
	meta["name"] = mapping.Name
	meta["account_id"] = caller.Account
	meta["user_id"] = caller.UserID

	return &auth.Identity{
		Method:  MethodName,
		Subject: normalized,
		Roles:   mapping.Roles,
		Meta:    meta,
	}, nil
}
