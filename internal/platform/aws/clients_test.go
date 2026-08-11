package aws_test

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	platformaws "nbox/internal/platform/aws"
)

func TestNewIdentity_WiresConfigCorrectly(t *testing.T) {
	cfg := &awssdk.Config{Region: "eu-west-2"}
	id := platformaws.NewIdentity(cfg)

	require.NotNil(t, id, "NewIdentity must return a non-nil *aws.Identity")
	require.NotNil(t, id.Config, "Identity.Config must be set")
	require.Equal(t, "eu-west-2", id.Config.Region, "Region must propagate from awssdk.Config")
}
