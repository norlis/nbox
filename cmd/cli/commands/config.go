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
	Short: "Administrator la tabla config dinámica (DynamoDB)",
	Long: `Crea/actualiza/borra entidades de auth en la tabla config sin reiniciar
los servicios: user (basic auth), aws-sts (M2M) y approle (M2M).

La tabla se toma de --table o de NBOX_CONFIG_TABLE_NAME.`,
}

// tableName resolves the table from --table or NBOX_CONFIG_TABLE_NAME.
func tableName(cmd *cobra.Command) string {
	t, _ := cmd.Flags().GetString("table")
	if t == "" {
		t = os.Getenv("NBOX_CONFIG_TABLE_NAME")
	}
	return t
}

// withAdminStore runs a minimal fx app and hands over a ready AdminStore.
func withAdminStore(ctx context.Context, cmd *cobra.Command, fn func(*config.AdminStore) error) {
	table := tableName(cmd)
	if table == "" {
		printError("config", errors.New("set --table o NBOX_CONFIG_TABLE_NAME"))
		os.Exit(1)
	}
	app := fx.New(
		fx.NopLogger,
		bootstrap.CommonModules,
		fx.Invoke(func(client *dynamodb.Client, sd fx.Shutdowner) {
			defer func() { _ = sd.Shutdown() }()
			if err := fn(config.NewAdminStore(client, table)); err != nil {
				printError("config operation", err)
				os.Exit(1)
			}
		}),
	)
	if err := app.Start(ctx); err != nil {
		printError(err.Error(), err)
		os.Exit(1)
	}
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

var configUserUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Crear/actualizar un usuario (merge: solo cambia los flags provistos)",
	Run: func(cmd *cobra.Command, args []string) {
		username, _ := cmd.Flags().GetString("username")
		if username == "" {
			printError("config user", errors.New("--username requerido"))
			os.Exit(1)
		}
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			// Merge onto the existing entity: only flags actually passed override
			// it, so e.g. changing just the password preserves roles/status.
			d := basicAuthData{Status: "active"}
			cur, err := s.Get(cmd.Context(), config.KeyBasicAuth.Kind, username)
			if err != nil {
				return err
			}
			if cur != nil {
				_ = json.Unmarshal([]byte(cur.Data), &d)
			}
			// Password is required when creating; re-hashed only if --password is given.
			if cur == nil || cmd.Flags().Changed("password") {
				password, _ := cmd.Flags().GetString("password")
				if password == "" {
					return errors.New("--password requerido para un usuario nuevo")
				}
				cost, _ := cmd.Flags().GetInt("cost")
				hash, herr := bcrypt.GenerateFromPassword([]byte(password), cost)
				if herr != nil {
					return herr
				}
				d.Password = string(hash)
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
			fmt.Printf("[ok] user %q guardado (roles=%v status=%s)\n", username, d.Roles, d.Status)
			return nil
		})
	},
}

var configUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "Listar usuarios de basic auth (sin exponer hashes)",
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyBasicAuth.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var d basicAuthData
				_ = json.Unmarshal([]byte(r.Data), &d)
				fmt.Printf("- %-20s roles=%v status=%s updated=%s\n", r.ID, d.Roles, d.Status, r.UpdatedAt)
			}
			return nil
		})
	},
}

var configUserRmCmd = &cobra.Command{
	Use:   "rm [username]",
	Short: "Borrar un usuario de basic auth",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyBasicAuth.Kind, args[0])
		})
		fmt.Printf("[ok] user %q borrado\n", args[0])
	},
}

// ---- arn (AWS-STS) ----

var configAwsStsUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Crear/actualizar un mapeo ARN (merge: solo cambia los flags provistos)",
	Run: func(cmd *cobra.Command, args []string) {
		arn, _ := cmd.Flags().GetString("arn")
		if arn == "" {
			printError("config aws-sts", errors.New("--arn requerido (forma canónica iam role)"))
			os.Exit(1)
		}
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
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
			fmt.Printf("[ok] arn %q guardado (roles=%v status=%s)\n", arn, m.Roles, m.Status)
			return nil
		})
	},
}

var configAwsStsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Listar mapeos ARN",
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyARNMap.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var m awssts.ARNMapping
				_ = json.Unmarshal([]byte(r.Data), &m)
				fmt.Printf("- %-50s name=%s roles=%v status=%s\n", r.ID, m.Name, m.Roles, m.Status)
			}
			return nil
		})
	},
}

var configAwsStsRmCmd = &cobra.Command{
	Use:   "rm [arn]",
	Short: "Borrar un mapeo ARN",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyARNMap.Kind, args[0])
		})
		fmt.Printf("[ok] arn %q borrado\n", args[0])
	},
}

// ---- approle (M2M) ----

var configApproleGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generar y persistir un AppRole (role_id + secret_id)",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		roles, _ := cmd.Flags().GetStringSlice("roles")
		cidrs, _ := cmd.Flags().GetStringSlice("cidrs")
		cost, _ := cmd.Flags().GetInt("cost")
		if name == "" {
			printError("config approle", errors.New("--name requerido"))
			os.Exit(1)
		}
		roleID := uuid.NewString()
		secretID := uuid.NewString()
		hash, err := bcrypt.GenerateFromPassword([]byte(secretID), cost)
		if err != nil {
			printError("bcrypt", err)
			os.Exit(1)
		}
		data := buildAppRole(roleID, name, roles, cidrs, string(hash), time.Now().UTC())
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Upsert(cmd.Context(), config.KeyAppRole.Kind, roleID, data, "nbox-cli")
		})
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("role_id:    %s\n", roleID)
		fmt.Printf("secret_id:  %s\n", secretID)
		fmt.Println(strings.Repeat("=", 70))
		fmt.Println("Distribuí el secret_id por canal seguro. NO se vuelve a mostrar.")
	},
}

var configApproleListCmd = &cobra.Command{
	Use:   "list",
	Short: "Listar AppRoles (sin exponer secret_hashes)",
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			recs, err := s.List(cmd.Context(), config.KeyAppRole.Kind)
			if err != nil {
				return err
			}
			for _, r := range recs {
				var role approle.Role
				_ = json.Unmarshal([]byte(r.Data), &role)
				fmt.Printf("- %s name=%s roles=%v hashes=%d status=%s\n",
					role.ID, role.Name, role.Roles, len(role.SecretHashes), role.Status)
			}
			return nil
		})
	},
}

var configApproleRmCmd = &cobra.Command{
	Use:   "rm [role_id]",
	Short: "Borrar un AppRole",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore) error {
			return s.Delete(cmd.Context(), config.KeyAppRole.Kind, args[0])
		})
		fmt.Printf("[ok] approle %q borrado\n", args[0])
	},
}

func init() {
	configCmd.PersistentFlags().String("table", "", "Tabla DynamoDB (default: NBOX_CONFIG_TABLE_NAME)")

	configUser := &cobra.Command{Use: "user", Short: "Usuarios de basic auth"}
	configUserUpsertCmd.Flags().String("username", "", "username (id)")
	configUserUpsertCmd.Flags().String("password", "", "password en claro (se bcryptea)")
	configUserUpsertCmd.Flags().StringSlice("roles", nil, "roles OPA")
	configUserUpsertCmd.Flags().String("status", "active", "active|disabled")
	configUserUpsertCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost (4-31)")
	configUser.AddCommand(configUserUpsertCmd, configUserListCmd, configUserRmCmd)

	configAwsSts := &cobra.Command{Use: "aws-sts", Short: "Auth AWS-STS M2M (mapeos ARN→roles)"}
	configAwsStsUpsertCmd.Flags().String("arn", "", "ARN canónico iam role (id)")
	configAwsStsUpsertCmd.Flags().String("name", "", "nombre legible")
	configAwsStsUpsertCmd.Flags().StringSlice("roles", nil, "roles OPA")
	configAwsStsUpsertCmd.Flags().String("status", "active", "active|disabled")
	configAwsSts.AddCommand(configAwsStsUpsertCmd, configAwsStsListCmd, configAwsStsRmCmd)

	configApprole := &cobra.Command{Use: "approle", Short: "AppRoles M2M"}
	configApproleGenerateCmd.Flags().String("name", "", "nombre del role")
	configApproleGenerateCmd.Flags().StringSlice("roles", nil, "roles OPA")
	configApproleGenerateCmd.Flags().StringSlice("cidrs", nil, "CIDRs permitidos (opcional)")
	configApproleGenerateCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost (4-31)")
	configApprole.AddCommand(configApproleGenerateCmd, configApproleListCmd, configApproleRmCmd)

	configCmd.AddCommand(configUser, configAwsSts, configApprole)
}
