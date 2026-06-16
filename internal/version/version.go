// Package version exposes build-time metadata for this repo's binaries.
// Values are injected at link time via Makefile ldflags. When built without
// ldflags (e.g. `go run`), the vars stay empty and consumers should fall
// back to "unknown.dev" or similar.
package version

// GitHash is the short commit SHA the binary was built from.
// Injected via: -X nbox/internal/version.GitHash=$(GIT_SHA).
var GitHash string

// Date is the UTC build timestamp in RFC3339 format.
// Injected via: -X nbox/internal/version.Date=$(DATE).
var Date string
