package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time: -ldflags "-X nbox/cmd/cli/commands.version=v1.2.3".
var version = "dev"

// rootCmd representa el commando base cuando se llama sin subcomandos.
var rootCmd = &cobra.Command{
	Use:   "nbox-cli",
	Short: "Herramienta CLI de administración para nbox",
	Long: `NBox CLI es una herramienta para realizar tareas administrativas y de mantenimiento
sobre la infraestructura de NBox, como poblar configuraciones iniciales,
gestionar backups o validar integridad de datos.`,
	// Si quisieras que el commando raíz haga algo por sí mismo, descomenta esto:
	// Run: func(cmd *cobra.Command, args []string) { fmt.Println("Hola NBox") },
}

// Execute runs the root command and exits the process with a status code
// mapped from the returned error (0 ok, 2 usage error, 1 otherwise). Cobra
// prints the error to stderr; usage is silenced so runtime errors don't dump
// the full help text. Called by main.main().
func Execute() {
	os.Exit(exitCode(rootCmd.Execute()))
}

func init() {
	// Aquí defines tus flags y configuración.

	// Runtime errors shouldn't spew usage; flag-parse errors become usage
	// errors (exit 2) while cobra still prints them to stderr.
	rootCmd.SilenceUsage = true
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageErrorf("%w", err)
	})

	// PersistentFlags: Flags que están disponibles para este commando Y todos sus hijos.
	rootCmd.PersistentFlags().StringP("region", "r", "", "Región de AWS (sobrescribe AWS_REGION)")

	// Apply --region to AWS_REGION before any subcommand runs (flag > env).
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		region, _ := cmd.Flags().GetString("region")
		return applyRegion(region, os.Setenv)
	}

	rootCmd.Version = version

	// Registrar subcomandos
	rootCmd.AddCommand(prefixCmd)
	rootCmd.AddCommand(hasherCmd)
	rootCmd.AddCommand(approleCmd)
	rootCmd.AddCommand(configCmd)
}
