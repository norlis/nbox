package grpc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"nbox/internal/auth"
	"nbox/internal/auth/approle"
	"nbox/internal/auth/awssts"
	grpcint "nbox/internal/entrypushd/grpc"
)

const testHmacKey = "test-secret-key-32-bytes-long!!!"

// fakeAuthenticator returns a fixed Identity (success) or error (fail).
type fakeAuthenticator struct {
	identity *auth.Identity
	err      error
}

func (f *fakeAuthenticator) Method() string { return "approle" }
func (f *fakeAuthenticator) Authenticate(_ context.Context, _ []byte) (*auth.Identity, error) {
	return f.identity, f.err
}

// captureSourceIPAuthenticator records the source IP from ctx.
type captureSourceIPAuthenticator struct {
	identity *auth.Identity
	captured *string
}

func (c *captureSourceIPAuthenticator) Method() string { return "approle" }
func (c *captureSourceIPAuthenticator) Authenticate(ctx context.Context, _ []byte) (*auth.Identity, error) {
	if ip, ok := auth.SourceIPFromContext(ctx); ok {
		*c.captured = ip
	}
	return c.identity, nil
}

// fakeStream implements just enough googlegrpc.ServerStream for tests.
type fakeStream struct {
	googlegrpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

// buildCredentialMD returns metadata with authorization: <scheme> <b64(JSON)>.
func buildCredentialMD(scheme, roleID, secretID string) metadata.MD {
	body, _ := json.Marshal(map[string]string{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	encoded := base64.StdEncoding.EncodeToString(body)
	return metadata.New(map[string]string{"authorization": scheme + " " + encoded})
}

func newInterceptor(t *testing.T, a auth.Authenticator) *grpcint.AuthInterceptor {
	t.Helper()
	reg := auth.NewRegistry(a)
	logger := slog.New(slog.DiscardHandler)
	return grpcint.NewAuthInterceptor(reg, grpcint.HMACKey([]byte(testHmacKey)), grpcint.AppRoleDisabled(false), logger)
}

func TestInterceptor_Stream_ValidCredentials_InjectsIdentityAndToken(t *testing.T) {
	identity := &auth.Identity{
		Method:  "approle",
		Subject: "role-uuid",
		Roles:   []string{"entrypushd"},
		Meta:    map[string]string{"name": "watcher-agent"},
	}
	i := newInterceptor(t, &fakeAuthenticator{identity: identity})

	md := buildCredentialMD("AppRole", "role-uuid", "secret-id")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	var seenCtx context.Context
	handler := func(_ any, ss googlegrpc.ServerStream) error {
		seenCtx = ss.Context()
		return nil
	}
	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{FullMethod: "/stream.v1.KVStream/Watch"}, handler)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	gotID, ok := auth.IdentityFromContext(seenCtx)
	if !ok {
		t.Fatal("Identity not injected into ctx")
	}
	if gotID.Subject != "role-uuid" {
		t.Errorf("Subject = %q", gotID.Subject)
	}
	if gotID.Method != "approle" {
		t.Errorf("Method = %q, want approle", gotID.Method)
	}
	if len(gotID.Roles) != 1 || gotID.Roles[0] != "entrypushd" {
		t.Errorf("Roles = %v, want [entrypushd]", gotID.Roles)
	}

	token, ok := auth.M2MTokenFromContext(seenCtx)
	if !ok || token == "" {
		t.Error("M2M token not injected into ctx")
	}
}

func TestInterceptor_Stream_NoMetadata_ReturnsUnauthenticated(t *testing.T) {
	i := newInterceptor(t, &fakeAuthenticator{identity: &auth.Identity{}})
	stream := &fakeStream{ctx: context.Background()}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s, want Unauthenticated", status.Code(err))
	}
}

func TestInterceptor_Stream_NoAuthorizationKey_ReturnsUnauthenticated(t *testing.T) {
	i := newInterceptor(t, &fakeAuthenticator{identity: &auth.Identity{}})
	md := metadata.New(map[string]string{"x-other-key": "value"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s", status.Code(err))
	}
}

func TestInterceptor_Stream_MalformedSchemeBody_ReturnsUnauthenticated(t *testing.T) {
	i := newInterceptor(t, &fakeAuthenticator{identity: &auth.Identity{}})
	md := metadata.New(map[string]string{"authorization": "MissingSpaceAndBody"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s", status.Code(err))
	}
}

func TestInterceptor_Stream_InvalidBase64_ReturnsUnauthenticated(t *testing.T) {
	i := newInterceptor(t, &fakeAuthenticator{identity: &auth.Identity{}})
	md := metadata.New(map[string]string{"authorization": "AppRole not-base64!!!"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s", status.Code(err))
	}
}

func TestInterceptor_Stream_UnknownScheme_ReturnsUnauthenticated(t *testing.T) {
	i := newInterceptor(t, &fakeAuthenticator{identity: &auth.Identity{}})
	md := buildCredentialMD("Bearer", "r", "s")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s", status.Code(err))
	}
}

func TestInterceptor_Stream_InvalidSecret_ReturnsUnauthenticated(t *testing.T) {
	// approle.ErrInvalidSecret → collapsed to Unauthenticated (no leak).
	i := newInterceptor(t, &fakeAuthenticator{err: approle.ErrInvalidSecret})
	md := buildCredentialMD("AppRole", "r", "wrong")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s, want Unauthenticated (no leak)", status.Code(err))
	}
}

func TestInterceptor_Stream_KillSwitchActive_ReturnsUnavailable(t *testing.T) {
	// Pass AppRoleDisabled(true) directly — no env var race.
	// Authenticator should NEVER be called when kill-switch is on.
	reg := auth.NewRegistry(&fakeAuthenticator{identity: &auth.Identity{}})
	logger := slog.New(slog.DiscardHandler)
	i := grpcint.NewAuthInterceptor(reg, grpcint.HMACKey([]byte(testHmacKey)), grpcint.AppRoleDisabled(true), logger)

	md := buildCredentialMD("AppRole", "r", "s")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %s, want Unavailable", status.Code(err))
	}
}

func TestInterceptor_Stream_PropagatesSourceIPFromPeer(t *testing.T) {
	captured := ""
	a := &captureSourceIPAuthenticator{
		identity: &auth.Identity{Subject: "role-uuid"},
		captured: &captured,
	}
	i := newInterceptor(t, a)

	md := buildCredentialMD("AppRole", "r", "s")
	addr := &net.TCPAddr{IP: net.ParseIP("198.51.100.42"), Port: 4242}
	ctx := peer.NewContext(metadata.NewIncomingContext(context.Background(), md), &peer.Peer{Addr: addr})
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if captured != "198.51.100.42" {
		t.Errorf("captured source IP = %q, want 198.51.100.42", captured)
	}
}

func TestInterceptor_Stream_InvalidCredential_ReturnsInvalidArgument(t *testing.T) {
	// ErrInvalidCredential → InvalidArgument (not Unauthenticated)
	i := newInterceptor(t, &fakeAuthenticator{err: approle.ErrInvalidCredential})
	md := buildCredentialMD("AppRole", "r", "s")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %s, want InvalidArgument for ErrInvalidCredential", status.Code(err))
	}
}

func TestInterceptor_Unary_ValidCredentials_InjectsIdentityAndToken(t *testing.T) {
	identity := &auth.Identity{Method: "approle", Subject: "role-uuid", Roles: []string{"entrypushd"}}
	i := newInterceptor(t, &fakeAuthenticator{identity: identity})

	md := buildCredentialMD("AppRole", "role-uuid", "s")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var seenCtx context.Context
	handler := func(c context.Context, _ any) (any, error) {
		seenCtx = c
		return "ok", nil
	}
	resp, err := i.Unary(ctx, nil, &googlegrpc.UnaryServerInfo{FullMethod: "/x.y/Z"}, handler)
	if err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v", resp)
	}
	if _, ok := auth.IdentityFromContext(seenCtx); !ok {
		t.Error("Identity not injected")
	}
	if _, ok := auth.M2MTokenFromContext(seenCtx); !ok {
		t.Error("M2M token not injected")
	}
}

// --- AWS STS integration tests (Fase 2.1.b) ---

func TestInterceptor_AWSSTSScheme_ValidCredentials_InjectsIdentityAndToken(t *testing.T) {
	fakeSTS := newFakeSTSForInterceptor(t, validSTSXML, http.StatusOK)
	defer fakeSTS.Close()

	awsstsAuth := awssts.NewWithTrustedHosts(
		mustAWSSTSStore(t, "arn:aws:iam::123456789012:role/entrypushd-watcher"),
		&http.Client{Timeout: 5 * time.Second},
		[]string{stripPort(fakeSTS.URL)},
	)
	reg := auth.NewRegistry(awsstsAuth)
	logger := slog.New(slog.DiscardHandler)
	i := grpcint.NewAuthInterceptor(reg, grpcint.HMACKey([]byte(testHmacKey)), grpcint.AppRoleDisabled(false), logger)

	md := buildAWSSTSMetadata(t, fakeSTS.URL)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	var seenCtx context.Context
	handler := func(_ any, ss googlegrpc.ServerStream) error {
		seenCtx = ss.Context()
		return nil
	}
	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{FullMethod: "/stream.v1.KVStream/Watch"}, handler)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	gotID, ok := auth.IdentityFromContext(seenCtx)
	if !ok {
		t.Fatal("Identity not injected")
	}
	if gotID.Method != "aws-sts" {
		t.Errorf("Method = %q", gotID.Method)
	}
	if _, ok := auth.M2MTokenFromContext(seenCtx); !ok {
		t.Error("M2M token not injected")
	}
}

func TestInterceptor_AWSSTSScheme_STSUnavailable_ReturnsUnavailable(t *testing.T) {
	fakeSTS := newFakeSTSForInterceptor(t, "down", http.StatusInternalServerError)
	defer fakeSTS.Close()

	awsstsAuth := awssts.NewWithTrustedHosts(
		mustAWSSTSStore(t, "arn:aws:iam::1:role/x"),
		&http.Client{Timeout: 5 * time.Second},
		[]string{stripPort(fakeSTS.URL)},
	)
	reg := auth.NewRegistry(awsstsAuth)
	i := grpcint.NewAuthInterceptor(reg, grpcint.HMACKey([]byte(testHmacKey)), grpcint.AppRoleDisabled(false), slog.New(slog.DiscardHandler))

	md := buildAWSSTSMetadata(t, fakeSTS.URL)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %s, want Unavailable", status.Code(err))
	}
}

func TestInterceptor_AWSSTSScheme_UnknownARN_ReturnsUnauthenticated(t *testing.T) {
	fakeSTS := newFakeSTSForInterceptor(t, validSTSXML, http.StatusOK)
	defer fakeSTS.Close()

	// Store has a different ARN.
	awsstsAuth := awssts.NewWithTrustedHosts(
		mustAWSSTSStore(t, "arn:aws:iam::999:role/different"),
		&http.Client{Timeout: 5 * time.Second},
		[]string{stripPort(fakeSTS.URL)},
	)
	reg := auth.NewRegistry(awsstsAuth)
	i := grpcint.NewAuthInterceptor(reg, grpcint.HMACKey([]byte(testHmacKey)), grpcint.AppRoleDisabled(false), slog.New(slog.DiscardHandler))

	md := buildAWSSTSMetadata(t, fakeSTS.URL)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	stream := &fakeStream{ctx: ctx}

	err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %s, want Unauthenticated", status.Code(err))
	}
}

// --- helpers for AWS STS interceptor tests ---

const validSTSXML = `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:role/entrypushd-watcher</Arn>
    <Account>123456789012</Account>
    <UserId>AROAEXAMPLEID</UserId>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`

func newFakeSTSForInterceptor(t *testing.T, body string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(statusCode)
		_, _ = fmt.Fprint(w, body)
	}))
}

func stripPort(serverURL string) string {
	h := strings.TrimPrefix(serverURL, "http://")
	if idx := strings.LastIndexByte(h, ':'); idx > 0 {
		h = h[:idx]
	}
	return h
}

func hostPortOf(serverURL string) string {
	return strings.TrimPrefix(serverURL, "http://")
}

func mustAWSSTSStore(t *testing.T, arn string) *awssts.InMemory {
	t.Helper()
	raw := fmt.Sprintf(`[{"arn":%q,"name":"prod","roles":["entrypushd"],"status":"active"}]`, arn)
	s, err := awssts.NewInMemory([]byte(raw))
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	return s
}

func buildAWSSTSMetadata(t *testing.T, targetURL string) metadata.MD {
	t.Helper()
	headers := map[string]string{
		"Authorization": "AWS4-HMAC-SHA256 sig",
		"X-Amz-Date":    "20260522T130000Z",
		"Host":          hostPortOf(targetURL),
	}
	headersJSON, _ := json.Marshal(headers)
	wire, _ := json.Marshal(map[string]string{
		"iam_http_request_method": "POST",
		"iam_request_url":         base64.StdEncoding.EncodeToString([]byte(targetURL + "/")),
		"iam_request_body":        base64.StdEncoding.EncodeToString([]byte("Action=GetCallerIdentity&Version=2011-06-15")),
		"iam_request_headers":     base64.StdEncoding.EncodeToString(headersJSON),
	})
	credB64 := base64.StdEncoding.EncodeToString(wire)
	return metadata.New(map[string]string{"authorization": "AWS-STS " + credB64})
}

// TestAuthErrorRules_SentinelToCodeAndKind exercises every row in
// authErrorRules to confirm the table drives both classifyAuthError and
// errKind correctly, and verifies the unknown-error fallbacks.
func TestAuthErrorRules_SentinelToCodeAndKind(t *testing.T) {
	rows := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantKind string
	}{
		// approle sentinels
		{"approle/ErrInvalidCredential", approle.ErrInvalidCredential, codes.InvalidArgument, "invalid_credential"},
		{"approle/ErrRoleNotFound", approle.ErrRoleNotFound, codes.Unauthenticated, "role_not_found"},
		{"approle/ErrRoleDisabled", approle.ErrRoleDisabled, codes.Unauthenticated, "role_disabled"},
		{"approle/ErrInvalidSecret", approle.ErrInvalidSecret, codes.Unauthenticated, "invalid_secret"},
		{"approle/ErrSourceNotAllowed", approle.ErrSourceNotAllowed, codes.Unauthenticated, "source_not_allowed"},
		// awssts sentinels
		{"awssts/ErrInvalidCredential", awssts.ErrInvalidCredential, codes.InvalidArgument, "invalid_credential"},
		{"awssts/ErrDecodeFailed", awssts.ErrDecodeFailed, codes.InvalidArgument, "decode_failed"},
		{"awssts/ErrUntrustedHost", awssts.ErrUntrustedHost, codes.Unauthenticated, "untrusted_host"},
		{"awssts/ErrSTSRejected", awssts.ErrSTSRejected, codes.Unauthenticated, "sts_rejected"},
		{"awssts/ErrUnknownARN", awssts.ErrUnknownARN, codes.Unauthenticated, "unknown_arn"},
		{"awssts/ErrARNDisabled", awssts.ErrARNDisabled, codes.Unauthenticated, "arn_disabled"},
		{"awssts/ErrSTSUnavailable", awssts.ErrSTSUnavailable, codes.Unavailable, "sts_unavailable"},
		// fallback / unknown error
		{"unknown error", errors.New("some unexpected error"), codes.Internal, "internal"},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			i := newInterceptor(t, &fakeAuthenticator{err: tc.err})
			md := buildCredentialMD("AppRole", "r", "s")
			ctx := metadata.NewIncomingContext(context.Background(), md)
			stream := &fakeStream{ctx: ctx}

			err := i.Stream(nil, stream, &googlegrpc.StreamServerInfo{}, func(any, googlegrpc.ServerStream) error { return nil })
			if status.Code(err) != tc.wantCode {
				t.Errorf("code = %s, want %s", status.Code(err), tc.wantCode)
			}
		})
	}
}
