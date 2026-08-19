package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/norlis/httpgate/logging"
	"github.com/norlis/httpgate/trace"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"nbox/internal/auth"
	"nbox/internal/auth/approle"
	"nbox/internal/auth/awssts"
	"nbox/internal/logfields"
)

const authMetadataKey = "authorization"

// HMACKey is the HMAC signing key, as a named type so fx can resolve
// it unambiguously (vs other []byte providers).
type HMACKey []byte

// AppRoleDisabled is the kill-switch flag as a named type so fx can
// resolve it unambiguously (same pattern as HMACKey).
type AppRoleDisabled bool

// AuthInterceptor authenticates incoming gRPC calls. Reads
// `authorization: <scheme> <base64(credential)>` metadata, dispatches
// to the right Authenticator via Registry, mints a JWT, and injects
// Identity + token into ctx for downstream use.
type AuthInterceptor struct {
	registry auth.Registry
	hmacKey  []byte
	disabled bool // kill switch from NBOX_APPROLE_DISABLED=true
	logger   *slog.Logger
}

// NewAuthInterceptor constructs an interceptor wired to registry, hmacKey,
// and disabled (AppRoleDisabled named type for unambiguous fx wiring).
// When disabled is true, all auth attempts are rejected with codes.Unavailable
// until the process restarts.
func NewAuthInterceptor(registry auth.Registry, hmacKey HMACKey, disabled AppRoleDisabled, logger *slog.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		registry: registry,
		hmacKey:  []byte(hmacKey),
		disabled: bool(disabled),
		logger:   logger,
	}
}

// Unary intercepts unary RPCs.
func (i *AuthInterceptor) Unary(
	ctx context.Context,
	req any,
	info *googlegrpc.UnaryServerInfo,
	handler googlegrpc.UnaryHandler,
) (any, error) {
	newCtx, err := i.authenticate(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}
	return handler(newCtx, req)
}

// Stream intercepts server-streaming RPCs like KVStream.Watch.
func (i *AuthInterceptor) Stream(
	srv any,
	ss googlegrpc.ServerStream,
	info *googlegrpc.StreamServerInfo,
	handler googlegrpc.StreamHandler,
) error {
	newCtx, err := i.authenticate(ss.Context(), info.FullMethod)
	if err != nil {
		return err
	}
	return handler(srv, &wrappedStream{ServerStream: ss, ctx: newCtx})
}

type wrappedStream struct {
	googlegrpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// authenticate is the shared validate-mint-inject flow.
func (i *AuthInterceptor) authenticate(ctx context.Context, fullMethod string) (context.Context, error) {
	sourceIP := peerIP(ctx)

	if i.disabled {
		i.audit(ctx, "auth attempted while disabled", "", "", sourceIP, fullMethod, nil)
		return nil, status.Error(codes.Unavailable, "approle auth is disabled")
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get(authMetadataKey)
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
	}

	scheme, credentialB64, ok := parseAuthHeader(values[0])
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "malformed authorization metadata (want: <scheme> <base64>)")
	}

	method := schemeToMethod(scheme)
	authenticator, ok := i.registry.Get(method)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unsupported auth scheme: %s", scheme)
	}

	credentialBytes, err := base64.StdEncoding.DecodeString(credentialB64)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "credential is not valid base64")
	}

	authCtx := auth.WithSourceIP(ctx, sourceIP)

	identity, err := authenticator.Authenticate(authCtx, credentialBytes)
	if err != nil {
		roleID := peekRoleID(credentialBytes)
		i.audit(ctx, "auth failed", roleID, scheme, sourceIP, fullMethod, err)
		return nil, classifyAuthError(err)
	}

	i.audit(ctx, "auth success", identity.Subject, scheme, sourceIP, fullMethod, nil)

	token, err := auth.MintM2MJWT(identity, i.hmacKey)
	if err != nil {
		return nil, status.Error(codes.Internal, "token mint failed")
	}

	newCtx := auth.WithIdentity(ctx, identity)
	newCtx = auth.WithM2MToken(newCtx, token)
	newCtx = trace.NewContext(newCtx, traceContextFromMD(md))
	return newCtx, nil
}

// traceContextFromMD resolves the stream's trace: inherit trace_id from a
// valid inbound traceparent (fresh local span), else start a new trace —
// the Watch RPC is an entry point (§2.3).
func traceContextFromMD(md metadata.MD) trace.Context {
	if vals := md.Get(trace.Header); len(vals) > 0 {
		if tc, err := trace.Parse(vals[0]); err == nil {
			return trace.Context{TraceID: tc.TraceID, SpanID: trace.NewSpanID()}
		}
	}
	return trace.New()
}

// parseAuthHeader splits "Scheme <value>" → ("Scheme", "<value>", true).
func parseAuthHeader(h string) (scheme, payload string, ok bool) {
	idx := strings.IndexByte(h, ' ')
	if idx <= 0 || idx == len(h)-1 {
		return "", "", false
	}
	return h[:idx], strings.TrimSpace(h[idx+1:]), true
}

// schemeToMethod: "AppRole" → "approle". Future: "AWS-STS" → "aws-sts".
func schemeToMethod(scheme string) string {
	return strings.ToLower(scheme)
}

// peerIP pulls the remote IP from the gRPC peer info in ctx.
func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	addr := p.Addr.String()
	if idx := strings.LastIndexByte(addr, ':'); idx > 0 {
		return addr[:idx]
	}
	return addr
}

// peekRoleID extracts role_id best-effort for audit logs. Returns ""
// on parse failure; never logs the secret_id.
func peekRoleID(body []byte) string {
	var c struct {
		RoleID string `json:"role_id"`
	}
	_ = json.Unmarshal(body, &c)
	return c.RoleID
}

// authErrorRule maps a single error sentinel to a gRPC status code, the
// message that code produces on the wire, and the short kind string used in
// audit logs.  Both classifyAuthError and errKind iterate this table so the
// mapping is defined in exactly one place.
var authErrorRules = []struct {
	sentinel error
	code     codes.Code
	msg      string
	kind     string
}{
	// approle
	{approle.ErrInvalidCredential, codes.InvalidArgument, "invalid credential format", "invalid_credential"},
	{approle.ErrRoleNotFound, codes.Unauthenticated, "invalid credentials", "role_not_found"},
	{approle.ErrRoleDisabled, codes.Unauthenticated, "invalid credentials", "role_disabled"},
	{approle.ErrInvalidSecret, codes.Unauthenticated, "invalid credentials", "invalid_secret"},
	{approle.ErrSourceNotAllowed, codes.Unauthenticated, "invalid credentials", "source_not_allowed"},
	// awssts (Fase 2.1.b)
	{awssts.ErrInvalidCredential, codes.InvalidArgument, "invalid credential format", "invalid_credential"},
	{awssts.ErrDecodeFailed, codes.InvalidArgument, "invalid credential format", "decode_failed"},
	{awssts.ErrUntrustedHost, codes.Unauthenticated, "invalid credentials", "untrusted_host"},
	{awssts.ErrSTSRejected, codes.Unauthenticated, "invalid credentials", "sts_rejected"},
	{awssts.ErrUnknownARN, codes.Unauthenticated, "invalid credentials", "unknown_arn"},
	{awssts.ErrARNDisabled, codes.Unauthenticated, "invalid credentials", "arn_disabled"},
	{awssts.ErrSTSUnavailable, codes.Unavailable, "authentication backend unavailable", "sts_unavailable"},
}

// classifyAuthError maps auth sentinels to gRPC status codes. All credential
// failures collapse to Unauthenticated (don't leak which check failed).
func classifyAuthError(err error) error {
	for _, rule := range authErrorRules {
		if errors.Is(err, rule.sentinel) {
			return status.Error(rule.code, rule.msg)
		}
	}
	return status.Error(codes.Internal, "authentication unavailable")
}

// errKind returns a short audit-log label for the error.
func errKind(err error) string {
	for _, rule := range authErrorRules {
		if errors.Is(err, rule.sentinel) {
			return rule.kind
		}
	}
	return "internal"
}

// audit emits a structured log entry. NEVER logs secret_id.
func (i *AuthInterceptor) audit(ctx context.Context, msg, roleID, scheme, sourceIP, method string, err error) {
	attrs := []any{
		slog.String(logfields.KeyAppRoleID, roleID),
		slog.String("scheme", scheme),
		slog.String(logfields.KeySourceIP, sourceIP),
		slog.String(logfields.KeyRPCMethod, method),
		slog.Bool("success", err == nil),
	}
	if err != nil {
		// Full error is server-side only (never returned to the client, which
		// still gets the collapsed "invalid credentials"). Auth errors are
		// sentinels + STS reasons — they never contain the secret_id.
		attrs = append(attrs,
			slog.String("error_kind", errKind(err)),
			logging.Err(err),
		)
	}
	i.logger.InfoContext(ctx, msg, attrs...)
}
