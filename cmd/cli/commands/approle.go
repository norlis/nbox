package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// approleCmd is the parent command for AppRole credential management.
var approleCmd = &cobra.Command{
	Use:   "approle",
	Short: "Manage AppRole credentials for nbox M2M auth",
	Long: `Generate and rotate AppRole credentials used to authenticate
service accounts (agents) against entrypushd via gRPC metadata:
  authorization: AppRole <base64(JSON{role_id, secret_id})>

Two subcommands:
  generate       Create a brand-new role with role_id + secret_id + hash.
  rotate-secret  Generate a new secret_id + hash for an existing role.`,
}

var approleGenerateCmd = &cobra.Command{
	Use:   "generate [role-name]",
	Short: "Generate a new AppRole (role_id + secret_id + bcrypt hash)",
	Long: `Generate a new AppRole. Outputs:

  - role_id (UUID v4) — stable identifier for the role
  - secret_id (UUID v4) — the credential to distribute to the client
  - JSON entry — copy/paste into NBOX_APPROLE_ROLES

Example:
  nbox-cli approle generate entrypushd-service --opa-role entrypushd`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := strings.TrimSpace(args[0])
		if name == "" {
			fmt.Println("Error: role name cannot be empty")
			os.Exit(1)
		}

		opaRoles, _ := cmd.Flags().GetStringSlice("opa-role")
		cost, _ := cmd.Flags().GetInt("cost")

		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			fmt.Printf("Error: cost must be between %d and %d\n", bcrypt.MinCost, bcrypt.MaxCost)
			os.Exit(1)
		}

		roleID := uuid.NewString()
		secretID := uuid.NewString()

		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			fmt.Printf("Error: bcrypt failed: %v\n", err)
			os.Exit(1)
		}

		entry := map[string]any{
			"id":    roleID,
			"name":  name,
			"roles": opaRoles,
			"secret_hashes": []map[string]any{
				{
					"hash":       string(hash),
					"created_at": time.Now().UTC().Format(time.RFC3339),
				},
			},
			"status": "active",
		}

		jsonOut, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			fmt.Printf("Error: marshal: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\nAppRole generated successfully")
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("role_id:    %s\n", roleID)
		fmt.Printf("secret_id:  %s\n", secretID)
		fmt.Println(strings.Repeat("=", 70))
		fmt.Println("Distribute secret_id to the client via a secure channel.")
		fmt.Println("It will NOT be shown again.")
		fmt.Println()
		fmt.Println("Add this entry to NBOX_APPROLE_ROLES:")
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println(string(jsonOut))
		fmt.Println(strings.Repeat("-", 70))
	},
}

var approleRotateSecretCmd = &cobra.Command{
	Use:   "rotate-secret",
	Short: "Generate a new secret_id + hash for an existing role",
	Long: `Generate a new secret_id and its bcrypt hash. Append the printed
SecretHash entry to the role's secret_hashes array in NBOX_APPROLE_ROLES.

After clients have migrated to the new secret_id, remove the old hash
to complete the rotation.`,
	Run: func(cmd *cobra.Command, args []string) {
		cost, _ := cmd.Flags().GetInt("cost")
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			fmt.Printf("Error: cost must be between %d and %d\n", bcrypt.MinCost, bcrypt.MaxCost)
			os.Exit(1)
		}

		secretID := uuid.NewString()
		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			fmt.Printf("Error: bcrypt failed: %v\n", err)
			os.Exit(1)
		}

		entry := map[string]any{
			"hash":       string(hash),
			"created_at": time.Now().UTC().Format(time.RFC3339),
		}
		jsonOut, _ := json.MarshalIndent(entry, "", "  ")

		fmt.Println("\nNew secret_id generated")
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("secret_id:  %s\n", secretID)
		fmt.Println(strings.Repeat("=", 70))
		fmt.Println("Distribute secret_id via secure channel. Not shown again.")
		fmt.Println()
		fmt.Println("Append this SecretHash entry to the role's secret_hashes array:")
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println(string(jsonOut))
		fmt.Println(strings.Repeat("-", 70))
	},
}

func init() {
	approleGenerateCmd.Flags().StringSlice("opa-role", nil, "OPA role(s) to grant the AppRole (can be repeated)")
	approleGenerateCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")

	approleRotateSecretCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")

	approleCmd.AddCommand(approleGenerateCmd)
	approleCmd.AddCommand(approleRotateSecretCmd)
}
