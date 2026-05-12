package event

import (
	"context"
	"encoding/json"
	"time"
)

// Type identifica un tipo de evento del sistema.
type Type string

const (
	EntryActions             Type = "entry.upsert"
	EntryDeleted             Type = "entry.deleted"
	EntryRetrieveSecretValue Type = "entry.retrieve.secret"
	TemplateCreated          Type = "template.created"
	TemplateUpdated          Type = "template.updated"
)

type Event[T any] struct {
	Type          Type      `json:"type"`
	TransactionId string    `json:"transactionId"`
	Username      string    `json:"username"`
	Timestamp     time.Time `json:"timestamp"`
	Payload       T         `json:"payload"`
}

type Webhook struct {
	ID     string `dynamodbav:"ID"     json:"id"`
	URL    string `dynamodbav:"URL"    json:"url"`
	Events []Type `dynamodbav:"Events" json:"events"` // Lista de eventos a los que está suscrito
}

// Publisher es el contrato para publicar eventos en el bus.
type Publisher interface {
	Publish(ctx context.Context, event Event[json.RawMessage]) error
}

// Dispatcher despacha eventos a la infraestructura subyacente.
type Dispatcher interface {
	Dispatch(ctx context.Context, event Event[json.RawMessage])
}

// WebhookStore persiste y consulta webhooks registrados.
type WebhookStore interface {
	FindByEventType(ctx context.Context, t Type) ([]Webhook, error)
}
