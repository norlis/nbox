package config

// Record is the DynamoDB shape of one config entity. Attribute names must
// match the table key schema (kind=HASH, id=RANGE) in deployments/template.yaml.
type Record struct {
	Kind      string `dynamodbav:"kind"`
	ID        string `dynamodbav:"id"`
	Data      string `dynamodbav:"data"`       // JSON of one domain entity
	Version   int64  `dynamodbav:"version"`    // bumped on each write
	UpdatedAt string `dynamodbav:"updated_at"` // RFC3339
	UpdatedBy string `dynamodbav:"updated_by"`
}
