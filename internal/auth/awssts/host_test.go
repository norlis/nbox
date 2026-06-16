package awssts_test

import (
	"testing"

	"nbox/internal/auth/awssts"
)

func TestIsTrustedHost(t *testing.T) {
	defaults := awssts.DefaultTrustedSTSHosts()

	cases := []struct {
		name    string
		host    string
		trusted []string
		want    bool
	}{
		{"global", "sts.amazonaws.com", defaults, true},
		{"regional", "sts.us-east-1.amazonaws.com", defaults, true},
		{"fips", "sts-fips.us-gov-west-1.amazonaws.com", defaults, true},
		{"vpc_endpoint", "sts.us-east-1.api.aws", defaults, true},
		{"strips_port", "sts.amazonaws.com:443", defaults, true},
		{"rejects_attacker_domain", "attacker.com", defaults, false},
		{"rejects_partial_match", "sts.amazonaws.com.attacker.com", defaults, false},
		{"rejects_typosquat_tld", "sts.amazonaws.io", defaults, false},
		{"test_seam_works", "127.0.0.1", []string{"127.0.0.1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := awssts.IsTrustedHost(tc.host, tc.trusted); got != tc.want {
				t.Errorf("IsTrustedHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}
