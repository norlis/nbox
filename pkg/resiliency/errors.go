package resiliency

import "fmt"

// PartialBatchError indicates a DynamoDB batch operation completed partially:
// the service processed some items but returned the rest as
// UnprocessedKeys / UnprocessedItems.
type PartialBatchError struct {
	Remaining int
}

func (e *PartialBatchError) Error() string {
	return fmt.Sprintf("partial batch: %d items remaining", e.Remaining)
}
