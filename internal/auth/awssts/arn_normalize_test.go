package awssts_test

import (
	"testing"

	"nbox/internal/auth/awssts"
)

func TestNormalizeARN(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{
			"canonical_iam_role",
			"arn:aws:iam::123456789012:role/foo",
			"arn:aws:iam::123456789012:role/foo",
		},
		{
			"assumed_role_with_session",
			"arn:aws:sts::123456789012:assumed-role/foo/session-name",
			"arn:aws:iam::123456789012:role/foo",
		},
		{
			"iam_role_with_trailing_session",
			"arn:aws:iam::123456789012:role/foo/i-0abcdef",
			"arn:aws:iam::123456789012:role/foo",
		},
		{
			"malformed_returned_as_is",
			"not-an-arn",
			"not-an-arn",
		},
		{
			"too_few_parts",
			"arn:aws:iam::123:role",
			"arn:aws:iam::123:role",
		},
		{
			"empty_string",
			"",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := awssts.NormalizeARN(tc.input); got != tc.want {
				t.Errorf("NormalizeARN(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
