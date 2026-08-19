package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/norlis/httpgate/logging"
	"nbox/internal/entry"
	"nbox/internal/export/exporter"
	"nbox/internal/logfields"
	"nbox/internal/nbox"
)

// Generator handles export of entries to various formats.
type Generator struct {
	entryAdapter entry.Manager
	config       *nbox.Config
	logger       *slog.Logger
	exporters    map[Format]exporter.Exporter
}

func NewGenerator(
	entryAdapter entry.Manager,
	config *nbox.Config,
	logger *slog.Logger,
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
	start := time.Now()

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate export options: %w", err)
	}

	entries, err := g.entryAdapter.List(ctx, opts.Prefix)
	if err != nil {
		g.logger.ErrorContext(ctx, "export failed", logging.Err(err))
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	if len(entries) == 0 {
		g.logger.InfoContext(ctx, "export returned no entries", slog.String(logfields.KeyNboxPrefix, opts.Prefix))
		return nil, fmt.Errorf("%w: %s", entry.ErrEntryNotFound, opts.Prefix)
	}

	ex, ok := g.exporters[opts.Format]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFormat, opts.Format)
	}

	content, err := ex.Export(entries)
	if err != nil {
		g.logger.ErrorContext(ctx, "export failed", logging.Err(err))
		return nil, fmt.Errorf("export failed: %w", err)
	}

	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	result := &Result{
		Entries: entries,
		Content: content,
		Size:    int64(len(content)),
	}

	g.logger.InfoContext(ctx, "export completed",
		slog.String(logfields.KeyNboxFormat, string(opts.Format)),
		slog.Int(logfields.KeyEntriesTotal, len(entries)),
		slog.Int64(logging.KeyEventDuration, time.Since(start).Nanoseconds()),
		slog.Int64("size_bytes", result.Size),
		slog.String("checksum", checksum),
	)

	return result, nil
}

func (g *Generator) GetContentType(format Format) string {
	return format.ContentType()
}

func (g *Generator) GetFilename(format Format, prefix string) string {
	timestamp := time.Now().Format("20060102-150405")
	instance := g.config.InstanceName
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
