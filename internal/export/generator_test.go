package export_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nbox/internal/entry"
	"nbox/internal/export"
	exporter "nbox/internal/export/exporter"
	"nbox/internal/nbox"
)

func TestJSONExporter_Export(t *testing.T) {
	t.Parallel()

	exp := exporter.NewJSON()

	entries := []entry.Entry{
		{Key: "test/key1", Value: "value1", Secure: false},
		{Key: "test/key2", Value: "value2", Secure: true},
	}

	data, err := exp.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "test/key1")
	assert.Contains(t, string(data), "value1")
}

func TestYAMLExporter_Export(t *testing.T) {
	t.Parallel()

	exp := exporter.NewYAML()

	entries := []entry.Entry{
		{Key: "test/key1", Value: "value1", Secure: false},
	}

	data, err := exp.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "test/key1")
}

func TestDotEnvExporter_Export(t *testing.T) {
	t.Parallel()

	exp := exporter.NewDotenv()

	entries := []entry.Entry{
		{Key: "production/myapp/database/host", Value: "localhost", Secure: false},
		{Key: "test/with spaces", Value: "value with spaces", Secure: false},
	}

	data, err := exp.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)

	content := string(data)

	assert.Contains(t, content, "PRODUCTION_MYAPP_DATABASE_HOST=localhost")
	assert.Contains(t, content, "TEST_WITH_SPACES=\"value with spaces\"")
}

func TestExportOptions_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options export.Options
		wantErr bool
	}{
		{"valid json format", export.Options{Format: export.FormatJSON}, false},
		{"valid yaml format", export.Options{Format: export.FormatYAML}, false},
		{"invalid format", export.Options{Format: export.Format("invalid")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.options.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExportFormat_ContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format      export.Format
		contentType string
	}{
		{export.FormatJSON, "application/json"},
		{export.FormatYAML, "application/x-yaml"},
		{export.FormatDotEnv, "text/plain"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			t.Parallel()
			result := tt.format.ContentType()
			assert.Equal(t, tt.contentType, result)
		})
	}
}

func TestExportFormat_FileExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format    export.Format
		extension string
	}{
		{export.FormatJSON, ".json"},
		{export.FormatYAML, ".yaml"},
		{export.FormatDotEnv, ".env"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			t.Parallel()
			result := tt.format.FileExtension()
			assert.Equal(t, tt.extension, result)
		})
	}
}

func TestGenerator_GetFilename_UsesInstanceNameFromConfig(t *testing.T) {
	t.Parallel()

	cfg := &nbox.Config{InstanceName: "myapp"}
	g := export.NewGenerator(nil, cfg, slog.New(slog.DiscardHandler))

	filename := g.GetFilename(export.FormatJSON, "production")

	assert.True(t, strings.HasPrefix(filename, "myapp-"), "filename should start with instance name, got: %s", filename)
	assert.Contains(t, filename, "production")
	assert.True(t, strings.HasSuffix(filename, ".json"), "filename should have .json extension, got: %s", filename)
}

func TestGenerator_GetFilename_DefaultsToNboxWhenInstanceNameEmpty(t *testing.T) {
	t.Parallel()

	cfg := &nbox.Config{InstanceName: ""}
	g := export.NewGenerator(nil, cfg, slog.New(slog.DiscardHandler))

	filename := g.GetFilename(export.FormatDotEnv, "qa")

	assert.True(t, strings.HasPrefix(filename, "nbox-"), "filename should default to 'nbox' prefix, got: %s", filename)
}
