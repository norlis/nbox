package spec

import (
	"io/fs"
)

// LayeredFS implements fs.FS with fallback layers.
// External (hot-reload) takes priority over Embedded (default).
type LayeredFS struct {
	layers []fs.FS
}

// NewLayeredFS creates a layered filesystem.
// Layers are checked in order; first match wins.
func NewLayeredFS(layers ...fs.FS) *LayeredFS {
	return &LayeredFS{layers: layers}
}

// Open implements fs.FS.
func (l *LayeredFS) Open(name string) (fs.File, error) {
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
	}
	return nil, fs.ErrNotExist
}

// ReadDir implements fs.ReadDirFS for directory listing.
func (l *LayeredFS) ReadDir(name string) ([]fs.DirEntry, error) {
	seen := make(map[string]bool)
	var result []fs.DirEntry

	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		if rdfs, ok := layer.(fs.ReadDirFS); ok {
			entries, err := rdfs.ReadDir(name)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !seen[entry.Name()] {
					seen[entry.Name()] = true
					result = append(result, entry)
				}
			}
		}
	}
	return result, nil
}
