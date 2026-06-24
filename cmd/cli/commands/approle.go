package commands

import (
	"encoding/json"
	"fmt"
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
	Args: exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return usageErrorf("role name cannot be empty")
		}

		opaRoles, _ := cmd.Flags().GetStringSlice("opa-role")
		cost, _ := cmd.Flags().GetInt("cost")

		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return usageErrorf("cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}

		roleID := uuid.NewString()
		secretID := uuid.NewString()

		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			return fmt.Errorf("bcrypt: %w", err)
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
			return fmt.Errorf("marshal: %w", err)
		}

		// Diagnostics + secret to stderr; the pasteable JSON entry to stdout so
		// `approle generate x > role.json` captures only the entry.
		infoln("\nAppRole generated successfully")
		infoln(strings.Repeat("=", 70))
		info("role_id:    %s\n", roleID)
		info("secret_id:  %s\n", secretID)
		infoln(strings.Repeat("=", 70))
		infoln("Distribute secret_id to the client via a secure channel.")
		infoln("It will NOT be shown again.")
		infoln("")
		infoln("Add this entry to NBOX_APPROLE_ROLES:")

		outln(string(jsonOut))
		return nil
	},
}

var approleRotateSecretCmd = &cobra.Command{
	Use:   "rotate-secret",
	Short: "Generate a new secret_id + hash for an existing role",
	Long: `Generate a new secret_id and its bcrypt hash. Append the printed
SecretHash entry to the role's secret_hashes array in NBOX_APPROLE_ROLES.

After clients have migrated to the new secret_id, remove the old hash
to complete the rotation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cost, _ := cmd.Flags().GetInt("cost")
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return usageErrorf("cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}

		secretID := uuid.NewString()
		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			return fmt.Errorf("bcrypt: %w", err)
		}

		entry := map[string]any{
			"hash":       string(hash),
			"created_at": time.Now().UTC().Format(time.RFC3339),
		}
		jsonOut, _ := json.MarshalIndent(entry, "", "  ")

		// Secret + guidance to stderr; the SecretHash entry to stdout.
		infoln("\nNew secret_id generated")
		infoln(strings.Repeat("=", 70))
		info("secret_id:  %s\n", secretID)
		infoln(strings.Repeat("=", 70))
		infoln("Distribute secret_id via secure channel. Not shown again.")
		infoln("")
		infoln("Append this SecretHash entry to the role's secret_hashes array:")

		outln(string(jsonOut))
		return nil
	},
}

func init() {
	approleGenerateCmd.Flags().StringSlice("opa-role", nil, "OPA role(s) to grant the AppRole (can be repeated)")
	approleGenerateCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")

	approleRotateSecretCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")

	approleCmd.AddCommand(approleGenerateCmd)
	approleCmd.AddCommand(approleRotateSecretCmd)
}
