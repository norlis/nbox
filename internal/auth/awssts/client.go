package awssts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// BuildCredential generates a wire credential by presigning a
// GetCallerIdentity request with the agent's IAM credentials and
// serializing it to the Vault-style 4-field format wrapped in base64.
//
// Usage by the agent:
//
//	cfg, _ := config.LoadDefaultConfig(ctx)
//	cred, err := awssts.BuildCredential(ctx, cfg)
//	// cred is "AWS-STS <base64>" ready to put in gRPC metadata:
//	//   md.Set("authorization", cred)
//
// The presigned URL is valid for 15 minutes (X-Amz-Date header sets
// the validity window). Agents typically rebuild the credential on
// every reconnection — no need to cache.
func BuildCredential(ctx context.Context, awsCfg aws.Config) (string, error) {
	presigner := sts.NewPresignClient(sts.NewFromConfig(awsCfg))
	presigned, err := presigner.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("awssts: presign: %w", err)
	}

	headersJSON, err := json.Marshal(presigned.SignedHeader)
	if err != nil {
		return "", fmt.Errorf("awssts: marshal headers: %w", err)
	}

	const body = "Action=GetCallerIdentity&Version=2011-06-15"
	wire, err := json.Marshal(WireCredential{
		Method:     presigned.Method,
		URLB64:     base64.StdEncoding.EncodeToString([]byte(presigned.URL)),
		BodyB64:    base64.StdEncoding.EncodeToString([]byte(body)),
		HeadersB64: base64.StdEncoding.EncodeToString(headersJSON),
	})
	if err != nil {
		return "", fmt.Errorf("awssts: marshal wire: %w", err)
	}
	return "AWS-STS " + base64.StdEncoding.EncodeToString(wire), nil
}
