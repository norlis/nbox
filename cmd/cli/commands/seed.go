package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"nbox/cmd/cli/bootstrap"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
)

// runSeed returns the Fx-invokable function.
// It receives a reader already prepared to decouple Fx logic from file handling.
func runSeed(reader io.Reader, sourceDescription string) func(domain.PrefixConfigRepository, fx.Shutdowner) {
	return func(repo domain.PrefixConfigRepository, shutdowner fx.Shutdowner) {
		// Ensure shutdown on exit
		defer func() { _ = shutdowner.Shutdown() }()

		fmt.Printf("\n🌱 Seeding prefix configurations from: %s\n", sourceDescription)

		// Read content
		content, err := io.ReadAll(reader)
		if err != nil {
			printError("Failed to read input", err)
			return
		}

		// Parse JSON (supports both array and single object)
		configs, err := parseConfigs(content)
		if err != nil {
			printError("Invalid JSON format", err)
			fmt.Println("💡 Tip: Ensure fields match 'prefix', 'typeDefault', etc.")
			return
		}

		if len(configs) == 0 {
			fmt.Println("⚠️  Warning: No configurations found in input.")
			return
		}

		// Prepare data (audit fields)
		prepareConfigs(configs)

		// Persist with batch upsert
		fmt.Printf("\n📊 Processing %d configurations...\n", len(configs))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fmt.Printf("💾 Saving to DynamoDB...\n")
		stats, err := repo.Upsert(ctx, configs)

		// Print summary report
		printSummary(stats, len(configs), err)
	}
}

// parseConfigs parses JSON that can be either an array [...] or a single object {...}.
func parseConfigs(content []byte) ([]backend.PrefixConfig, error) {
	trimmed := bytes.TrimSpace(content)

	// Try as array first
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var configs []backend.PrefixConfig
		if err := json.Unmarshal(content, &configs); err != nil {
			return nil, err
		}
		return configs, nil
	}

	// Try as single object and convert to slice
	var single backend.PrefixConfig
	if err := json.Unmarshal(content, &single); err != nil {
		return nil, err
	}
	return []backend.PrefixConfig{single}, nil
}

// prepareConfigs sets timestamps and audit fields.
func prepareConfigs(configs []backend.PrefixConfig) {
	now := time.Now().UTC()
	for i := range configs {
		if configs[i].CreatedAt.IsZero() {
			configs[i].CreatedAt = now
		}
		configs[i].UpdatedAt = now
		configs[i].UpdatedBy = "nbox-cli"
		// Visual feedback
		fmt.Printf("  • %s (%s)\n", configs[i].Prefix, configs[i].TypeDefault)
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

// printError prints formatted error messages.
func printError(msg string, err error) {
	fmt.Printf("❌ Error: %s: %v\n", msg, err)
}

// printSummary displays the operation summary.
func printSummary(stats backend.UpsertStats, total int, err error) {
	if err != nil {
		fmt.Printf("\n⚠️  Warning: Batch operation reported errors: %v\n", err)
	}

	fmt.Printf("\n%s\n", strings.Repeat("═", 50))
	fmt.Printf("📈 Summary:\n")
	fmt.Printf("   Total items: %d\n", total)
	fmt.Printf("   Processed:   %d\n", stats.Processed)
	fmt.Printf("   Failed:      %d\n", stats.Failed)
	fmt.Printf("   Skipped:     %d\n", stats.Skipped)
	fmt.Printf("%s\n", strings.Repeat("═", 50))

	if stats.Failed == 0 && err == nil {
		fmt.Printf("\n✨ Success! All configurations saved.\n")
	} else {
		fmt.Printf("\n⚠️  Completed with issues.\n")
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
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Prepare reader before starting Fx to validate input early
		reader, desc, err := getInputReader(args[0])
		if err != nil {
			printError("Input validation", err)
			os.Exit(1)
		}

		// Ensure file is closed if not STDIN
		if closer, ok := reader.(io.Closer); ok && args[0] != "-" {
			defer func() { _ = closer.Close() }()
		}

		app := fx.New(
			fx.NopLogger,
			bootstrap.CommonModules,
			// Pass reader and description to Fx invokable
			fx.Invoke(runSeed(reader, desc)),
		)

		if err := app.Start(cmd.Context()); err != nil {
			printError(err.Error(), err)
			os.Exit(1)
		}
	},
}
