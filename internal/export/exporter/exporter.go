package exporter

import (
	"nbox/internal/entry"
)

// Exporter converts a list of entries to a specific format.
type Exporter interface {
	Export(entries []entry.Entry) ([]byte, error)
}
