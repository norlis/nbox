package domain

import "time"

type EventType string

const (
	EventEntryActions             EventType = "entry.upsert"
	EventEntryDeleted             EventType = "entry.deleted"
	EventEntryRetrieveSecretValue EventType = "entry.retrieve.secret"
	EventTemplateCreated          EventType = "template.created"
	EventTemplateUpdated          EventType = "template.updated"
)

type Event[T any] struct {
	Type          EventType `json:"type"`
	TransactionId string    `json:"transactionId"`
	Username      string    `json:"username"`
	Timestamp     time.Time `json:"timestamp"`
	Payload       T         `json:"payload"`
}
