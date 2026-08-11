package event

import (
	"context"
	"encoding/json"
	"time"
)

// Type identifica un tipo de evento del sistema.
type Type string

const (
	EntryUpserted   Type = "nbox.entry.upserted"
	EntryDeleted    Type = "nbox.entry.deleted"
	EntrySecretRead Type = "nbox.entry.secret.read" //nolint:gosec // G101: event type name, not a credential
	TemplateCreated Type = "nbox.template.created"
	TemplateUpdated Type = "nbox.template.updated"
	// Keepalive is the application-level heartbeat emitted by entrypushd
	// on otherwise-idle Watch streams so L7 proxies (ALB) don't idle-kill
	// them. Carries no subject or payload; consumers must skip it.
	Keepalive Type = "nbox.keepalive"
)

type Event struct {
	Type          Type            `json:"type"`
	TransactionId string          `json:"transactionId"`
	Username      string          `json:"username"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
}

type Webhook struct {
	ID     string `dynamodbav:"ID"     json:"id"`
	URL    string `dynamodbav:"URL"    json:"url"`
	Events []Type `dynamodbav:"Events" json:"events"` // Lista de eventos a los que está suscrito
}

// Publisher es el contrato para publicar eventos en el bus.
// Accepts 1..N events; implementations choose batching strategy.
type Publisher interface {
	Publish(ctx context.Context, events ...Event) error
}

// WebhookStore persiste y consulta webhooks registrados.
type WebhookStore interface {
	FindByEventType(ctx context.Context, t Type) ([]Webhook, error)
}
