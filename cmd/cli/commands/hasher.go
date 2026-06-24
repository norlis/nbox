package commands

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// generateSecurePassword generates a cryptographically secure random password.
func generateSecurePassword(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("error reading random bytes: %w", err)
	}

	// Encode to base64 URL-safe format (removes padding)
	password := base64.URLEncoding.EncodeToString(bytes)

	// Trim to desired length
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

var hasherCmd = &cobra.Command{
	Use:   "hasher [username]",
	Short: "Generate bcrypt hash and JSON structure for basic auth",
	Long: `Generate a bcrypt password hash and output the complete JSON structure
for NBOX basic authentication credentials.

The password is read securely from stdin without displaying it.
If you leave the password empty, a secure random password will be generated.
The output is a JSON object ready to use in NBOX_BASIC_AUTH_CREDENTIALS.

Examples:
  # Generate credentials with your own password
  nbox-cli hasher admin

  # Generate credentials with auto-generated secure password (press Enter)
  nbox-cli hasher admin

  # With custom bcrypt cost
  nbox-cli hasher admin --cost 12`,
	Args: exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := strings.TrimSpace(args[0])
		cost, _ := cmd.Flags().GetInt("cost")

		if username == "" {
			return usageErrorf("username cannot be empty")
		}

		// Validate cost
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return usageErrorf("cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}

		// Prompt + hidden input are diagnostics → stderr.
		info("🔐 Enter password for user '%s' (leave empty to generate): ", username)
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		infoln("") // New line after hidden input
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}

		password := strings.TrimSpace(string(passwordBytes))
		generatedPassword := false

		// If password is empty, generate a secure one
		if password == "" {
			infoln("🎲 Generating secure password...")
			password, err = generateSecurePassword(10)
			if err != nil {
				return fmt.Errorf("generate password: %w", err)
			}
			generatedPassword = true
		}

		// Generate hash
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), cost)
		if err != nil {
			return fmt.Errorf("generate hash: %w", err)
		}

		// Create credentials structure
		userInfo := map[string]any{
			"password": string(hashedPassword),
			"active":   true,
			"roles":    []string{},
		}

		credentials := map[string]any{
			username: userInfo,
		}

		// Marshal to JSON with indentation
		jsonOutput, err := json.MarshalIndent(credentials, "", "  ")
		if err != nil {
			return fmt.Errorf("generate JSON: %w", err)
		}

		// Banners + generated secret to stderr; the credentials JSON to stdout
		// so `hasher admin > creds.json` captures only the pasteable payload.
		infoln("\n✅ Credentials generated successfully!")
		if generatedPassword {
			infoln(strings.Repeat("═", 70))
			info("🔑 Generated Password: %s\n", password)
			infoln(strings.Repeat("═", 70))
			infoln("⚠️  IMPORTANT: Save this password! It won't be shown again.")
			infoln("")
		}

		outln(string(jsonOutput))
		return nil
	},
}

func init() {
	hasherCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")
}
