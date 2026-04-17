package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	Use:   "list",
	Short: "List sprints on a board",
	Args:  cobra.NoArgs,
	RunE:  runSprintList,
}

func runSprintList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
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
	Use:   "get [sprintName]",
	Short: "Get a sprint by name or select interactively",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintGet,
}

func runSprintGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, _, err := resolveSprintArg(boardID, args, c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, sprintID)
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
	Use:   "create",
	Short: "Create a new sprint",
	Args:  cobra.NoArgs,
	RunE:  runSprintCreate,
}

func runSprintCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")

	// Interactive mode if name not provided
	if name == "" && isInteractive() {
		var promptErr error
		name, promptErr = promptTextRequired("Name")
		if promptErr != nil {
			return promptErr
		}
	}
	if name == "" {
		return fmt.Errorf("--name is required (or run interactively)")
	}

	req := client.SprintCreateRequest{
		Name: name,
	}

	if v, _ := cmd.Flags().GetString("description"); v != "" {
		req.Description = v
	} else if isInteractive() && !cmd.Flags().Changed("description") {
		desc, promptErr := promptText("Description (optional)")
		if promptErr == nil && desc != "" {
			req.Description = desc
		}
	}

	if v, _ := cmd.Flags().GetString("start-date"); v != "" {
		req.StartDate = &v
	} else if isInteractive() && !cmd.Flags().Changed("start-date") {
		startDate, promptErr := promptDate("Start Date (optional)")
		if promptErr == nil && startDate != "" {
			req.StartDate = &startDate
		}
	}

	if v, _ := cmd.Flags().GetString("end-date"); v != "" {
		req.EndDate = &v
	} else if isInteractive() && !cmd.Flags().Changed("end-date") {
		endDate, promptErr := promptDate("End Date (optional)")
		if promptErr == nil && endDate != "" {
			req.EndDate = &endDate
		}
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
	Use:   "update [sprintName]",
	Short: "Update a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintUpdate,
}

func runSprintUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, _, err := resolveSprintArg(boardID, args, c)
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

	// Interactive field picker if no flags changed
	if !hasChanges && isInteractive() {
		updateFields := []string{"Name", "Description", "Start Date", "End Date"}

		cyan := promptColor("\033[36m")
		dim := promptColor("\033[90m")
		reset := promptReset()

		for {
			label := "What would you like to update?"
			if hasChanges {
				label = "What else would you like to update?"
			}

			_, _ = fmt.Fprintf(os.Stdout, "%s%s%s\n", cyan, label, reset)
			for i, f := range updateFields {
				_, _ = fmt.Fprintf(os.Stdout, "  %s%d%s  %s\n", dim, i+1, reset, f)
			}

			reader := bufio.NewReader(os.Stdin)
			_, _ = fmt.Fprintf(os.Stdout, "Select (or 'd' when done): ")
			input, readErr := reader.ReadString('\n')
			if readErr != nil {
				return fmt.Errorf("failed to read input: %w", readErr)
			}
			input = strings.TrimSpace(input)

			if strings.EqualFold(input, "d") {
				break
			}

			num, parseErr := strconv.Atoi(input)
			if parseErr != nil || num < 1 || num > len(updateFields) {
				_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(updateFields))
				continue
			}

			switch num {
			case 1: // Name
				val, promptErr := promptTextRequired("Name")
				if promptErr != nil {
					return promptErr
				}
				req.Name = &val
				hasChanges = true
			case 2: // Description
				val, promptErr := promptText("Description")
				if promptErr != nil {
					return promptErr
				}
				req.Description = &val
				hasChanges = true
			case 3: // Start Date
				val, promptErr := promptDate("Start Date")
				if promptErr != nil {
					return promptErr
				}
				if val != "" {
					req.StartDate = &val
					hasChanges = true
				}
			case 4: // End Date
				val, promptErr := promptDate("End Date")
				if promptErr != nil {
					return promptErr
				}
				if val != "" {
					req.EndDate = &val
					hasChanges = true
				}
			}
			fmt.Println()
		}
	}

	if !hasChanges {
		fmt.Println("No changes selected.")
		return nil
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, sprintID)
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

	_, _ = fmt.Fprintf(os.Stdout, "Updated sprint: %s\n", resp.Data.Name)
	return nil
}

// ---------------------------------------------------------------------------
// sprint start
// ---------------------------------------------------------------------------

var sprintStartCmd = &cobra.Command{
	Use:   "start [sprintName]",
	Short: "Start a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintStart,
}

func runSprintStart(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArgFiltered(boardID, args, "PLANNED", c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/start", boardID, sprintID)
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to start sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	_, _ = fmt.Fprintf(os.Stdout, "Started sprint %s\n", displayName)
	return nil
}

// ---------------------------------------------------------------------------
// sprint complete
// ---------------------------------------------------------------------------

var sprintCompleteCmd = &cobra.Command{
	Use:   "complete [sprintName]",
	Short: "Complete a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintComplete,
}

func runSprintComplete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArgFiltered(boardID, args, "ACTIVE", c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/complete", boardID, sprintID)
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to complete sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	_, _ = fmt.Fprintf(os.Stdout, "Completed sprint %s\n", displayName)
	return nil
}

// ---------------------------------------------------------------------------
// sprint link
// ---------------------------------------------------------------------------

var sprintLinkCmd = &cobra.Command{
	Use:   "link [sprintName]",
	Short: "Link tickets to a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintLink,
}

func runSprintLink(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArg(boardID, args, c)
	if err != nil {
		return err
	}

	tickets, _ := cmd.Flags().GetString("tickets")
	if tickets == "" {
		if isInteractive() {
			// Browse and multi-select tickets
			ticketIDs, selectErr := browseAndMultiSelectTickets(boardID, c)
			if selectErr != nil {
				return selectErr
			}
			tickets = strings.Join(ticketIDs, ",")
		} else {
			return fmt.Errorf("--tickets is required")
		}
	}

	req := client.SprintLinkRequest{
		TicketIDs: strings.Split(tickets, ","),
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/link", boardID, sprintID)
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to link tickets to sprint: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Linked %d tickets to sprint %s\n", len(req.TicketIDs), displayName)
	return nil
}

// browseAndMultiSelectTickets lets the user interactively pick multiple tickets.
func browseAndMultiSelectTickets(boardID string, c *client.KaizenClient) ([]string, error) {
	var ticketIDs []string
	for {
		_, ticketID, err := browseAndSelectTicket(boardID, c)
		if err != nil {
			if len(ticketIDs) > 0 {
				break // user cancelled after selecting at least one
			}
			return nil, err
		}
		ticketIDs = append(ticketIDs, ticketID)
		_, _ = fmt.Fprintf(os.Stdout, "  Selected %d ticket(s).\n", len(ticketIDs))

		more, promptErr := promptYesNo("Select another ticket")
		if promptErr != nil || !more {
			break
		}
	}
	if len(ticketIDs) == 0 {
		return nil, fmt.Errorf("no tickets selected")
	}
	return ticketIDs, nil
}

// ---------------------------------------------------------------------------
// sprint unlink
// ---------------------------------------------------------------------------

var sprintUnlinkCmd = &cobra.Command{
	Use:   "unlink [sprintName]",
	Short: "Unlink tickets from a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintUnlink,
}

func runSprintUnlink(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArg(boardID, args, c)
	if err != nil {
		return err
	}

	tickets, _ := cmd.Flags().GetString("tickets")
	if tickets == "" {
		if isInteractive() {
			ticketIDs, selectErr := browseAndMultiSelectTickets(boardID, c)
			if selectErr != nil {
				return selectErr
			}
			tickets = strings.Join(ticketIDs, ",")
		} else {
			return fmt.Errorf("--tickets is required")
		}
	}

	req := client.SprintLinkRequest{
		TicketIDs: strings.Split(tickets, ","),
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/unlink", boardID, sprintID)
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to unlink tickets from sprint: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Unlinked %d tickets from sprint %s\n", len(req.TicketIDs), displayName)
	return nil
}

// ---------------------------------------------------------------------------
// sprint delete
// ---------------------------------------------------------------------------

var sprintDeleteCmd = &cobra.Command{
	Use:   "delete [sprintName]",
	Short: "Delete a sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintDelete,
}

func runSprintDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArg(boardID, args, c)
	if err != nil {
		return err
	}

	if isInteractive() {
		confirmed, _ := promptYesNo(fmt.Sprintf("Delete sprint '%s'", displayName))
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s", boardID, sprintID)
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	_, _ = fmt.Fprintf(os.Stdout, "Deleted sprint %s\n", displayName)
	return nil
}

// ---------------------------------------------------------------------------
// sprint restore
// ---------------------------------------------------------------------------

var sprintRestoreCmd = &cobra.Command{
	Use:   "restore [sprintName]",
	Short: "Restore a deleted sprint",
	Args:  cobra.ArbitraryArgs,
	RunE:  runSprintRestore,
}

func runSprintRestore(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	sprintID, displayName, err := resolveSprintArg(boardID, args, c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints/%s/restore", boardID, sprintID)
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to restore sprint: %w", err)
	}

	// Invalidate sprint cache
	_ = cache.Delete(fmt.Sprintf("sprints:%s", boardID))

	_, _ = fmt.Fprintf(os.Stdout, "Restored sprint %s\n", displayName)
	return nil
}

// ---------------------------------------------------------------------------
// init — register all subcommands
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(sprintCmd)

	// sprint list
	sprintCmd.AddCommand(sprintListCmd)
	sprintListCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	sprintListCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh data")

	// sprint get
	sprintCmd.AddCommand(sprintGetCmd)
	sprintGetCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// sprint create
	sprintCmd.AddCommand(sprintCreateCmd)
	sprintCreateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	sprintCreateCmd.Flags().String("name", "", "Sprint name")
	sprintCreateCmd.Flags().String("description", "", "Sprint description")
	sprintCreateCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	sprintCreateCmd.Flags().String("end-date", "", "End date (YYYY-MM-DD)")

	// sprint update
	sprintCmd.AddCommand(sprintUpdateCmd)
	sprintUpdateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	sprintUpdateCmd.Flags().String("name", "", "New name")
	sprintUpdateCmd.Flags().String("description", "", "New description")
	sprintUpdateCmd.Flags().String("start-date", "", "New start date (YYYY-MM-DD)")
	sprintUpdateCmd.Flags().String("end-date", "", "New end date (YYYY-MM-DD)")

	// sprint start
	sprintCmd.AddCommand(sprintStartCmd)
	sprintStartCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// sprint complete
	sprintCmd.AddCommand(sprintCompleteCmd)
	sprintCompleteCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// sprint link
	sprintCmd.AddCommand(sprintLinkCmd)
	sprintLinkCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	sprintLinkCmd.Flags().String("tickets", "", "Comma-separated ticket IDs")

	// sprint unlink
	sprintCmd.AddCommand(sprintUnlinkCmd)
	sprintUnlinkCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	sprintUnlinkCmd.Flags().String("tickets", "", "Comma-separated ticket IDs")

	// sprint delete
	sprintCmd.AddCommand(sprintDeleteCmd)
	sprintDeleteCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// sprint restore
	sprintCmd.AddCommand(sprintRestoreCmd)
	sprintRestoreCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
}
