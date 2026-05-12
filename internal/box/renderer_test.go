package box

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nbox/internal/box/store/spec"
	"nbox/internal/entry"
)

type mockTemplateAdapter struct{}

type mockEntryAdapter struct{}

func (m *mockEntryAdapter) Upsert(_ context.Context, _ []entry.Entry) entry.Results {
	return nil
}

func (m *mockEntryAdapter) Retrieve(_ context.Context, _ string, _ ...entry.RetrieveOption) (*entry.Entry, error) {
	return nil, errors.New("mock retrieval error")
}

func (m *mockEntryAdapter) Resolve(_ context.Context, _ string) (*entry.Entry, error) {
	return nil, nil
}

func (m *mockEntryAdapter) List(_ context.Context, _ string) ([]entry.Entry, error) {
	text := `[
		{ "path": "widget-x/development", "key": "key", "value": "key-test", "secure": false },
		{ "path": "widget-x/development", "key": "debug", "value": "false", "secure": false },
		{ "path": "widget-x", "key": "sentry", "value": "xxxxx12345", "secure": false },
		{ "path": " ", "key": "private-domain", "value": "private.io", "secure": false }
	]`
	var entries []entry.Entry
	_ = json.Unmarshal([]byte(text), &entries)
	return entries, nil
}

func (m *mockEntryAdapter) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockEntryAdapter) RetrieveMany(_ context.Context, keys []string) (map[string]*entry.Entry, error) {
	data := map[string]string{
		"widget-x/development/key":   "key-test",
		"widget-x/development/debug": "false",
		"widget-x/sentry":            "xxxxx12345",
		"private-domain":             "private.io",
	}
	results := make(map[string]*entry.Entry, len(keys))
	for _, k := range keys {
		if v, ok := data[k]; ok {
			results[k] = &entry.Entry{Key: k, Value: v}
		}
	}
	return results, nil
}

func (m *mockEntryAdapter) RegisterBackend(_ entry.PartialStore) {}

func (m *mockTemplateAdapter) UpsertBox(_ context.Context, _ *Box) map[string]spec.Result {
	return nil
}

func (m *mockTemplateAdapter) BoxExists(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

func (m *mockTemplateAdapter) RetrieveBox(_ context.Context, _, _, _ string) ([]byte, error) {
	text := `{"service": ":service","ENV_1": "{{ widget-x/:stage/key }}", "ENV_2": "{{widget-x/development/debug}}", "GLOBAL_SERVICE": "{{widget-x/sentry}}", "domain": "{{private-domain}}", "version": "1", "missing":"{{missing}}"}`
	return []byte(text), nil
}

func (m *mockTemplateAdapter) List(_ context.Context) ([]Box, error) {
	return nil, nil
}

func (m *mockTemplateAdapter) Detail(_ context.Context, _, _ string) (*Stage, error) {
	return nil, nil
}

func TestRenderer_BuildBox(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer(&mockTemplateAdapter{}, &mockEntryAdapter{}, &entry.Processor{}, NewStrategyResolver())
	results, err := renderer.BuildBox(context.Background(), "test", "development", "test.json", map[string]string{})

	fmt.Println(results)

	expected := `{"service": "test","ENV_1": "key-test", "ENV_2": "false", "GLOBAL_SERVICE": "xxxxx12345", "domain": "private.io", "version": "1", "missing":""}`

	if err != nil {
		t.Errorf(`Expected %s got: err %s`, expected, err)
	}

	if strings.TrimSpace(results) != strings.TrimSpace(expected) {
		t.Errorf(`Expected %s got: %s`, expected, results)
	}
}
