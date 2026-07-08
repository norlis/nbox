package commands

import (
	"fmt"
	"strings"

	"nbox/internal/config"
)

// formatExport builds a shell `export VAR='value'` line. Single-quotes are
// escaped as '\” so the line is safe to paste or eval.
func formatExport(envVar string, value []byte) string {
	esc := strings.ReplaceAll(string(value), "'", `'\''`)
	return fmt.Sprintf("export %s='%s'", envVar, esc)
}

// emitEnv prints the single-entity env-var export to stdout (local/env-only mode).
func emitEnv(k config.Key, id string, data []byte) {
	outln(formatExport(k.EnvVar, config.EnvValue(k, id, data)))
}
