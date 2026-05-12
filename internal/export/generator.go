package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	"nbox/internal/export/exporter"
)

// Generator handles export of entries to various formats.
type Generator struct {
	entryAdapter entry.Manager
	config       *application.Config
	logger       *zap.Logger
	exporters    map[Format]exporter.Exporter
}

func NewGenerator(
	entryAdapter entry.Manager,
	config *application.Config,
	logger *zap.Logger,
) *Generator {
	g := &Generator{
		entryAdapter: entryAdapter,
		config:       config,
		logger:       logger,
		exporters:    make(map[Format]exporter.Exporter),
	}

	g.exporters[FormatJSON] = exporter.NewJSON()
	g.exporters[FormatYAML] = exporter.NewYAML()
	g.exporters[FormatDotEnv] = exporter.NewDotenv()
	g.exporters[FormatECSTaskDef] = exporter.NewECS()

	return g
}

func (g *Generator) Export(ctx context.Context, opts Options) (*Result, error) {
	g.logger.Info("Starting export",
		zap.String("prefix", opts.Prefix),
		zap.String("format", string(opts.Format)),
	)

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate export options: %w", err)
	}

	entries, err := g.entryAdapter.List(ctx, opts.Prefix)
	if err != nil {
		g.logger.Error("Failed to list entries", zap.Error(err))
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	if len(entries) == 0 {
		g.logger.Warn("No entries found for export", zap.String("prefix", opts.Prefix))
		return nil, fmt.Errorf("%w: %s", entry.ErrEntryNotFound, opts.Prefix)
	}

	ex, ok := g.exporters[opts.Format]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFormat, opts.Format)
	}

	content, err := ex.Export(entries)
	if err != nil {
		g.logger.Error("Export failed", zap.Error(err))
		return nil, fmt.Errorf("export failed: %w", err)
	}

	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	result := &Result{
		Entries: entries,
		Content: content,
		Size:    int64(len(content)),
	}

	g.logger.Info("Export completed successfully",
		zap.Int("entries_count", len(entries)),
		zap.Int64("size_bytes", result.Size),
		zap.String("checksum", checksum),
	)

	return result, nil
}

func (g *Generator) GetContentType(format Format) string {
	return format.ContentType()
}

func (g *Generator) GetFilename(format Format, prefix string) string {
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
