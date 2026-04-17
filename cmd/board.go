package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Manage Kaizen boards",
}

var boardListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all boards",
	RunE:  runBoardList,
}

var boardGetCmd = &cobra.Command{
	Use:   "get <board>",
	Short: "Get a board by name or ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardGet,
}

var boardCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new board",
	RunE:  runBoardCreate,
}

var boardUpdateCmd = &cobra.Command{
	Use:   "update <board>",
	Short: "Update an existing board",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardUpdate,
}

var boardDeleteCmd = &cobra.Command{
	Use:   "delete <board>",
	Short: "Delete a board",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardDelete,
}

var boardRestoreCmd = &cobra.Command{
	Use:   "restore <board>",
	Short: "Restore a deleted board",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardRestore,
}

var boardChildrenCmd = &cobra.Command{
	Use:   "children",
	Short: "Manage board children",
}

var boardChildrenAddCmd = &cobra.Command{
	Use:   "add <board>",
	Short: "Add child boards to a parent board",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardChildrenAdd,
}

var boardRelatedCmd = &cobra.Command{
	Use:   "related <board>",
	Short: "List related boards",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardRelated,
}

var boardSetDefaultCmd = &cobra.Command{
	Use:   "set-default <board name>",
	Short: "Set the default board in config",
	Args:  cobra.ExactArgs(1),
	RunE:  runBoardSetDefault,
}

func init() {
	rootCmd.AddCommand(boardCmd)

	boardCmd.AddCommand(boardListCmd)
	boardCmd.AddCommand(boardGetCmd)
	boardCmd.AddCommand(boardCreateCmd)
	boardCmd.AddCommand(boardUpdateCmd)
	boardCmd.AddCommand(boardDeleteCmd)
	boardCmd.AddCommand(boardRestoreCmd)
	boardCmd.AddCommand(boardChildrenCmd)
	boardCmd.AddCommand(boardRelatedCmd)
	boardCmd.AddCommand(boardSetDefaultCmd)

	boardChildrenCmd.AddCommand(boardChildrenAddCmd)

	// Flags
	boardListCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh data")

	boardCreateCmd.Flags().String("name", "", "Board name (required)")
	boardCreateCmd.Flags().String("description", "", "Board description")
	boardCreateCmd.Flags().String("key", "", "Board key/prefix (required)")
	boardCreateCmd.Flags().String("type", "", "Board type")

	boardUpdateCmd.Flags().String("name", "", "Board name")
	boardUpdateCmd.Flags().String("description", "", "Board description")
	boardUpdateCmd.Flags().String("key", "", "Board key/prefix")
	boardUpdateCmd.Flags().String("type", "", "Board type")

	boardChildrenAddCmd.Flags().String("child-ids", "", "Comma-separated child board IDs")
}

func runBoardList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)
	refresh, _ := cmd.Flags().GetBool("refresh")

	boardsTTL := 30 * time.Minute
	cacheKey := "boards"

	// Try cache unless --refresh
	if !refresh {
		if cached, ok := cache.Get(cacheKey, boardsTTL); ok {
			if cfgJSON {
				fmt.Println(string(cached))
				return nil
			}
			var boards []client.Board
			if err := json.Unmarshal(cached, &boards); err == nil {
				printBoardTable(boards)
				return nil
			}
		}
	}

	body, err := c.Get("/kaizen/boards?includeChildren=true&amount=100")
	if err != nil {
		return fmt.Errorf("failed to list boards: %w", err)
	}

	var resp client.APIResponse[[]client.Board]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse boards response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	if cfgJSON {
		out, _ := json.MarshalIndent(resp.Data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	printBoardTable(resp.Data)
	return nil
}

func printBoardTable(boards []client.Board) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tKEY\tDESCRIPTION")
	for _, b := range boards {
		desc := b.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", b.Name, b.Prefix, desc)
	}
	_ = w.Flush()
}

func runBoardGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	// First fetch boards list and filter
	body, err := c.Get("/kaizen/boards?includeChildren=true&amount=100")
	if err != nil {
		return fmt.Errorf("failed to fetch boards: %w", err)
	}

	var resp client.APIResponse[[]client.Board]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse boards response: %w", err)
	}

	nameOrID := args[0]
	for _, b := range resp.Data {
		if strings.EqualFold(b.Name, nameOrID) || b.ID == nameOrID {
			if cfgJSON {
				out, _ := json.MarshalIndent(b, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Printf("Name:        %s\n", b.Name)
			fmt.Printf("ID:          %s\n", b.ID)
			fmt.Printf("Key:         %s\n", b.Prefix)
			fmt.Printf("Description: %s\n", b.Description)
			if len(b.ChildBoards) > 0 {
				fmt.Println("Children:")
				for _, ch := range b.ChildBoards {
					fmt.Printf("  - %s (%s)\n", ch.Name, ch.Prefix)
				}
			}
			return nil
		}
	}

	return fmt.Errorf("board %q not found", nameOrID)
}

func runBoardCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	key, _ := cmd.Flags().GetString("key")

	if name == "" || key == "" {
		return fmt.Errorf("--name and --key are required")
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	payload := client.BoardCreateRequest{
		Name:        name,
		Description: description,
		Prefix:      key,
	}

	body, err := c.Post("/kaizen/boards", payload)
	if err != nil {
		return fmt.Errorf("failed to create board: %w", err)
	}

	// Invalidate boards cache
	_ = cache.Delete("boards")

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Board]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Board created: %s (%s)\n", resp.Data.Name, resp.Data.ID)
	return nil
}

func runBoardUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	payload := client.BoardUpdateRequest{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		payload.Name = &v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		payload.Description = &v
	}
	if cmd.Flags().Changed("key") {
		v, _ := cmd.Flags().GetString("key")
		payload.Prefix = &v
	}

	body, err := c.Put(fmt.Sprintf("/kaizen/boards/%s", boardID), payload)
	if err != nil {
		return fmt.Errorf("failed to update board: %w", err)
	}

	_ = cache.Delete("boards")

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Board %s updated.\n", args[0])
	return nil
}

func runBoardDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	_, err = c.Delete(fmt.Sprintf("/kaizen/boards/%s", boardID))
	if err != nil {
		return fmt.Errorf("failed to delete board: %w", err)
	}

	_ = cache.Delete("boards")

	fmt.Printf("Board %s deleted.\n", args[0])
	return nil
}

func runBoardRestore(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	_, err = c.Post(fmt.Sprintf("/kaizen/boards/%s/restore", boardID), nil)
	if err != nil {
		return fmt.Errorf("failed to restore board: %w", err)
	}

	_ = cache.Delete("boards")

	fmt.Printf("Board %s restored.\n", args[0])
	return nil
}

func runBoardChildrenAdd(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	childIDs, _ := cmd.Flags().GetString("child-ids")
	if childIDs == "" {
		return fmt.Errorf("--child-ids is required")
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	ids := strings.Split(childIDs, ",")
	for i := range ids {
		ids[i] = strings.TrimSpace(ids[i])
	}

	_, err = c.Put(fmt.Sprintf("/kaizen/boards/%s/children", boardID), ids)
	if err != nil {
		return fmt.Errorf("failed to add children: %w", err)
	}

	_ = cache.Delete("boards")

	fmt.Printf("Added %d child board(s) to %s.\n", len(ids), args[0])
	return nil
}

func runBoardRelated(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/related", boardID))
	if err != nil {
		return fmt.Errorf("failed to get related boards: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[[]client.Board]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No related boards found.")
		return nil
	}

	printBoardTable(resp.Data)
	return nil
}

func runBoardSetDefault(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	boardName := args[0]

	// Validate board exists
	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)
	if _, err := cache.ResolveBoard(boardName, c); err != nil {
		return err
	}

	// Write to ~/.kaizen/config.yaml
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".kaizen")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig() // ignore error if file doesn't exist yet

	v.Set("default-board", boardName)
	if err := v.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Default board set to %s\n", boardName)
	return nil
}
