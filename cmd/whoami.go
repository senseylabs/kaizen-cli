package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the currently authenticated user",
	RunE:  runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	body, err := c.Get("/users/me")
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	// JSON output mode
	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	// Human-readable output
	var resp client.APIResponse[client.User]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse user response: %w", err)
	}

	user := resp.Data
	fmt.Printf("Email: %s\n", user.Email)
	if user.Profile != nil {
		fmt.Printf("Name:  %s %s\n", user.Profile.FirstName, user.Profile.LastName)
	}
	fmt.Printf("ID:    %s\n", user.ID)
	if user.DefaultOrganizationID != nil {
		fmt.Printf("Org:   %s\n", *user.DefaultOrganizationID)
	}
	if len(user.Organizations) > 0 {
		fmt.Println("Organizations:")
		for _, org := range user.Organizations {
			fmt.Printf("  - %s (%s, role: %s)\n", org.Name, org.ID, org.Role)
		}
	}

	return nil
}
