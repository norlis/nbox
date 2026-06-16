package awssts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxSTSResponseBytes caps the GetCallerIdentity response body read to 64 KiB.
// A legitimate AWS STS XML response is ~500 bytes; 64 KiB is a generous ceiling
// that prevents unbounded memory growth from a malicious or misbehaving endpoint.
const maxSTSResponseBytes = 64 << 10

// callSTS reconstructs the agent's presigned request from the wire
// credential, validates the host is a trusted AWS STS endpoint, forwards
// it via httpClient, and parses the XML response.
//
// Anti-SSRF: validates the URL host against trustedHosts before any
// network call. Without this, an attacker could presign a request to
// an attacker-controlled domain that responds with a fake "you are
// <victim-arn>" XML.
func callSTS(
	ctx context.Context,
	httpClient *http.Client,
	trustedHosts []string,
	wire *WireCredential,
) (*CallerIdentity, error) {
	rawURL, body, headers, err := decodeWire(wire)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid url", ErrDecodeFailed)
	}
	if !IsTrustedHost(parsed.Host, trustedHosts) {
		return nil, ErrUntrustedHost
	}

	req, err := http.NewRequestWithContext(ctx, wire.Method, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("awssts: build request: %w", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSTSUnavailable, err)
	}
	defer resp.Body.Close()

	// Error responses are bounded by the same LimitReader, so reading them to
	// surface the STS reason (e.g. InvalidClientTokenId, SignatureDoesNotMatch)
	// is safe. The reason is the operator's main debugging signal.
	switch {
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: status %d: %s", ErrSTSUnavailable, resp.StatusCode, stsErrorReason(resp.Body))
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("%w: status %d: %s", ErrSTSRejected, resp.StatusCode, stsErrorReason(resp.Body))
	}

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSTSResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", ErrSTSUnavailable, err)
	}
	ci, perr := parseSTSResponse(rawBody)
	if perr != nil {
		// 2xx but unparseable: surface content headers + a bounded body
		// snippet so the operator can see what STS actually returned.
		return nil, fmt.Errorf("%w (content-type=%q content-encoding=%q body=%.300q)",
			perr, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"), rawBody)
	}
	return ci, nil
}

// decodeWire base64-decodes the 3 inner fields. Supports two header
// shapes: map[string][]string (AWS SDK Go v2) and map[string]string
// (Vault libraries).
func decodeWire(wire *WireCredential) (rawURL, body string, headers http.Header, err error) {
	urlBytes, e1 := base64.StdEncoding.DecodeString(wire.URLB64)
	bodyBytes, e2 := base64.StdEncoding.DecodeString(wire.BodyB64)
	headersBytes, e3 := base64.StdEncoding.DecodeString(wire.HeadersB64)
	if e1 != nil || e2 != nil || e3 != nil {
		return "", "", nil, fmt.Errorf("%w: base64", ErrDecodeFailed)
	}

	headers = make(http.Header)
	var multi map[string][]string
	if jsonErr := json.Unmarshal(headersBytes, &multi); jsonErr == nil {
		maps.Copy(headers, multi)
		return string(urlBytes), string(bodyBytes), headers, nil
	}
	var single map[string]string
	if jsonErr := json.Unmarshal(headersBytes, &single); jsonErr != nil {
		return "", "", nil, fmt.Errorf("%w: headers json", ErrDecodeFailed)
	}
	for k, v := range single {
		headers.Set(k, v)
	}
	return string(urlBytes), string(bodyBytes), headers, nil
}

// IsTrustedHost returns true if host matches any entry in trusted.
// Entries support "*." prefix for subdomain wildcards (e.g. "*.amazonaws.com"
// matches "sts.amazonaws.com" but NOT "sts.amazonaws.com.attacker.com").
// Strips port from host before matching (IPv4/IPv6 aware via net.SplitHostPort).
func IsTrustedHost(host string, trusted []string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	for _, t := range trusted {
		if strings.HasPrefix(t, "*.") {
			suffix := t[1:] // ".amazonaws.com"
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		} else if t == host {
			return true
		}
	}
	return false
}

// DefaultTrustedSTSHosts covers all real AWS STS endpoints:
//   - sts.amazonaws.com         (global)
//   - sts.<region>.amazonaws.com (regional)
//   - sts-fips.<region>.amazonaws.com (FIPS regional)
//   - sts.<region>.api.aws       (VPC PrivateLink endpoints)
func DefaultTrustedSTSHosts() []string {
	return []string{
		"sts.amazonaws.com",
		"*.amazonaws.com", // matches any subdomain of amazonaws.com (sts.<region>, sts-fips.<region>, etc.)
		"*.api.aws",       // matches any subdomain of api.aws (sts.<region>.api.aws, etc.)
	}
}

type stsResponseXML struct {
	XMLName xml.Name `xml:"GetCallerIdentityResponse"`
	Result  struct {
		ARN     string `xml:"Arn"`
		Account string `xml:"Account"`
		UserID  string `xml:"UserId"`
	} `xml:"GetCallerIdentityResult"`
}

// stsErrorReason extracts "Code: Message" from an STS error response
// (e.g. "InvalidClientTokenId: The security token ... is invalid"), falling
// back to a trimmed snippet. Body is bounded by maxSTSResponseBytes.
func stsErrorReason(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, maxSTSResponseBytes))
	var e struct {
		Error struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}
	if xml.Unmarshal(raw, &e) == nil && e.Error.Code != "" {
		return e.Error.Code + ": " + e.Error.Message
	}
	return strings.TrimSpace(string(raw))
}

func parseSTSResponse(body []byte) (*CallerIdentity, error) {
	var r stsResponseXML
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("%w: parse xml", ErrSTSRejected)
	}
	if r.Result.ARN == "" {
		return nil, fmt.Errorf("%w: empty arn in response", ErrSTSRejected)
	}
	return &CallerIdentity{
		ARN:     r.Result.ARN,
		Account: r.Result.Account,
		UserID:  r.Result.UserID,
	}, nil
}
