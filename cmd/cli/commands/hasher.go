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
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		username := strings.TrimSpace(args[0])
		cost, _ := cmd.Flags().GetInt("cost")

		if username == "" {
			fmt.Println("❌ Error: Username cannot be empty")
			os.Exit(1)
		}

		// Validate cost
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			fmt.Printf("❌ Error: cost must be between %d and %d\n", bcrypt.MinCost, bcrypt.MaxCost)
			os.Exit(1)
		}

		// Prompt for password
		fmt.Printf("🔐 Enter password for user '%s' (leave empty to generate): ", username)

		// Read password securely (hidden input)
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // New line after hidden input
		if err != nil {
			fmt.Printf("❌ Error: Failed to read password: %v\n", err)
			os.Exit(1)
		}

		password := strings.TrimSpace(string(passwordBytes))
		generatedPassword := false

		// If password is empty, generate a secure one
		if password == "" {
			fmt.Println("🎲 Generating secure password...")
			password, err = generateSecurePassword(10)
			if err != nil {
				fmt.Printf("❌ Error: Failed to generate password: %v\n", err)
				os.Exit(1)
			}
			generatedPassword = true
		}

		// Generate hash
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), cost)
		if err != nil {
			fmt.Printf("❌ Error: Failed to generate hash: %v\n", err)
			os.Exit(1)
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
			fmt.Printf("❌ Error: Failed to generate JSON: %v\n", err)
			os.Exit(1)
		}

		// Display result
		fmt.Println("\n✅ Credentials generated successfully!")

		// Show generated password (important!)
		if generatedPassword {
			fmt.Println(strings.Repeat("═", 70))
			fmt.Printf("🔑 Generated Password: %s\n", password)
			fmt.Println(strings.Repeat("═", 70))
			fmt.Println("⚠️  IMPORTANT: Save this password! It won't be shown again.")
			fmt.Println()
		}

		fmt.Println(strings.Repeat("═", 70))
		fmt.Println(string(jsonOutput))
		fmt.Println(strings.Repeat("═", 70))
	},
}

func init() {
	hasherCmd.Flags().IntP("cost", "c", bcrypt.DefaultCost, "Bcrypt cost factor (4-31)")
}
