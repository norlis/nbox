package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd representa el comando base cuando se llama sin subcomandos.
var rootCmd = &cobra.Command{
	Use:   "nbox-cli",
	Short: "Herramienta CLI de administración para nbox",
	Long: `NBox CLI es una herramienta para realizar tareas administrativas y de mantenimiento
sobre la infraestructura de NBox, como poblar configuraciones iniciales,
gestionar backups o validar integridad de datos.`,
	// Si quisieras que el comando raíz haga algo por sí mismo, descomenta esto:
	// Run: func(cmd *cobra.Command, args []string) { fmt.Println("Hola NBox") },
}

// Execute añade todos los comandos hijos al comando raíz y configura los flags.
// Esta función es llamada por main.main().
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
		return err
	}
	return nil
}

func init() {
	// Aquí defines tus flags y configuración.

	// PersistentFlags: Flags que están disponibles para este comando Y todos sus hijos.
	// Es ideal poner la 'region' aquí, ya que seed, list, delete, etc., necesitarán saber la región AWS.
	rootCmd.PersistentFlags().StringP("region", "r", "us-east-1", "Región de AWS por defecto")

	// Flags locales: Solo aplican al comando root (no se heredan).
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Registrar subcomandos
	rootCmd.AddCommand(seedCmd)
	rootCmd.AddCommand(hasherCmd)
}
