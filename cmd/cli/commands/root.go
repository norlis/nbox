package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time: -ldflags "-X nbox/cmd/cli/commands.version=v1.2.3".
var version = "dev"

// rootCmd is the base command when invoked without subcommands.
var rootCmd = &cobra.Command{
	Use:   "nbox-cli",
	Short: "Admin CLI for nbox",
	Long: `NBox CLI performs administrative and maintenance tasks against the
NBox infrastructure: prefix routing, dynamic auth config (users, AppRole,
AWS-STS) and local env-var generation via --emit-env.`,
}

// Execute runs the root command and exits the process with a status code
// mapped from the returned error (0 ok, 2 usage error, 1 otherwise). Cobra
// prints the error to stderr; usage is silenced so runtime errors don't dump
// the full help text. Called by main.main().
func Execute() {
	os.Exit(exitCode(rootCmd.Execute()))
}

func init() {
	// Runtime errors shouldn't spew usage; flag-parse errors become usage
	// errors (exit 2) while cobra still prints them to stderr.
	rootCmd.SilenceUsage = true
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageErrorf("%w", err)
	})

	// Persistent flags are inherited by every subcommand.
	rootCmd.PersistentFlags().StringP("region", "r", "", "AWS region (overrides AWS_REGION)")

	// Apply --region to AWS_REGION before any subcommand runs (flag > env).
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		region, _ := cmd.Flags().GetString("region")
		return applyRegion(region, os.Setenv)
	}

	rootCmd.Version = version

	// Registrar subcomandos
	rootCmd.AddCommand(prefixCmd)
	rootCmd.AddCommand(configCmd)
}
