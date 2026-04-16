package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/auth"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Keycloak via password grant",
	Long:  "Authenticates with Keycloak using username and password. Stores credentials securely for future use.",
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	if cfgDevMode {
		fmt.Println("Using local development URLs")
	}

	// Check KAIZEN_TOKEN env var — skip login if set
	if os.Getenv("KAIZEN_TOKEN") != "" {
		fmt.Println("KAIZEN_TOKEN is set. Using token from environment.")
		return nil
	}

	var username, password string

	// Check env vars for non-interactive login
	username = os.Getenv("KAIZEN_USERNAME")
	password = os.Getenv("KAIZEN_PASSWORD")

	// Prompt if not provided via env
	if username == "" {
		fmt.Print("Username: ")
		if _, err := fmt.Scanln(&username); err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
	}

	if password == "" {
		fmt.Print("Password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(pwBytes)
		fmt.Println() // newline after masked input
	}

	// Perform password grant
	fmt.Println("Authenticating...")
	tokenResp, err := auth.PasswordGrant(cfgIssuer, cfgClientID, cfgClientSecret, username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	fmt.Println("Authentication successful!")

	// Call GET /users/me to get user info
	tempClient := client.NewKaizenClientWithToken(cfgAPIURL, cfgOrgID, tokenResp.AccessToken)

	userBody, err := tempClient.Get("/users/me")
	if err != nil {
		// If 404, try POST /users/me first (first-time sync)
		fmt.Println("Syncing user profile...")
		if _, postErr := tempClient.Post("/users/me", nil); postErr != nil {
			return fmt.Errorf("failed to sync user profile: %w", postErr)
		}
		userBody, err = tempClient.Get("/users/me")
		if err != nil {
			return fmt.Errorf("failed to get user info after sync: %w", err)
		}
	}

	var userResp client.APIResponse[client.User]
	if err := json.Unmarshal(userBody, &userResp); err != nil {
		return fmt.Errorf("failed to parse user response: %w", err)
	}

	user := userResp.Data

	// Resolve org ID from user's default organization
	orgID := cfgOrgID
	if orgID == "" && user.DefaultOrganizationID != nil {
		orgID = *user.DefaultOrganizationID
	}

	// Store credentials
	store := auth.NewCredentialStore()
	creds := auth.Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		IssuerURL:    cfgIssuer,
		ClientID:     cfgClientID,
		ClientSecret: cfgClientSecret,
		APIURL:       cfgAPIURL,
		OrgID:        orgID,
		UserID:       user.ID,
		DevMode:      cfgDevMode,
	}

	if err := store.Save(creds); err != nil {
		return fmt.Errorf("failed to store credentials: %w", err)
	}

	fmt.Println()
	fmt.Println("Login successful!")
	fmt.Printf("API:  %s\n", cfgAPIURL)
	fmt.Printf("User: %s\n", user.Email)
	if user.Profile != nil {
		fmt.Printf("Name: %s %s\n", user.Profile.FirstName, user.Profile.LastName)
	}
	if orgID != "" {
		fmt.Printf("Org:  %s\n", orgID)
	}

	return nil
}
