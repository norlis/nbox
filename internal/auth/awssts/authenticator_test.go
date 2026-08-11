package awssts_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"nbox/internal/auth/awssts"
)

const assumedRoleSTSResponse = `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::123456789012:assumed-role/entrypushd-watcher/i-0abcdef</Arn>
    <Account>123456789012</Account>
    <UserId>AROAEXAMPLEID:i-0abcdef</UserId>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`

func TestAuthenticate_ValidCredentials_ReturnsIdentity(t *testing.T) {
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::123456789012:role/entrypushd-watcher")

	id, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Method != "aws-sts" {
		t.Errorf("Method = %q", id.Method)
	}
	if id.Subject != "arn:aws:iam::123456789012:role/entrypushd-watcher" {
		t.Errorf("Subject = %q", id.Subject)
	}
	if id.Meta["name"] != "prod" {
		t.Errorf("Meta[name] = %q", id.Meta["name"])
	}
	if id.Meta["account_id"] != "123456789012" {
		t.Errorf("account_id = %q", id.Meta["account_id"])
	}
	if !slices.Equal(id.Roles, []string{"entrypushd"}) {
		t.Errorf("Roles = %v", id.Roles)
	}
}

func TestAuthenticate_AssumedRole_ARNNormalizedAndMatched(t *testing.T) {
	fakeSTS := newFakeSTS(t, assumedRoleSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	// Store has the canonical IAM role ARN (no session segment).
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::123456789012:role/entrypushd-watcher")

	id, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Subject != "arn:aws:iam::123456789012:role/entrypushd-watcher" {
		t.Errorf("Subject = %q, want canonical IAM ARN", id.Subject)
	}
}

func TestAuthenticate_MalformedWireJSON_ReturnsErr(t *testing.T) {
	fakeSTS := newFakeSTS(t, "", http.StatusOK)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::1:role/x")

	_, err := a.Authenticate(context.Background(), []byte(`not json`))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAuthenticate_MissingMethodField_ReturnsErrInvalidCredential(t *testing.T) {
	fakeSTS := newFakeSTS(t, "", http.StatusOK)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::1:role/x")

	wire := []byte(`{"iam_request_url":"aGVsbG8=","iam_request_body":"aGVsbG8=","iam_request_headers":"e30="}`)
	_, err := a.Authenticate(context.Background(), wire)
	if !errors.Is(err, awssts.ErrInvalidCredential) {
		t.Fatalf("want ErrInvalidCredential, got %v", err)
	}
}

func TestAuthenticate_MissingURLField_ReturnsErrInvalidCredential(t *testing.T) {
	fakeSTS := newFakeSTS(t, "", http.StatusOK)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::1:role/x")

	wire := []byte(`{"iam_http_request_method":"POST","iam_request_body":"aGVsbG8=","iam_request_headers":"e30="}`)
	_, err := a.Authenticate(context.Background(), wire)
	if !errors.Is(err, awssts.ErrInvalidCredential) {
		t.Fatalf("want ErrInvalidCredential, got %v", err)
	}
}

func TestAuthenticate_STSReturns4xx_ReturnsErrSTSRejected(t *testing.T) {
	fakeSTS := newFakeSTS(t, "<ErrorResponse><Error><Code>SignatureDoesNotMatch</Code></Error></ErrorResponse>", http.StatusForbidden)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::1:role/x")

	_, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrSTSRejected) {
		t.Fatalf("want ErrSTSRejected, got %v", err)
	}
}

func TestAuthenticate_STSReturns5xx_ReturnsErrSTSUnavailable(t *testing.T) {
	fakeSTS := newFakeSTS(t, "internal server error", http.StatusInternalServerError)
	defer fakeSTS.Close()
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::1:role/x")

	_, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrSTSUnavailable) {
		t.Fatalf("want ErrSTSUnavailable, got %v", err)
	}
}

func TestAuthenticate_UntrustedHost_ReturnsErrUntrustedHost(t *testing.T) {
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	// Authenticator's trustedHosts list does NOT include the fakeSTS host.
	a := awssts.NewWithTrustedHosts(
		mustStoreWithARN(t, "arn:aws:iam::1:role/x"),
		&http.Client{},
		[]string{"sts.amazonaws.com"}, // only the real AWS STS, NOT 127.0.0.1
	)

	_, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrUntrustedHost) {
		t.Fatalf("want ErrUntrustedHost, got %v", err)
	}
}

func TestAuthenticate_UnknownARN_ReturnsErrUnknownARN(t *testing.T) {
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	// Store has a DIFFERENT ARN than what STS returns.
	a := newAuthForFakeSTS(t, fakeSTS, "arn:aws:iam::999999999999:role/different")

	_, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrUnknownARN) {
		t.Fatalf("want ErrUnknownARN, got %v", err)
	}
}

func TestAuthenticate_DisabledARN_ReturnsErrARNDisabled(t *testing.T) {
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	raw := `[{"arn":"arn:aws:iam::123456789012:role/entrypushd-watcher","name":"x","roles":[],"status":"disabled"}]`
	store, err := awssts.NewInMemory([]byte(raw))
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	a := awssts.NewWithTrustedHosts(store, &http.Client{}, []string{hostWithoutPort(fakeSTS.URL)})

	_, err = a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrARNDisabled) {
		t.Fatalf("want ErrARNDisabled, got %v", err)
	}
}

func TestAuthenticate_AttributesFlowToMeta(t *testing.T) {
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	raw := `[{
		"arn":"arn:aws:iam::123456789012:role/entrypushd-watcher",
		"name":"prod-watcher",
		"roles":["entrypushd"],
		"attributes":{"env":"production","team":"vault"},
		"status":"active"
	}]`
	store, _ := awssts.NewInMemory([]byte(raw))
	a := awssts.NewWithTrustedHosts(store, &http.Client{}, []string{hostWithoutPort(fakeSTS.URL)})

	id, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Meta["env"] != "production" {
		t.Errorf("Meta[env] = %q", id.Meta["env"])
	}
	if id.Meta["team"] != "vault" {
		t.Errorf("Meta[team] = %q", id.Meta["team"])
	}
	if id.Meta["account_id"] != "123456789012" {
		t.Errorf("account_id = %q", id.Meta["account_id"])
	}
	if id.Meta["user_id"] != "AROAEXAMPLEID" {
		t.Errorf("user_id = %q", id.Meta["user_id"])
	}
}

func TestAuthenticate_StoreReturnsErrInChain_PropagatesAsErrUnknownARN(t *testing.T) {
	// Sanity: any error from the store path collapses to ErrUnknownARN
	// (the only Store error possible from InMemory is ErrUnknownARN).
	fakeSTS := newFakeSTS(t, validSTSResponse, http.StatusOK)
	defer fakeSTS.Close()
	emptyStore, _ := awssts.NewInMemory(nil)
	a := awssts.NewWithTrustedHosts(emptyStore, &http.Client{}, []string{hostWithoutPort(fakeSTS.URL)})

	_, err := a.Authenticate(context.Background(), buildWireCredentialBytes(t, fakeSTS.URL, nil))
	if !errors.Is(err, awssts.ErrUnknownARN) {
		t.Fatalf("want ErrUnknownARN, got %v", err)
	}
}
