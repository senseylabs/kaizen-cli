package cmd

import (
	"fmt"

	"github.com/senseylabs/kaizen-cli/internal/auth"
	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored credentials",
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	store := auth.NewCredentialStore()

	if _, err := store.Load(); err != nil {
		fmt.Println("You are not logged in.")
		return nil
	}

	if err := store.Delete(); err != nil {
		return fmt.Errorf("failed to clear credentials: %w", err)
	}

	if err := cache.Clear(); err != nil {
		fmt.Printf("Warning: could not clear cache: %v\n", err)
	}

	fmt.Println("Logged out successfully.")
	return nil
}
