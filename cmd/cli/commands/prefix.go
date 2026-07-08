package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"nbox/internal/config"
	"nbox/internal/prefix"
)

// prefixCmd manages storage-routing config (kind="prefix_config") in the
// shared config table. Replaces the old `seed` command.
var prefixCmd = &cobra.Command{
	Use:   "prefix",
	Short: "Manage prefix (routing) configuration in the config table",
	Long:  `Create/list/delete prefix configurations. Table: --table or NBOX_CONFIG_TABLE_NAME.`,
}

func toBackends(ss []string) []prefix.StorageBackendType {
	result := make([]prefix.StorageBackendType, 0, len(ss))
	for _, s := range ss {
		result = append(result, prefix.StorageBackendType(s))
	}
	return result
}

// prefixOverrides carries only the flags the user actually set (nil = unchanged).
type prefixOverrides struct {
	typeDefault *string
	typeSecure  *string
	allowed     *[]string
}

// buildPrefixConfig merges overrides onto cur (nil ⇒ create with dynamodb
// default). Pure → unit-tested; the command keeps only I/O.
func buildPrefixConfig(id string, cur *prefix.Config, set prefixOverrides) prefix.Config {
	cfg := prefix.Config{Prefix: id, TypeDefault: prefix.BackendDynamoDB}
	if cur != nil {
		cfg = *cur
	}
	cfg.Prefix = id
	if set.typeDefault != nil {
		cfg.TypeDefault = prefix.StorageBackendType(*set.typeDefault)
	}
	if set.typeSecure != nil {
		cfg.TypeSecure = prefix.StorageBackendType(*set.typeSecure)
	}
	if set.allowed != nil {
		cfg.TypeAllowed = toBackends(*set.allowed)
	}
	return cfg
}

// changedOverrides reads only the flags cobra reports as Changed.
func changedOverrides(cmd *cobra.Command) prefixOverrides {
	var o prefixOverrides
	if cmd.Flags().Changed("type") {
		v, _ := cmd.Flags().GetString("type")
		o.typeDefault = &v
	}
	if cmd.Flags().Changed("secure") {
		v, _ := cmd.Flags().GetString("secure")
		o.typeSecure = &v
	}
	if cmd.Flags().Changed("allowed") {
		v, _ := cmd.Flags().GetStringSlice("allowed")
		o.allowed = &v
	}
	return o
}

var prefixUpsertCmd = &cobra.Command{
	Use:     "upsert",
	Short:   "Create/update a prefix (merge: only the provided flags change)",
	Example: "  nbox-cli prefix upsert --prefix=global/serverless --type=parameterstore --secure=parameterstore_secure --allowed=parameterstore_secure",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, _ := cmd.Flags().GetString("prefix")
		if p == "" {
			return usageErrorf("--prefix required")
		}
		id := strings.Trim(p, "/")
		set := changedOverrides(cmd)
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore, by string) error {
			cur, err := s.Get(cmd.Context(), config.KeyPrefixConfig.Kind, id)
			if err != nil {
				return err
			}
			var curCfg *prefix.Config
			if cur != nil {
				var c prefix.Config
				if err := json.Unmarshal([]byte(cur.Data), &c); err != nil {
					return fmt.Errorf("existing record for %q is corrupt: %w", id, err)
				}
				curCfg = &c
			}
			cfg := buildPrefixConfig(id, curCfg, set)
			data, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			if err := s.Upsert(cmd.Context(), config.KeyPrefixConfig.Kind, id, data, by); err != nil {
				return err
			}
			info("[ok] prefix %q saved (default=%s secure=%s allowed=%v)\n", id, cfg.TypeDefault, cfg.TypeSecure, cfg.TypeAllowed)
			return nil
		})
	},
}

var prefixListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List prefix configurations",
	Example: "  nbox-cli prefix list --json",
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		return withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore, _ string) error {
			recs, err := s.List(cmd.Context(), config.KeyPrefixConfig.Kind)
			if err != nil {
				return err
			}
			rendered, err := formatPrefixList(recs, asJSON)
			if err != nil {
				return err
			}
			out("%s", rendered)
			return nil
		})
	},
}

// formatPrefixList renders config records as a human table or a JSON array
// (machine-readable). Pure → unit-tested.
func formatPrefixList(recs []config.Record, asJSON bool) (string, error) {
	cfgs := make([]prefix.Config, 0, len(recs))
	for _, r := range recs {
		var c prefix.Config
		if err := json.Unmarshal([]byte(r.Data), &c); err != nil {
			return "", fmt.Errorf("decode %q: %w", r.ID, err)
		}
		cfgs = append(cfgs, c)
	}
	if asJSON {
		b, err := json.MarshalIndent(cfgs, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	var sb strings.Builder
	for _, c := range cfgs {
		fmt.Fprintf(&sb, "- %-30s default=%s secure=%s allowed=%v\n", c.Prefix, c.TypeDefault, c.TypeSecure, c.TypeAllowed)
	}
	return sb.String(), nil
}

var prefixRmCmd = &cobra.Command{
	Use:     "rm [prefix]",
	Short:   "Delete a prefix configuration",
	Example: "  nbox-cli prefix rm global/serverless --force",
	Args:    exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.Trim(args[0], "/")
		force, _ := cmd.Flags().GetBool("force")
		proceed, err := confirmDelete(force, stdinIsTTY(), os.Stdin, id)
		if err != nil {
			return err
		}
		if !proceed {
			info("cancelled\n")
			return nil
		}
		if err := withAdminStore(cmd.Context(), cmd, func(s *config.AdminStore, _ string) error {
			return s.Delete(cmd.Context(), config.KeyPrefixConfig.Kind, id)
		}); err != nil {
			return err
		}
		info("[ok] prefix %q deleted\n", id)
		return nil
	},
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// confirmDelete gates a destructive delete: --force skips it; non-interactive
// without --force is refused (fail-closed); interactive prompts y/N. Pure
// (in is injected) → unit-tested.
func confirmDelete(force, isTTY bool, in io.Reader, id string) (bool, error) {
	if force {
		return true, nil
	}
	if !isTTY {
		return false, usageErrorf("rm of %q is destructive: use --force in non-interactive mode", id)
	}
	info("delete prefix %q? [y/N]: ", id)
	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

// applyRegion forces AWS_REGION from an explicit --region (flag > env). Empty
// region is a no-op so the AWS SDK's own resolution is left intact. Pure → tested.
func applyRegion(region string, setenv func(string, string) error) error {
	if region == "" {
		return nil
	}
	return setenv("AWS_REGION", region)
}

func init() {
	prefixCmd.PersistentFlags().String("table", "", "DynamoDB table (default: NBOX_CONFIG_TABLE_NAME)")

	prefixUpsertCmd.Flags().String("prefix", "", "prefix (id) — REQUIRED")
	prefixUpsertCmd.Flags().String("type", "", "backend por defecto (dynamodb|parameterstore|parameterstore_secure)")
	prefixUpsertCmd.Flags().String("secure", "", "backend para entries secure")
	prefixUpsertCmd.Flags().StringSlice("allowed", nil, "allowed backends (list)")

	prefixListCmd.Flags().Bool("json", false, "JSON output (machine-readable)")
	prefixRmCmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")

	prefixCmd.AddCommand(prefixUpsertCmd, prefixListCmd, prefixRmCmd)
}
