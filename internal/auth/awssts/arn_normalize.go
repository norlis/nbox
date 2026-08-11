package awssts

import "strings"

// NormalizeARN converts the ARN forms STS returns into the canonical IAM
// role ARN the admin configures in NBOX_AWS_ARN_MAP.
//
// STS variants:
//
//	arn:aws:sts::123:assumed-role/foo/session-name   → arn:aws:iam::123:role/foo
//	arn:aws:iam::123:role/foo/i-0abc                 → arn:aws:iam::123:role/foo
//	arn:aws:iam::123:role/foo                        → unchanged
//
// Output is always arn:aws:iam::<account>:role/<role-name>. Malformed
// input is returned as-is (let the Store lookup return ErrUnknownARN).
func NormalizeARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 {
		return arn
	}
	// parts[0]="arn" [1]="aws" [2]="sts|iam" [3]="" [4]=account [5]=resource

	switch {
	case parts[2] == "sts" && strings.HasPrefix(parts[5], "assumed-role/"):
		roleAndSession := strings.TrimPrefix(parts[5], "assumed-role/")
		if slash := strings.IndexByte(roleAndSession, '/'); slash > 0 {
			parts[2] = "iam"
			parts[5] = "role/" + roleAndSession[:slash]
		}
	case parts[2] == "iam" && strings.HasPrefix(parts[5], "role/"):
		roleAndSession := strings.TrimPrefix(parts[5], "role/")
		if slash := strings.IndexByte(roleAndSession, '/'); slash > 0 {
			parts[5] = "role/" + roleAndSession[:slash]
		}
	}
	return strings.Join(parts, ":")
}
