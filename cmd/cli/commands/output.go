package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Output channels follow clig.dev: the primary, machine-readable output of a
// command (pasteable JSON, list rows) goes to stdout so it pipes cleanly into
// jq or a file; everything else — prompts, banners, status, warnings and
// errors — goes to stderr so it never pollutes that stream.

// out writes primary/machine-readable output to stdout.
func out(format string, a ...any) { fmt.Fprintf(os.Stdout, format, a...) }

// outln writes a primary-output line to stdout.
func outln(s string) { fmt.Fprintln(os.Stdout, s) }

// info writes a human diagnostic (status, banner, prompt) to stderr.
func info(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }

// infoln writes a human diagnostic line to stderr.
func infoln(s string) { fmt.Fprintln(os.Stderr, s) }

// usageError marks a CLI misuse (bad/missing flags or args). It maps to exit
// code 2, distinct from a general runtime failure (1).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// usageErrorf builds a usage error (exit code 2).
func usageErrorf(format string, a ...any) error {
	return &usageError{err: fmt.Errorf(format, a...)}
}

// exactArgs wraps cobra.ExactArgs so a wrong argument count is reported as a
// usage error (exit code 2), consistent with flag-parse errors.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return usageErrorf("%w", err)
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs mapped to a usage error (exit 2), consistent
// with exactArgs.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(n)(cmd, args); err != nil {
			return usageErrorf("%w", err)
		}
		return nil
	}
}

// exitCode maps an error to a process exit status: 0 on success, 2 on usage
// error, 1 on any other failure.
func exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.As(err, new(*usageError)):
		return 2
	default:
		return 1
	}
}
