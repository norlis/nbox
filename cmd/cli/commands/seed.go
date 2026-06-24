package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"nbox/cmd/cli/bootstrap"
	"nbox/internal/prefix"
)

// runSeed returns the Fx-invokable function. It receives a reader already
// prepared to decouple Fx logic from file handling, and reports any failure
// through outErr (fx.Invoke can't return one) so RunE maps it to an exit code.
func runSeed(reader io.Reader, sourceDescription string, outErr *error) func(prefix.Store, fx.Shutdowner) {
	return func(repo prefix.Store, shutdowner fx.Shutdowner) {
		// Ensure shutdown on exit
		defer func() { _ = shutdowner.Shutdown() }()

		info("\n🌱 Seeding prefix configurations from: %s\n", sourceDescription)

		// Read content
		content, err := io.ReadAll(reader)
		if err != nil {
			*outErr = fmt.Errorf("read input: %w", err)
			return
		}

		// Parse JSON (supports both array and single object)
		configs, err := parseConfigs(content)
		if err != nil {
			infoln("💡 Tip: Ensure fields match 'prefix', 'typeDefault', etc.")
			*outErr = fmt.Errorf("invalid JSON format: %w", err)
			return
		}

		if len(configs) == 0 {
			*outErr = errors.New("no configurations found in input")
			return
		}

		// Prepare data (audit fields)
		prepareConfigs(configs)

		// Persist with batch upsert
		info("\n📊 Processing %d configurations...\n", len(configs))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		info("💾 Saving to DynamoDB...\n")
		stats, err := repo.Upsert(ctx, configs)

		// Print summary report (diagnostics) and surface a failure.
		printSummary(stats, len(configs), err)
		if err != nil {
			*outErr = fmt.Errorf("batch upsert: %w", err)
			return
		}
		if stats.Failed > 0 {
			*outErr = fmt.Errorf("%d configuration(s) failed", stats.Failed)
		}
	}
}

// parseConfigs parses JSON that can be either an array [...] or a single object {...}.
func parseConfigs(content []byte) ([]prefix.Config, error) {
	trimmed := bytes.TrimSpace(content)

	// Try as array first
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var configs []prefix.Config
		if err := json.Unmarshal(content, &configs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal seed data: %w", err)
		}
		return configs, nil
	}

	// Try as single object and convert to slice
	var single prefix.Config
	if err := json.Unmarshal(content, &single); err != nil {
		return nil, err
	}
	return []prefix.Config{single}, nil
}

// prepareConfigs sets timestamps and audit fields.
func prepareConfigs(configs []prefix.Config) {
	now := time.Now().UTC()
	for i := range configs {
		if configs[i].CreatedAt.IsZero() {
			configs[i].CreatedAt = now
		}
		configs[i].UpdatedAt = now
		configs[i].UpdatedBy = "nbox-cli"
		// Visual feedback
		info("  • %s (%s)\n", configs[i].Prefix, configs[i].TypeDefault)
	}
}

// getInputReader decides where to read from: STDIN, file, or inline string.
func getInputReader(input string) (io.Reader, string, error) {
	// Check for STDIN pipe
	if input == "-" {
		return os.Stdin, "STDIN", nil
	}

	// Check for existing file
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		f, err := os.Open(input)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open file: %w", err)
		}
		return f, fmt.Sprintf("file (%s)", input), nil
	}

	// Fallback to inline JSON
	// Validate it looks like JSON to give helpful error
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return strings.NewReader(input), "inline JSON", nil
	}

	return nil, "", fmt.Errorf("input '%s' is not a valid file and does not look like JSON", input)
}

// printSummary displays the operation summary on stderr (diagnostic output).
func printSummary(stats prefix.UpsertStats, total int, err error) {
	if err != nil {
		info("\n⚠️  Warning: Batch operation reported errors: %v\n", err)
	}

	info("\n%s\n", strings.Repeat("═", 50))
	info("📈 Summary:\n")
	info("   Total items: %d\n", total)
	info("   Processed:   %d\n", stats.Processed)
	info("   Failed:      %d\n", stats.Failed)
	info("   Skipped:     %d\n", stats.Skipped)
	info("%s\n", strings.Repeat("═", 50))

	if stats.Failed == 0 && err == nil {
		info("\n✨ Success! All configurations saved.\n")
	} else {
		info("\n⚠️  Completed with issues.\n")
	}
}

// seedCmd defines the Cobra command for seeding.
var seedCmd = &cobra.Command{
	Use:   "seed [file|json|-]",
	Short: "Seed prefix configurations",
	Long: `Seed prefix configurations from a file path, inline JSON, or STDIN.

Examples:
  # From file
  nbox-cli seed data.json

  # From inline JSON (supports single object or array)
  nbox-cli seed '{"prefix":"test","typeDefault":"dynamodb","typeAllowed":[]}'

  # From array
  nbox-cli seed '[{"prefix":"test","typeDefault":"dynamodb","typeAllowed":[]}]'

  # From Pipe (STDIN)
  cat config.json | nbox-cli seed -`,
	Args: exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Prepare reader before starting Fx to validate input early
		reader, desc, err := getInputReader(args[0])
		if err != nil {
			return usageErrorf("input: %w", err)
		}

		// Ensure file is closed if not STDIN
		if closer, ok := reader.(io.Closer); ok && args[0] != "-" {
			defer func() { _ = closer.Close() }()
		}

		// fx.Invoke runs synchronously during Start, so seedErr is set by the
		// time Start returns.
		var seedErr error
		app := fx.New(
			fx.NopLogger,
			bootstrap.CommonModules,
			// Pass reader and description to Fx invokable
			fx.Invoke(runSeed(reader, desc, &seedErr)),
		)

		if err := app.Start(cmd.Context()); err != nil {
			return err
		}
		return seedErr
	},
}
