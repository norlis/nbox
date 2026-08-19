package spec

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/norlis/httpgate/logging"
	"nbox/internal/logfields"
)

// FSStore carga SpecDefinitions desde un filesystem.
// Implementa SpecStore.
type FSStore struct {
	fileSystem fs.FS
	logger     *slog.Logger
}

// NewFSStore creates a new filesystem-based store.
func NewFSStore(fileSystem fs.FS, logger *slog.Logger) *FSStore {
	return &FSStore{
		fileSystem: fileSystem,
		logger:     logger,
	}
}

// LoadAll scans the filesystem and extracts specs from .cue files.
func (s *FSStore) LoadAll(ctx context.Context) ([]SpecDefinition, error) {
	var specs []SpecDefinition
	cueCtx := cuecontext.New()

	err := fs.WalkDir(s.fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			s.logger.WarnContext(ctx, "boxspec definition skipped", slog.String(logfields.KeyFilePath, path), logging.Err(err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".cue" {
			return nil
		}

		content, err := fs.ReadFile(s.fileSystem, path)
		if err != nil {
			s.logger.WarnContext(ctx, "boxspec definition skipped", slog.String(logfields.KeyFilePath, path), logging.Err(err))
			return nil
		}

		val := cueCtx.CompileString(string(content))
		if val.Err() != nil {
			s.logger.WarnContext(ctx, "boxspec definition skipped", slog.String(logfields.KeyFilePath, path), logging.Err(val.Err()))
			return nil
		}

		metaVal := val.LookupPath(cue.ParsePath("#Meta"))
		if !metaVal.Exists() {
			return nil
		}

		var meta struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Version       string   `json:"version"`
			MatchPatterns []string `json:"matchPatterns"`
		}
		if err := metaVal.Decode(&meta); err != nil {
			s.logger.WarnContext(ctx, "boxspec definition skipped", slog.String(logfields.KeyFilePath, path), logging.Err(err))
			return nil
		}

		s.logger.DebugContext(ctx, "boxspec definition loaded",
			slog.String(logfields.KeyFilePath, path),
			slog.String(logfields.KeyEventID, meta.ID),
			slog.String(logfields.KeyNboxTemplate, meta.Name),
		)

		specs = append(specs, SpecDefinition{
			ID:            meta.ID,
			Name:          meta.Name,
			Version:       meta.Version,
			MatchPatterns: meta.MatchPatterns,
			RawContent:    string(content),
		})
		return nil
	})

	s.logger.InfoContext(ctx, "boxspec definitions loaded", slog.Int(logfields.KeyEntriesTotal, len(specs)))

	return specs, err
}
