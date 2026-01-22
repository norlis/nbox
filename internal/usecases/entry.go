package usecases

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
)

type EntryUseCase struct {
	entryAdapter  domain.EntryAdapter
	secretAdapter domain.SecretAdapter
	config        *application.Config
}

// TODO eliminar
func NewEntryUseCase(
	entryAdapter domain.EntryAdapter,
	secretAdapter domain.SecretAdapter,
	config *application.Config,
) domain.EntryUseCase {
	return &EntryUseCase{entryAdapter: entryAdapter, secretAdapter: secretAdapter, config: config}
}

// Upsert
// ARN arn:aws:ssm:<REGION_NAME>:<ACCOUNT_ID>:parameter/<parameter-name>.
func (e *EntryUseCase) Upsert(ctx context.Context, entries []models.Entry) []operations.Result {
	secrets := make([]models.Entry, 0)
	for _, entry := range entries {
		if entry.Secure {
			secrets = append(secrets, entry)
		}
	}

	secureResults := e.secretAdapter.Upsert(ctx, secrets)

	for i, entry := range entries {
		if !entry.Secure {
			continue
		}

		err := secureResults[entry.Key].Err
		entries[i].Value = ""

		if err != nil {
			continue
		}

		key := cleanedKey(entry.Key)
		entries[i].Value = e.GetParameterArn(key)
	}

	updated := e.entryAdapter.Upsert(ctx, entries)

	maps.Copy(updated, secureResults)

	results := make([]operations.Result, 0, len(updated))
	for _, v := range updated {
		results = append(results, v)
	}

	return results
}

func (e *EntryUseCase) GetParameterArn(key string) string {
	if e.config.ParameterShortArn && !strings.HasPrefix(key, "/") {
		return "/" + key
	}

	if e.config.ParameterShortArn {
		return key
	}

	return fmt.Sprintf(
		"arn:aws:ssm:%s:%s:parameter/%s", e.config.RegionName, e.config.AccountId, cleanedKey(key),
	)
}

func cleanedKey(key string) string {
	return strings.TrimPrefix(key, "/")
}
