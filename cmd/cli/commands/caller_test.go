package commands

import (
	"context"
	"errors"
	"testing"

	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
	eventdrivenaws "github.com/norlis/event-driven/pkg/transport/aws"
	"github.com/stretchr/testify/assert"
)

type fakeSTS struct {
	arn *string
	err error
}

func (f *fakeSTS) GetCallerIdentity(context.Context, *awssts.GetCallerIdentityInput, ...func(*awssts.Options)) (*awssts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &awssts.GetCallerIdentityOutput{Arn: f.arn}, nil
}

func TestCallerBy(t *testing.T) {
	arn := "arn:aws:sts::123456789012:assumed-role/AdminSSO/nviamonte"

	// Happy path: the caller ARN is recorded verbatim.
	id := &eventdrivenaws.Identity{Client: &fakeSTS{arn: &arn}}
	assert.Equal(t, arn, callerBy(context.Background(), id))

	// STS error ⇒ fallback, never blocks the write.
	id = &eventdrivenaws.Identity{Client: &fakeSTS{err: errors.New("boom")}}
	assert.Equal(t, fallbackBy, callerBy(context.Background(), id))

	// Defensive: empty ARN in a successful response ⇒ fallback.
	id = &eventdrivenaws.Identity{Client: &fakeSTS{}}
	assert.Equal(t, fallbackBy, callerBy(context.Background(), id))
}
