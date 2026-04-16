package cmd

import (
	"fmt"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage CLI cache",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached data",
	RunE:  runCacheClear,
}

func init() {
	rootCmd.AddCommand(cacheCmd)

	cacheCmd.AddCommand(cacheClearCmd)
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	if err := cache.Clear(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared")
	return nil
}
