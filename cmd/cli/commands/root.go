package commands

import (
	"os"

	"github.com/spf13/cobra"
)

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
	// Es ideal poner la 'region' aquí, ya que seed, list, delete, etc., necesitarán saber la región AWS.
	rootCmd.PersistentFlags().StringP("region", "r", "us-east-1", "Región de AWS por defecto")

	// Flags locales: Solo aplican al commando root (no se heredan).
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Registrar subcomandos
	rootCmd.AddCommand(seedCmd)
	rootCmd.AddCommand(hasherCmd)
	rootCmd.AddCommand(approleCmd)
	rootCmd.AddCommand(configCmd)
}
