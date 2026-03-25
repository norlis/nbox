package domain

import (
	"context"
	"encoding/json"

	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"nbox/internal/domain/validation"
)

type EntryUseCase interface {
	Upsert(ctx context.Context, entries []models.Entry) []operations.Result
}

// TemplateAdapter store templates.
type TemplateAdapter interface {
	UpsertBox(ctx context.Context, box *models.Box) map[string]validation.Result
	BoxExists(ctx context.Context, service, stage, template string) (bool, error)
	RetrieveBox(ctx context.Context, service, stage, template string) ([]byte, error)
	List(ctx context.Context) ([]models.Box, error)
}

// EntryAdapter vars backend operations.
type EntryAdapter interface {
	Upsert(ctx context.Context, entries []models.Entry) operations.Results
	Retrieve(ctx context.Context, key string) (*models.Entry, error)
	List(ctx context.Context, prefix string) ([]models.Entry, error)
	Delete(ctx context.Context, key string) error
	Tracking(ctx context.Context, key string) ([]models.Tracking, error)
}

// type

// SecretAdapter vars encrypt.
type SecretAdapter interface {
	Upsert(ctx context.Context, entries []models.Entry) operations.Results
	RetrieveSecretValue(ctx context.Context, key string) (*models.Entry, error)
}

type EventNotifier interface {
	Dispatch(ctx context.Context, event Event[json.RawMessage])
}

type WebhookRepository interface {
	FindByEventType(ctx context.Context, eventType EventType) ([]Webhook, error)
	// TODO: Add implementations
	// Create(ctx context.Context, webhook Webhook) error
	// Delete(ctx context.Context, webhookID string) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event Event[json.RawMessage]) error
}

type ExportAdapter interface {
	Export(ctx context.Context, entries []models.Entry, options models.ExportOptions) ([]byte, error)
}

type PrefixConfigRepository interface {
	List(ctx context.Context) ([]backend.PrefixConfig, error)
	GetByPrefix(ctx context.Context, prefix string) (*backend.PrefixConfig, error)
	Upsert(ctx context.Context, prefixes []backend.PrefixConfig) (backend.UpsertStats, error)
}
