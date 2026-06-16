package awssts

import "errors"

var (
	// ErrInvalidCredential — required fields missing in wire JSON.
	ErrInvalidCredential = errors.New("awssts: missing required wire fields")

	// ErrDecodeFailed — base64 or JSON decode failed for one of the
	// inner fields. Indicates a client bug.
	ErrDecodeFailed = errors.New("awssts: failed to decode wire credential")

	// ErrUntrustedHost — the URL in the wire credential points at a host
	// outside the trusted STS whitelist. SSRF defense.
	ErrUntrustedHost = errors.New("awssts: presigned URL host not in trusted STS whitelist")

	// ErrSTSRejected — STS returned 4xx. Signature expired, invalid, or
	// request malformed. Client problem.
	ErrSTSRejected = errors.New("awssts: STS rejected the signed request")

	// ErrSTSUnavailable — STS returned 5xx, timed out, or network error.
	// Transient; agent should retry.
	ErrSTSUnavailable = errors.New("awssts: STS unavailable")

	// ErrUnknownARN — STS validated the request, but the returned ARN
	// isn't in NBOX_AWS_ARN_MAP.
	ErrUnknownARN = errors.New("awssts: ARN not in mapping")

	// ErrARNDisabled — ARN found in mapping but status="disabled".
	ErrARNDisabled = errors.New("awssts: ARN disabled")
)
