package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/auth"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Keycloak via Device Authorization Grant",
	Long:  "Starts a Keycloak Device Authorization Grant flow. Opens your browser to complete authentication.",
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

	// Build DeviceFlow
	deviceFlow := auth.NewDeviceFlow(cfgIssuer, cfgClientID, "openid")

	// Step 1: Discover OIDC endpoints
	fmt.Println("Discovering Keycloak endpoints...")
	endpoints, err := deviceFlow.DiscoverEndpoints()
	if err != nil {
		return fmt.Errorf("failed to discover OIDC endpoints: %w", err)
	}

	// Step 2: Request device authorization
	deviceResp, err := deviceFlow.RequestDeviceAuthorization(endpoints.DeviceAuthorizationEndpoint)
	if err != nil {
		return fmt.Errorf("failed to start device authorization: %w", err)
	}

	// Step 3: Display instructions and try to open browser
	fmt.Println()
	fmt.Printf("Open this URL in your browser: %s\n", deviceResp.VerificationURIComplete)
	fmt.Printf("Enter code: %s\n", deviceResp.UserCode)
	fmt.Println()

	if deviceResp.VerificationURIComplete != "" {
		openBrowser(deviceResp.VerificationURIComplete)
	}

	// Step 4: Poll for token
	fmt.Println("Waiting for authentication...")
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}
	expiresAt := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)

	tokenResp, err := deviceFlow.PollForToken(endpoints.TokenEndpoint, deviceResp.DeviceCode, interval, expiresAt)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Authentication successful!")

	// Step 5: Call GET /users/me to get user info
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

	// Resolve org ID: prefer user's default org unless --org was explicitly passed
	orgID := ""
	if cmd.Flags().Changed("org") {
		orgID = cfgOrgID
	} else if user.DefaultOrganizationID != nil {
		orgID = *user.DefaultOrganizationID
	}

	// Step 6: Store credentials
	store := auth.NewCredentialStore()
	creds := auth.Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		IssuerURL:    cfgIssuer,
		ClientID:     cfgClientID,
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser automatically. Please open the URL manually.\n")
	}
}
