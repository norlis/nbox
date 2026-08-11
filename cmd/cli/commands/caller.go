package commands

import (
	"context"

	eventdrivenaws "github.com/norlis/event-driven/pkg/transport/aws"
)

// fallbackBy is recorded in audit fields when the caller ARN cannot be resolved.
const fallbackBy = "nbox-cli"

// callerBy returns the caller's ARN for audit fields (updated_by), so writes
// are attributed to the real AWS principal instead of a generic tool name.
// Falls back to fallbackBy on error so an STS hiccup never blocks a write.
func callerBy(ctx context.Context, id *eventdrivenaws.Identity) string {
	arn, err := id.Arn(ctx)
	if err != nil || arn == "" {
		info("warning: could not resolve caller identity; recording %q\n", fallbackBy)
		return fallbackBy
	}
	return arn
}
