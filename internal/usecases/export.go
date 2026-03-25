package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/usecases/exporter"
)

// ExportUseCase maneja la lógica de exportación.
type ExportUseCase struct {
	entryAdapter domain.EntryManager
	config       *application.Config
	logger       *zap.Logger
	exporters    map[models.ExportFormat]exporter.Exporter
}

// NewExportUseCase crea una nueva instancia.
func NewExportUseCase(
	entryAdapter domain.EntryManager,
	config *application.Config,
	logger *zap.Logger,
) *ExportUseCase {
	uc := &ExportUseCase{
		entryAdapter: entryAdapter,
		config:       config,
		logger:       logger,
		exporters:    make(map[models.ExportFormat]exporter.Exporter),
	}

	uc.exporters[models.ExportFormatJSON] = exporter.NewJSONExporter()
	uc.exporters[models.ExportFormatYAML] = exporter.NewYAMLExporter()
	uc.exporters[models.ExportFormatDotEnv] = exporter.NewDotEnvExporter()
	uc.exporters[models.ExportFormatECSTaskDef] = exporter.NewECSTaskDefExporter()

	return uc
}

func (uc *ExportUseCase) Export(ctx context.Context, opts models.ExportOptions) (*models.ExportResult, error) {
	uc.logger.Info("Starting export",
		zap.String("prefix", opts.Prefix),
		zap.String("format", string(opts.Format)),
	)

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate export options: %w", err)
	}

	entries, err := uc.entryAdapter.List(ctx, opts.Prefix)
	if err != nil {
		uc.logger.Error("Failed to list entries", zap.Error(err))
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	if len(entries) == 0 {
		uc.logger.Warn("No entries found for export", zap.String("prefix", opts.Prefix))
		return nil, fmt.Errorf("%w: %s", domain.ErrEntryNotFound, opts.Prefix)
	}

	ex, ok := uc.exporters[opts.Format]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidExportFormat, opts.Format)
	}

	content, err := ex.Export(entries)
	if err != nil {
		uc.logger.Error("Export failed", zap.Error(err))
		return nil, fmt.Errorf("export failed: %w", err)
	}

	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	result := &models.ExportResult{
		Entries: entries,
		Content: content,
		Size:    int64(len(content)),
	}

	uc.logger.Info("Export completed successfully",
		zap.Int("entries_count", len(entries)),
		zap.Int64("size_bytes", result.Size),
		zap.String("checksum", checksum),
	)

	return result, nil
}

func (uc *ExportUseCase) GetContentType(format models.ExportFormat) string {
	return format.ContentType()
}

func (uc *ExportUseCase) GetFilename(format models.ExportFormat, prefix string) string {
	timestamp := time.Now().Format("20060102-150405")
	instance := os.Getenv("INSTANCE_NAME")
	if instance == "" {
		instance = "nbox"
	}

	cleanPrefix := prefix
	if cleanPrefix == "" {
		cleanPrefix = "all"
	}

	return fmt.Sprintf("%s-export-%s-%s%s",
		instance,
		cleanPrefix,
		timestamp,
		format.FileExtension(),
	)
}
