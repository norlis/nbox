package specs

import "embed"

// FS expone los archivos CUE embebidos para ser consumidos por la aplicación.
//
//go:embed **/*.cue
var FS embed.FS
