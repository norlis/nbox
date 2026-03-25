package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"nbox/internal/domain/validation"
)

type mockTemplateAdapter struct{}

type mockEntryAdapter struct{}

func (m *mockEntryAdapter) Upsert(_ context.Context, _ []models.Entry) operations.Results {
	return nil
}

func (m *mockEntryAdapter) Retrieve(_ context.Context, _ string, _ ...domain.RetrieveOption) (*models.Entry, error) {
	return nil, errors.New("mock retrieval error")
}

func (m *mockEntryAdapter) Resolve(_ context.Context, _ string) (*models.Entry, error) {
	return nil, nil
}

func (m *mockEntryAdapter) List(_ context.Context, _ string) ([]models.Entry, error) {
	text := `[
		{ "path": "widget-x/development", "key": "key", "value": "key-test", "secure": false },
		{ "path": "widget-x/development", "key": "debug", "value": "false", "secure": false },
		{ "path": "widget-x", "key": "sentry", "value": "xxxxx12345", "secure": false },
		{ "path": " ", "key": "private-domain", "value": "private.io", "secure": false }
	]`
	var entries []models.Entry
	_ = json.Unmarshal([]byte(text), &entries)
	return entries, nil
}

func (m *mockEntryAdapter) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockEntryAdapter) RetrieveMany(_ context.Context, _ []string) (map[string]*models.Entry, error) {
	return nil, nil
}

func (m *mockEntryAdapter) RegisterBackend(_ domain.EntryPartialStore) {}

func (m *mockTemplateAdapter) UpsertBox(_ context.Context, _ *models.Box) map[string]validation.Result {
	return nil
}

func (m *mockTemplateAdapter) BoxExists(ctx context.Context, service, stage, template string) (bool, error) {
	return false, nil
}

func (m *mockTemplateAdapter) RetrieveBox(ctx context.Context, service, stage, template string) ([]byte, error) {
	text := `{"service": ":service","ENV_1": "{{ widget-x/:stage/key }}", "ENV_2": "{{widget-x/development/debug}}", "GLOBAL_SERVICE": "{{widget-x/sentry}}", "domain": "{{private-domain}}", "version": "1", "missing":"{{missing}}"}`
	return []byte(text), nil
}

func (m *mockTemplateAdapter) List(ctx context.Context) ([]models.Box, error) {
	return nil, nil
}

func TestBoxUseCase_BuildBox(t *testing.T) {
	t.Parallel()

	mockTemplate := &mockTemplateAdapter{}
	mockEntry := &mockEntryAdapter{}

	useCase := NewBox(mockTemplate, mockEntry, NewPathUseCase(), nil)
	results, err := useCase.BuildBox(context.Background(), "test", "development", "test.json", map[string]string{})

	fmt.Println(results)

	expected := `{"service": "test","ENV_1": "key-test", "ENV_2": "false", "GLOBAL_SERVICE": "xxxxx12345", "domain": "private.io", "version": "1", "missing":""}`

	if err != nil {
		t.Errorf(`Expected %s got: err %s`, expected, err)
	}

	if strings.TrimSpace(results) != strings.TrimSpace(expected) {
		t.Errorf(`Expected %s got: %s`, expected, results)
	}
}
