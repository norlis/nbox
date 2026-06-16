// Package awssts implements the AWS STS authentication method for nbox
// M2M auth. The agent presents a presigned GetCallerIdentity request;
// entrypushd forwards it to AWS STS and matches the returned ARN against
// the configured NBOX_AWS_ARN_MAP.
//
// Bootstrap (admin):
//
//  1. Create an IAM role in the agent's AWS account (or have one already).
//  2. Add the role ARN to NBOX_AWS_ARN_MAP in entrypushd's deployment.
//  3. Configure the agent workload to assume the IAM role (instance
//     profile, IRSA, ECS task role, etc.) — this is standard AWS work,
//     no nbox-specific setup.
//
// Authentication (agent → entrypushd gRPC):
//
//	metadata:
//	  authorization: AWS-STS <base64(JSON{method, url, body, headers})>
//
// The interceptor in internal/entrypushd/grpc validates the credential
// via awssts.Authenticator and mints the same internal M2M JWT as the
// AppRole flow (aud=[nbox, entrypushd], TTL=15min). The agent never
// sees the JWT.
package awssts

// Status is the lifecycle state of an ARNMapping.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// MethodName matches what (*Authenticator).Method() returns and what the
// registry uses to dispatch requests with the "AWS-STS" scheme.
const MethodName = "aws-sts"

// WireCredential is the on-the-wire shape after base64-decoding the
// metadata body and JSON-unmarshalling the outer envelope. The 3
// base64-encoded inner fields (URLB64, BodyB64, HeadersB64) are decoded
// by callSTS.
type WireCredential struct {
	Method     string `json:"iam_http_request_method"`
	URLB64     string `json:"iam_request_url"`
	BodyB64    string `json:"iam_request_body"`
	HeadersB64 string `json:"iam_request_headers"`
}

// ARNMapping is one entry in the NBOX_AWS_ARN_MAP env var. The ARN is
// the canonical IAM role ARN (e.g. "arn:aws:iam::123456789012:role/foo").
// STS-returned ARNs are normalized to this form before lookup — see
// NormalizeARN.
type ARNMapping struct {
	// ARN is the full IAM role ARN. Exact match against the normalized
	// ARN returned by STS.
	//
	// Example: "arn:aws:iam::123456789012:role/entrypushd-watcher"
	ARN string `json:"arn"`

	// Name is the human-readable identifier. Appears in Identity.Meta
	// and JWT Claims.Name for traceability.
	Name string `json:"name"`

	// Roles are the OPA role names this principal gets when authenticated.
	Roles []string `json:"roles"`

	// Attributes are optional key-value pairs that flow into Identity.Meta
	// and the JWT `attributes` claim. None reserved by awssts itself; the
	// JWT minter strips only "name".
	Attributes map[string]string `json:"attributes,omitempty"`

	// Status is "active" (default) or "disabled".
	Status Status `json:"status"`
}

// CallerIdentity is the parsed result of STS GetCallerIdentity, populated
// by parseSTSResponse and consumed by the Authenticator.
type CallerIdentity struct {
	// ARN is the raw ARN as STS returned it (pre-normalization).
	ARN string

	// Account is the 12-digit AWS account number.
	Account string

	// UserID is the STS user identifier (e.g. "AROAID:i-0abc" for assumed roles).
	UserID string
}
