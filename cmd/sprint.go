package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Parent command
// ---------------------------------------------------------------------------

var sprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "Manage Kaizen sprints",
	Long:  "Create, list, start, complete, and manage sprints on a Kaizen board.",
}

// ---------------------------------------------------------------------------
// sprint list
// ---------------------------------------------------------------------------

var sprintListCmd = &cobra.Command{
	Use:   "list [board]",
	Short: "List sprints on a board",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSprintList,
}

func runSprintList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardName := cfgDefaultBoard
	if len(args) > 0 {
		boardName = args[0]
	}
	if boardName == "" {
		return fmt.Errorf("board is required. Pass as argument or set a default board")
	}

	boardID, err := cache.ResolveBoard(boardName, c)
	if err != nil {
		return err
	}

	refresh, _ := cmd.Flags().GetBool("refresh")
	cacheKey := fmt.Sprintf("sprints:%s", boardID)
	sprintsTTL := 15 * time.Minute

	// Try cache unless --refresh
	if !refresh {
		if cached, ok := cache.Get(cacheKey, sprintsTTL); ok {
			if cfgJSON {
				fmt.Println(string(cached))
				return nil
			}
			var sprints []client.Sprint
			if json.Unmarshal(cached, &sprints) == nil {
				printSprintTable(sprints)
				return nil
			}
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints", boardID)
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to list sprints: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[[]client.Sprint]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse sprints response: %w", err)
	}

	// Cache the sprints
	_ = cache.Set(cacheKey, resp.Data)

	if cfgJSON {
		return nil
	}

	if len(resp.Data) == 0 {
		fmt.Println("No sprints found.")
		return nil
	}

	printSprintTable(resp.Data)
	return nil
}

func printSprintTable(sprints []client.Sprint) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATUS\tSTART\tEND")
	for _, s := range sprints {
		start := "-"
		if s.StartDate != nil {
			start = *s.StartDate
		}
		end := "-"
		if s.EndDate != nil {
			end = *s.EndDate
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Status, start, end)
	}
	_ = w.Flush()
}

// ---------------------------------------------------------------------------
// sprint get
// ---------------------------------------------------------------------------

var sprintGetCmd = &cobra.Command{
	Use:   "get <board> <sprintId>",
	Short: "Get a sprint by ID",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintGet,
}

func runSprintGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, args[1])
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to get sprint: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Sprint]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse sprint response: %w", err)
	}

	s := resp.Data
	fmt.Printf("Name:        %s\n", s.Name)
	fmt.Printf("ID:          %s\n", s.ID)
	fmt.Printf("Status:      %s\n", s.Status)
	fmt.Printf("Description: %s\n", s.Description)
	if s.StartDate != nil {
		fmt.Printf("Start Date:  %s\n", *s.StartDate)
	}
	if s.EndDate != nil {
		fmt.Printf("End Date:    %s\n", *s.EndDate)
	}
	fmt.Printf("Created:     %s\n", s.CreatedAt)

	return nil
}

// ---------------------------------------------------------------------------
// sprint create
// ---------------------------------------------------------------------------

var sprintCreateCmd = &cobra.Command{
	Use:   "create <board>",
	Short: "Create a new sprint",
	Args:  cobra.ExactArgs(1),
	RunE:  runSprintCreate,
}

func runSprintCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	req := client.SprintCreateRequest{
		Name: name,
	}
	if v, _ := cmd.Flags().GetString("description"); v != "" {
		req.Description = v
	}
	if v, _ := cmd.Flags().GetString("start-date"); v != "" {
		req.StartDate = &v
	}
	if v, _ := cmd.Flags().GetString("end-date"); v != "" {
		req.EndDate = &v
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints", boardID)
	body, err := c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to create sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Sprint]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse sprint response: %w", err)
	}

	fmt.Printf("Created sprint: %s\n", resp.Data.Name)
	return nil
}

// ---------------------------------------------------------------------------
// sprint update
// ---------------------------------------------------------------------------

var sprintUpdateCmd = &cobra.Command{
	Use:   "update <board> <sprintId>",
	Short: "Update a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintUpdate,
}

func runSprintUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	req := client.SprintUpdateRequest{}
	hasChanges := false

	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		req.Name = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		req.Description = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("start-date") {
		v, _ := cmd.Flags().GetString("start-date")
		req.StartDate = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("end-date") {
		v, _ := cmd.Flags().GetString("end-date")
		req.EndDate = &v
		hasChanges = true
	}

	if !hasChanges {
		return fmt.Errorf("no fields specified to update")
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, args[1])
	body, err := c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to update sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Sprint]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse sprint response: %w", err)
	}

	fmt.Printf("Updated sprint: %s\n", resp.Data.Name)
	return nil
}

// ---------------------------------------------------------------------------
// sprint start
// ---------------------------------------------------------------------------

var sprintStartCmd = &cobra.Command{
	Use:   "start <board> <sprintId>",
	Short: "Start a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintStart,
}

func runSprintStart(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/start", boardID, args[1])
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to start sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	fmt.Printf("Started sprint %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// sprint complete
// ---------------------------------------------------------------------------

var sprintCompleteCmd = &cobra.Command{
	Use:   "complete <board> <sprintId>",
	Short: "Complete a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintComplete,
}

func runSprintComplete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/complete", boardID, args[1])
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to complete sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	fmt.Printf("Completed sprint %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// sprint link
// ---------------------------------------------------------------------------

var sprintLinkCmd = &cobra.Command{
	Use:   "link <board> <sprintId>",
	Short: "Link tickets to a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintLink,
}

func runSprintLink(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	tickets, _ := cmd.Flags().GetString("tickets")
	if tickets == "" {
		return fmt.Errorf("--tickets is required")
	}

	req := client.SprintLinkRequest{
		TicketIDs: strings.Split(tickets, ","),
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/link", boardID, args[1])
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to link tickets to sprint: %w", err)
	}

	fmt.Printf("Linked %d tickets to sprint %s\n", len(req.TicketIDs), args[1])
	return nil
}

// ---------------------------------------------------------------------------
// sprint unlink
// ---------------------------------------------------------------------------

var sprintUnlinkCmd = &cobra.Command{
	Use:   "unlink <board> <sprintId>",
	Short: "Unlink tickets from a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintUnlink,
}

func runSprintUnlink(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	tickets, _ := cmd.Flags().GetString("tickets")
	if tickets == "" {
		return fmt.Errorf("--tickets is required")
	}

	req := client.SprintLinkRequest{
		TicketIDs: strings.Split(tickets, ","),
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/unlink", boardID, args[1])
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to unlink tickets from sprint: %w", err)
	}

	fmt.Printf("Unlinked %d tickets from sprint %s\n", len(req.TicketIDs), args[1])
	return nil
}

// ---------------------------------------------------------------------------
// sprint delete
// ---------------------------------------------------------------------------

var sprintDeleteCmd = &cobra.Command{
	Use:   "delete <board> <sprintId>",
	Short: "Delete a sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintDelete,
}

func runSprintDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, args[1])
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	fmt.Printf("Deleted sprint %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// sprint restore
// ---------------------------------------------------------------------------

var sprintRestoreCmd = &cobra.Command{
	Use:   "restore <board> <sprintId>",
	Short: "Restore a deleted sprint",
	Args:  cobra.ExactArgs(2),
	RunE:  runSprintRestore,
}

func runSprintRestore(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/restore", boardID, args[1])
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to restore sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	fmt.Printf("Restored sprint %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// init — register all subcommands
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(sprintCmd)

	// sprint list
	sprintCmd.AddCommand(sprintListCmd)
	sprintListCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh data")

	// sprint get
	sprintCmd.AddCommand(sprintGetCmd)

	// sprint create
	sprintCmd.AddCommand(sprintCreateCmd)
	sprintCreateCmd.Flags().String("name", "", "Sprint name (required)")
	sprintCreateCmd.Flags().String("description", "", "Sprint description")
	sprintCreateCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	sprintCreateCmd.Flags().String("end-date", "", "End date (YYYY-MM-DD)")
	_ = sprintCreateCmd.MarkFlagRequired("name")

	// sprint update
	sprintCmd.AddCommand(sprintUpdateCmd)
	sprintUpdateCmd.Flags().String("name", "", "New name")
	sprintUpdateCmd.Flags().String("description", "", "New description")
	sprintUpdateCmd.Flags().String("start-date", "", "New start date (YYYY-MM-DD)")
	sprintUpdateCmd.Flags().String("end-date", "", "New end date (YYYY-MM-DD)")

	// sprint start
	sprintCmd.AddCommand(sprintStartCmd)

	// sprint complete
	sprintCmd.AddCommand(sprintCompleteCmd)

	// sprint link
	sprintCmd.AddCommand(sprintLinkCmd)
	sprintLinkCmd.Flags().String("tickets", "", "Comma-separated ticket IDs (required)")

	// sprint unlink
	sprintCmd.AddCommand(sprintUnlinkCmd)
	sprintUnlinkCmd.Flags().String("tickets", "", "Comma-separated ticket IDs (required)")

	// sprint delete
	sprintCmd.AddCommand(sprintDeleteCmd)

	// sprint restore
	sprintCmd.AddCommand(sprintRestoreCmd)
}
