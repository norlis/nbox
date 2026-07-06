package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"
	"nbox/cmd/cli/bootstrap"
	"nbox/internal/auth/approle"
	"nbox/internal/auth/awssts"
	"nbox/internal/config"
)

// configCmd is the parent for config-table (DynamoDB) operations.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the dynamic config table (DynamoDB)",
	Long: `Create/update/delete auth entities in the config table without restarting
the services: user (basic auth), aws-sts (M2M) and approle (M2M).

The table is resolved from --table or NBOX_CONFIG_TABLE_NAME.`,
}

// tableName resolves the table from --table or NBOX_CONFIG_TABLE_NAME.
func tableName(cmd *cobra.Command) string {
	t, _ := cmd.Flags().GetString("table")
	if t == "" {
		t = os.Getenv("NBOX_CONFIG_TABLE_NAME")
	}
	return t
}

// withAdminStore runs a minimal fx app and hands a ready AdminStore to fn,
// returning any error so the calling RunE maps it to an exit code. fx.Invoke
// runs synchronously during Start, so opErr is set by the time Start returns.
func withAdminStore(ctx context.Context, cmd *cobra.Command, fn func(*config.AdminStore) error) error {
	table := tableName(cmd)
	if table == "" {
		return usageErrorf("set --table or NBOX_CONFIG_TABLE_NAME")
	}
	var opErr error
	app := fx.New(
		fx.NopLogger,
		bootstrap.CommonModules,
		fx.Invoke(func(client *dynamodb.Client, sd fx.Shutdowner) {
			defer func() { _ = sd.Shutdown() }()
			opErr = fn(config.NewAdminStore(client, table))
		}),
	)
	if err := app.Start(ctx); err != nil {
		return err
	}
	return opErr
}

// ---- user (basic auth) ----

type basicAuthData struct {
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
	Status   string   `json:"status"`
}

// Pure builders (SoC + testability): build the JSON of one domain entity,
// no side-effects (bcrypt/uuid happen in the command).
func buildBasicAuthData(passwordHash string, roles []string, status string) []byte {
	// #nosec G117 -- "password" holds a bcrypt hash (not plaintext); the json key
	// is fixed by the basic-auth parser and is persisted intentionally.
	b, _ := json.Marshal(basicAuthData{Password: passwordHash, Roles: roles, Status: status})
	return b
}

func buildARNMapping(arn, name string, roles []string, status string) []byte {
	b, _ := json.Marshal(awssts.ARNMapping{ARN: arn, Name: name, Roles: roles, Status: awssts.Status(status)})
	return b
}

func buildAppRole(roleID, name string, roles, cidrs []string, secretHash string, createdAt time.Time) []byte {
	b, _ := json.Marshal(approle.Role{
		ID: roleID, Name: name, Roles: roles, AllowedCIDRs: cidrs,
		SecretHashes: []approle.SecretHash{{Hash: secretHash, CreatedAt: createdAt}},
		Status:       approle.StatusActive,
	})
	return b
}

// hashPassword bcrypts --password (required) at --cost.
func hashPassword(cmd *cobra.Command) (string, error) {
	password, _ := cmd.Flags().GetString("password")
	if password == "" {
		return "", errors.New("--password required")
	}
	cost, _ := cmd.Flags().GetInt("cost")
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(h), err
}

// printApproleCreds writes the role credentials to stderr (secret, shown once).
func printApproleCreds(roleID, secretID string) {
	infoln(strings.Repeat("=", 70))
	info("role_id:    %s\n", roleID)
	info("secret_id:  %s\n", secretID)
	infoln(strings.Repeat("=", 70))
	infoln("Distribute the secret_id via a secure channel. It will NOT be shown again.")
}

var configUserUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Create/update a user (merge: only the provided flags change)",
	Example: "  nbox-cli config user upsert --username admin --password s3cr3t --roles admin\n" +
		"  nbox-cli config user upsert --username admin --password s3cr3t --roles admin --emit-env",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("username")
		if username == "" {
			return usageErrorf("--username required")
		}

		// Local: build a fresh entity from flags and print the env var; no table.
		if emit, _ := cmd.Flags().GetBool("emit-env"); emit {
			hash, err := hashPassword(cmd)
			if err != nil {
				return err
			}
			roles, _ := cmd.Flags().GetStringSlice("roles")
			status, _ := cmd.Flags().GetString("status")
			emitEnv(config.KeyBasicAuth, username, buildBasicAuthData(hash, roles, status))
			return nil
		}

		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			d := basicAuthData{Status: "active"}
			cur, err := s.Get(cmd.Context(), config.KeyBasicAuth.Kind, username)
			if err != nil {
				return err
			}
			if cur != nil {
				_ = json.Unmarshal([]byte(cur.Data), &d)
			}
			if cur == nil || cmd.Flags().Changed("password") {
				hash, herr := hashPassword(cmd)
				if herr != nil {
					return herr
				}
				d.Password = hash
			}
			if cmd.Flags().Changed("roles") {
				d.Roles, _ = cmd.Flags().GetStringSlice("roles")
			}
			if cmd.Flags().Changed("status") {
				d.Status, _ = cmd.Flags().GetString("status")
			}
			if err := s.Upsert(cmd.Context(), config.KeyBasicAuth.Kind, username, buildBasicAuthData(d.Password, d.Roles, d.Status), "nbox-cli"); err != nil {
				return err
			}
			info("[ok] user %q saved (roles=%v status=%s)\n", username, d.Roles, d.Status)
			return nil
		})
	},
}

var configUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "List basic auth users (hashes not exposed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyBasicAuth.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var d basicAuthData
				_ = json.Unmarshal([]byte(r.Data), &d)
				out("- %-20s roles=%v status=%s updated=%s\n", r.ID, d.Roles, d.Status, r.UpdatedAt)
			}
			return nil
		})
	},
}

var configUserRmCmd = &cobra.Command{
	Use:   "rm [username]",
	Short: "Delete a basic auth user",
	Args:  exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyBasicAuth.Kind, args[0])
		}); err != nil {
			return err
		}
		info("[ok] user %q deleted\n", args[0])
		return nil
	},
}

// ---- arn (AWS-STS) ----

var configAwsStsUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Create/update an ARN mapping (merge: only the provided flags change)",
	Example: "  nbox-cli config aws-sts upsert --arn arn:aws:iam::123:role/foo --roles entrypushd\n" +
		"  nbox-cli config aws-sts upsert --arn arn:aws:iam::123:role/foo --roles entrypushd --emit-env",
	RunE: func(cmd *cobra.Command, args []string) error {
		arn, _ := cmd.Flags().GetString("arn")
		if arn == "" {
			return usageErrorf("--arn required (canonical iam role form)")
		}
		if emit, _ := cmd.Flags().GetBool("emit-env"); emit {
			name, _ := cmd.Flags().GetString("name")
			roles, _ := cmd.Flags().GetStringSlice("roles")
			status, _ := cmd.Flags().GetString("status")
			emitEnv(config.KeyARNMap, arn, buildARNMapping(arn, name, roles, status))
			return nil
		}
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			// Merge onto the existing mapping: only passed flags override it.
			m := awssts.ARNMapping{ARN: arn, Status: awssts.StatusActive}
			cur, err := s.Get(cmd.Context(), config.KeyARNMap.Kind, arn)
			if err != nil {
				return err
			}
			if cur != nil {
				_ = json.Unmarshal([]byte(cur.Data), &m)
			}
			m.ARN = arn
			if cmd.Flags().Changed("name") {
				m.Name, _ = cmd.Flags().GetString("name")
			}
			if cmd.Flags().Changed("roles") {
				m.Roles, _ = cmd.Flags().GetStringSlice("roles")
			}
			if cmd.Flags().Changed("status") {
				st, _ := cmd.Flags().GetString("status")
				m.Status = awssts.Status(st)
			}
			data := buildARNMapping(m.ARN, m.Name, m.Roles, string(m.Status))
			if err := s.Upsert(cmd.Context(), config.KeyARNMap.Kind, arn, data, "nbox-cli"); err != nil {
				return err
			}
			info("[ok] arn %q saved (roles=%v status=%s)\n", arn, m.Roles, m.Status)
			return nil
		})
	},
}

var configAwsStsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ARN mappings",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyARNMap.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var m awssts.ARNMapping
				_ = json.Unmarshal([]byte(r.Data), &m)
				out("- %-50s name=%s roles=%v status=%s\n", r.ID, m.Name, m.Roles, m.Status)
			}
			return nil
		})
	},
}

var configAwsStsRmCmd = &cobra.Command{
	Use:   "rm [arn]",
	Short: "Delete an ARN mapping",
	Args:  exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyARNMap.Kind, args[0])
		}); err != nil {
			return err
		}
		info("[ok] arn %q deleted\n", args[0])
		return nil
	},
}

// ---- approle (M2M) ----

var configApproleGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate and persist an AppRole (role_id + secret_id)",
	Example: "  nbox-cli config approle generate --name watcher --roles entrypushd --cidrs 10.0.0.0/8\n" +
		"  nbox-cli config approle generate --name watcher --roles entrypushd --emit-env",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		roles, _ := cmd.Flags().GetStringSlice("roles")
		cidrs, _ := cmd.Flags().GetStringSlice("cidrs")
		cost, _ := cmd.Flags().GetInt("cost")
		if name == "" {
			return usageErrorf("--name required")
		}
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return usageErrorf("cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}
		roleID := uuid.NewString()
		secretID := uuid.NewString()
		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			return fmt.Errorf("bcrypt: %w", err)
		}
		data := buildAppRole(roleID, name, roles, cidrs, string(hash), time.Now().UTC())

		if emit, _ := cmd.Flags().GetBool("emit-env"); emit {
			emitEnv(config.KeyAppRole, roleID, data)
			printApproleCreds(roleID, secretID)
			return nil
		}
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Upsert(cmd.Context(), config.KeyAppRole.Kind, roleID, data, "nbox-cli")
		}); err != nil {
			return err
		}
		printApproleCreds(roleID, secretID)
		return nil
	},
}

var configApproleRotateCmd = &cobra.Command{
	Use:   "rotate-secret [role_id]",
	Short: "Rotate an AppRole secret (appends a new secret_hash, zero-downtime)",
	Example: "  nbox-cli config approle rotate-secret <role_id>\n" +
		"  nbox-cli config approle rotate-secret --emit-env",
	Args: maxArgs(1),
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
		newHash := approle.SecretHash{Hash: string(hash), CreatedAt: time.Now().UTC()}

		// Local: no role to read — print the hash fragment to append + secret_id.
		if emit, _ := cmd.Flags().GetBool("emit-env"); emit {
			// Reject a role_id here: emit-env never touches the table, so
			// accepting one would wrongly imply that role was rotated.
			if len(args) > 0 {
				return usageErrorf("--emit-env does not accept a role_id (nothing is persisted; it only prints the fragment)")
			}
			frag, err := json.Marshal(newHash)
			if err != nil {
				return err
			}
			outln(string(frag))
			info("secret_id: %s  (append the hash above to the role's secret_hashes in NBOX_APPROLE_ROLES)\n", secretID)
			return nil
		}

		if len(args) != 1 {
			return usageErrorf("role_id required (or use --emit-env)")
		}
		roleID := args[0]
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			cur, err := s.Get(cmd.Context(), config.KeyAppRole.Kind, roleID)
			if err != nil {
				return err
			}
			if cur == nil {
				return fmt.Errorf("approle %q not found", roleID)
			}
			var role approle.Role
			if err := json.Unmarshal([]byte(cur.Data), &role); err != nil {
				return fmt.Errorf("approle %q is corrupt: %w", roleID, err)
			}
			role.SecretHashes = append(role.SecretHashes, newHash)
			data, mErr := json.Marshal(role)
			if mErr != nil {
				return mErr
			}
			return s.Upsert(cmd.Context(), config.KeyAppRole.Kind, roleID, data, "nbox-cli")
		}); err != nil {
			return err
		}
		info("[ok] approle %q rotated (new secret_hash appended; the old one remains valid)\n", roleID)
		info("secret_id: %s  (distribute securely; remove the old hash after the grace period)\n", secretID)
		return nil
	},
}

var configApproleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List AppRoles (secret_hashes not exposed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyAppRole.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var role approle.Role
				_ = json.Unmarshal([]byte(r.Data), &role)
				out("- %s name=%s roles=%v hashes=%d status=%s\n",
					role.ID, role.Name, role.Roles, len(role.SecretHashes), role.Status)
			}
			return nil
		})
	},
}

var configApproleRmCmd = &cobra.Command{
	Use:   "rm [role_id]",
	Short: "Delete an AppRole",
	Args:  exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyAppRole.Kind, args[0])
		}); err != nil {
			return err
		}
		info("[ok] approle %q deleted\n", args[0])
		return nil
	},
}

func init() {
	configCmd.PersistentFlags().String("table", "", "DynamoDB table (default: NBOX_CONFIG_TABLE_NAME)")

	configUser := &cobra.Command{Use: "user", Short: "Basic auth users"}
	configUserUpsertCmd.Flags().String("username", "", "username (id)")
	configUserUpsertCmd.Flags().String("password", "", "plaintext password (bcrypt-hashed)")
	configUserUpsertCmd.Flags().StringSlice("roles", nil, "OPA roles")
	configUserUpsertCmd.Flags().String("status", "active", "active|disabled")
	configUserUpsertCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost (4-31)")
	configUserUpsertCmd.Flags().Bool("emit-env", false, "print export NBOX_BASIC_AUTH_CREDENTIALS instead of persisting (local dev)")
	configUser.AddCommand(configUserUpsertCmd, configUserListCmd, configUserRmCmd)

	configAwsSts := &cobra.Command{Use: "aws-sts", Short: "AWS-STS M2M auth (ARN→roles mappings)"}
	configAwsStsUpsertCmd.Flags().String("arn", "", "canonical iam role ARN (id)")
	configAwsStsUpsertCmd.Flags().String("name", "", "human-readable name")
	configAwsStsUpsertCmd.Flags().StringSlice("roles", nil, "OPA roles")
	configAwsStsUpsertCmd.Flags().String("status", "active", "active|disabled")
	configAwsStsUpsertCmd.Flags().Bool("emit-env", false, "print export NBOX_AWS_ARN_MAP instead of persisting (local dev)")
	configAwsSts.AddCommand(configAwsStsUpsertCmd, configAwsStsListCmd, configAwsStsRmCmd)

	configApprole := &cobra.Command{Use: "approle", Short: "AppRoles M2M"}
	configApproleGenerateCmd.Flags().String("name", "", "role name")
	configApproleGenerateCmd.Flags().StringSlice("roles", nil, "OPA roles")
	configApproleGenerateCmd.Flags().StringSlice("cidrs", nil, "allowed CIDRs (optional)")
	configApproleGenerateCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost (4-31)")
	configApproleGenerateCmd.Flags().Bool("emit-env", false, "print export NBOX_APPROLE_ROLES instead of persisting (local dev)")
	configApproleRotateCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost (4-31)")
	configApproleRotateCmd.Flags().Bool("emit-env", false, "print the secret_hash to append instead of persisting (local dev)")
	configApprole.AddCommand(configApproleGenerateCmd, configApproleRotateCmd, configApproleListCmd, configApproleRmCmd)

	configCmd.AddCommand(configUser, configAwsSts, configApprole)
}
