package usecases

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nbox/internal/domain/models"
	exporter2 "nbox/internal/usecases/exporter"
)

func TestJSONExporter_Export(t *testing.T) {
	t.Parallel()

	exporter := exporter2.NewJSONExporter()

	entries := []models.Entry{
		{Key: "test/key1", Value: "value1", Secure: false},
		{Key: "test/key2", Value: "value2", Secure: true},
	}

	data, err := exporter.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "test/key1")
	assert.Contains(t, string(data), "value1")
}

func TestYAMLExporter_Export(t *testing.T) {
	t.Parallel()

	exporter := exporter2.NewYAMLExporter()

	entries := []models.Entry{
		{Key: "test/key1", Value: "value1", Secure: false},
	}

	data, err := exporter.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "test/key1")
}

func TestDotEnvExporter_Export(t *testing.T) {
	t.Parallel()

	exporter := exporter2.NewDotEnvExporter()

	entries := []models.Entry{
		{Key: "production/myapp/database/host", Value: "localhost", Secure: false},
		{Key: "test/with spaces", Value: "value with spaces", Secure: false},
	}

	data, err := exporter.Export(entries)

	require.NoError(t, err)
	assert.NotEmpty(t, data)

	content := string(data)

	// Verificar conversión de keys (/ se convierte en _)
	assert.Contains(t, content, "PRODUCTION_MYAPP_DATABASE_HOST=localhost")

	// Verificar escape de valores con espacios
	assert.Contains(t, content, "TEST_WITH_SPACES=\"value with spaces\"")
}

// Note: convertToEnvVarName and escapeValue are tested indirectly
// through TestDotEnvExporter_Export which covers the complete export flow

func TestExportOptions_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options models.ExportOptions
		wantErr bool
	}{
		{
			name: "valid json format",
			options: models.ExportOptions{
				Format: models.ExportFormatJSON,
			},
			wantErr: false,
		},
		{
			name: "valid yaml format",
			options: models.ExportOptions{
				Format: models.ExportFormatYAML,
			},
			wantErr: false,
		},
		{
			name: "invalid format",
			options: models.ExportOptions{
				Format: models.ExportFormat("invalid"),
			},
			wantErr: true,
		},
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
		format      models.ExportFormat
		contentType string
	}{
		{models.ExportFormatJSON, "application/json"},
		{models.ExportFormatYAML, "application/x-yaml"},
		{models.ExportFormatDotEnv, "text/plain"},
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
		format    models.ExportFormat
		extension string
	}{
		{models.ExportFormatJSON, ".json"},
		{models.ExportFormatYAML, ".yaml"},
		{models.ExportFormatDotEnv, ".env"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			t.Parallel()
			result := tt.format.FileExtension()
			assert.Equal(t, tt.extension, result)
		})
	}
}
