package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatExport(t *testing.T) {
	assert.Equal(t, `export NBOX_X='{"a":1}'`, formatExport("NBOX_X", []byte(`{"a":1}`)))

	// Single quotes escaped so the export line stays eval/paste-safe.
	assert.Equal(t, `export NBOX_X='it'\''s'`, formatExport("NBOX_X", []byte(`it's`)))
}
