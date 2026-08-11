package spec

import (
	"context"
	"io/fs"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"go.uber.org/zap"
)

// FSStore carga SpecDefinitions desde un filesystem.
// Implementa SpecStore.
type FSStore struct {
	fileSystem fs.FS
	logger     *zap.Logger
}

// NewFSStore creates a new filesystem-based store.
func NewFSStore(fileSystem fs.FS, logger *zap.Logger) *FSStore {
	return &FSStore{
		fileSystem: fileSystem,
		logger:     logger.Named("boxspec.store"),
	}
}

// LoadAll scans the filesystem and extracts specs from .cue files.
func (s *FSStore) LoadAll(_ context.Context) ([]SpecDefinition, error) {
	var specs []SpecDefinition
	cueCtx := cuecontext.New()

	s.logger.Info("Starting to load specs from filesystem")

	err := fs.WalkDir(s.fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			s.logger.Warn("Error walking directory", zap.String("path", path), zap.Error(err))
			return nil
		}
		if d.IsDir() {
			s.logger.Debug("Entering directory", zap.String("path", path))
			return nil
		}
		if filepath.Ext(path) != ".cue" {
			return nil
		}

		s.logger.Debug("Found CUE file", zap.String("path", path))

		bytes, err := fs.ReadFile(s.fileSystem, path)
		if err != nil {
			s.logger.Error("Failed to read file", zap.String("path", path), zap.Error(err))
			return nil
		}
		content := string(bytes)

		s.logger.Debug("Compiling CUE file", zap.String("path", path), zap.Int("size", len(content)))

		val := cueCtx.CompileString(content)
		if val.Err() != nil {
			s.logger.Error("Invalid CUE syntax", zap.String("path", path), zap.Error(val.Err()))
			return nil
		}

		s.logger.Debug("CUE compiled successfully", zap.String("path", path))

		metaVal := val.LookupPath(cue.ParsePath("#Meta"))
		if !metaVal.Exists() {
			s.logger.Debug("No #Meta block found, skipping", zap.String("path", path))
			return nil
		}

		s.logger.Debug("Found #Meta block", zap.String("path", path))

		var meta struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			Version       string   `json:"version"`
			MatchPatterns []string `json:"matchPatterns"`
		}
		if err := metaVal.Decode(&meta); err != nil {
			s.logger.Error("Failed to decode #Meta", zap.String("path", path), zap.Error(err))
			return nil
		}

		s.logger.Info("Loaded spec",
			zap.String("path", path),
			zap.String("id", meta.ID),
			zap.String("name", meta.Name),
			zap.Strings("matchPatterns", meta.MatchPatterns),
		)

		specs = append(specs, SpecDefinition{
			ID:            meta.ID,
			Name:          meta.Name,
			Version:       meta.Version,
			MatchPatterns: meta.MatchPatterns,
			RawContent:    content,
		})
		return nil
	})

	s.logger.Info("Finished loading specs", zap.Int("total", len(specs)))

	return specs, err
}
