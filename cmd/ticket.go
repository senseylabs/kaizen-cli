package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/senseylabs/kaizen-cli/internal/auth"
	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Parent command
// ---------------------------------------------------------------------------

var ticketCmd = &cobra.Command{
	Use:   "ticket",
	Short: "Manage Kaizen tickets",
	Long:  "Create, list, update, delete, move, and restore tickets on a Kaizen board.",
}

// ---------------------------------------------------------------------------
// ticket list
// ---------------------------------------------------------------------------

var ticketListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets on a board",
	RunE:  runTicketList,
}

func runTicketList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardName, _ := cmd.Flags().GetString("board")
	if boardName == "" {
		boardName = cfgDefaultBoard
	}
	if boardName == "" {
		return fmt.Errorf("board is required. Use --board or set a default board")
	}

	boardID, err := cache.ResolveBoard(boardName, c)
	if err != nil {
		return err
	}

	params := url.Values{}

	if v, _ := cmd.Flags().GetString("status"); v != "" {
		for _, s := range strings.Split(v, ",") {
			params.Add("status[]", strings.TrimSpace(s))
		}
	}
	if v, _ := cmd.Flags().GetString("assignee"); v != "" {
		for _, a := range strings.Split(v, ",") {
			params.Add("assigneeIds[]", strings.TrimSpace(a))
		}
	}
	if v, _ := cmd.Flags().GetString("label"); v != "" {
		for _, l := range strings.Split(v, ",") {
			params.Add("labelIds[]", strings.TrimSpace(l))
		}
	}
	if v, _ := cmd.Flags().GetString("search"); v != "" {
		params.Set("search", v)
	}
	if v, _ := cmd.Flags().GetString("sprint"); v != "" {
		params.Set("sprintId", v)
	}
	if v, _ := cmd.Flags().GetString("backlog"); v != "" {
		params.Set("backlogId", v)
	}
	page, _ := cmd.Flags().GetInt("page")
	params.Set("page", strconv.Itoa(page))
	amount, _ := cmd.Flags().GetInt("amount")
	params.Set("amount", strconv.Itoa(amount))
	if v, _ := cmd.Flags().GetString("sort-by"); v != "" {
		params.Set("sortBy", v)
	}
	if v, _ := cmd.Flags().GetString("sort-dir"); v != "" {
		params.Set("sortDirection", v)
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets?%s", boardID, params.Encode())
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to list tickets: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.PaginatedResponse[client.Ticket]]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	if len(resp.Data.Content) == 0 {
		fmt.Println("No tickets found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE")
	for _, t := range resp.Data.Content {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d of %d tickets (page %d/%d)\n",
		len(resp.Data.Content), resp.Data.TotalElements, resp.Data.Number+1, resp.Data.TotalPages)

	return nil
}

// ---------------------------------------------------------------------------
// ticket all
// ---------------------------------------------------------------------------

var ticketAllCmd = &cobra.Command{
	Use:   "all",
	Short: "List tickets across all boards",
	RunE:  runTicketAll,
}

func runTicketAll(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	params := url.Values{}
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		for _, s := range strings.Split(v, ",") {
			params.Add("status[]", strings.TrimSpace(s))
		}
	}
	if v, _ := cmd.Flags().GetString("assignee"); v != "" {
		for _, a := range strings.Split(v, ",") {
			params.Add("assigneeIds[]", strings.TrimSpace(a))
		}
	}
	if v, _ := cmd.Flags().GetString("search"); v != "" {
		params.Set("search", v)
	}
	page, _ := cmd.Flags().GetInt("page")
	params.Set("page", strconv.Itoa(page))
	amount, _ := cmd.Flags().GetInt("amount")
	params.Set("amount", strconv.Itoa(amount))

	path := fmt.Sprintf("/kaizen/tickets?%s", params.Encode())
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to list tickets: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.PaginatedResponse[client.Ticket]]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	if len(resp.Data.Content) == 0 {
		fmt.Println("No tickets found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE")
	for _, t := range resp.Data.Content {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d of %d tickets (page %d/%d)\n",
		len(resp.Data.Content), resp.Data.TotalElements, resp.Data.Number+1, resp.Data.TotalPages)

	return nil
}

// ---------------------------------------------------------------------------
// ticket mine
// ---------------------------------------------------------------------------

var ticketMineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List tickets assigned to me",
	RunE:  runTicketMine,
}

func runTicketMine(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	store := auth.NewCredentialStore()
	creds, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}
	if creds.UserID == "" {
		return fmt.Errorf("user ID not stored. Please run 'kaizen login' again")
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	params := url.Values{}
	params.Add("assigneeIds[]", creds.UserID)
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		for _, s := range strings.Split(v, ",") {
			params.Add("status[]", strings.TrimSpace(s))
		}
	}
	if v, _ := cmd.Flags().GetString("search"); v != "" {
		params.Set("search", v)
	}
	page, _ := cmd.Flags().GetInt("page")
	params.Set("page", strconv.Itoa(page))
	amount, _ := cmd.Flags().GetInt("amount")
	params.Set("amount", strconv.Itoa(amount))

	path := fmt.Sprintf("/kaizen/tickets?%s", params.Encode())
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to list tickets: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.PaginatedResponse[client.Ticket]]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	if len(resp.Data.Content) == 0 {
		fmt.Println("No tickets assigned to you.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE")
	for _, t := range resp.Data.Content {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d of %d tickets (page %d/%d)\n",
		len(resp.Data.Content), resp.Data.TotalElements, resp.Data.Number+1, resp.Data.TotalPages)

	return nil
}

// ---------------------------------------------------------------------------
// ticket get
// ---------------------------------------------------------------------------

var ticketGetCmd = &cobra.Command{
	Use:   "get <board> <ticketId>",
	Short: "Get a ticket by ID",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketGet,
}

func runTicketGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, args[1])
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to get ticket: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.TicketDetail]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse ticket response: %w", err)
	}

	t := resp.Data
	fmt.Printf("Key:      %s\n", t.Key)
	fmt.Printf("Title:    %s\n", t.Title)
	fmt.Printf("Type:     %s\n", t.Type)
	fmt.Printf("Status:   %s\n", t.Status)
	fmt.Printf("Priority: %s\n", t.Priority)
	if t.Description != nil {
		fmt.Printf("Description: %s\n", *t.Description)
	}
	if len(t.Assignees) > 0 {
		names := make([]string, len(t.Assignees))
		for i, a := range t.Assignees {
			names[i] = a.FirstName + " " + a.LastName
		}
		fmt.Printf("Assignees: %s\n", strings.Join(names, ", "))
	}
	if t.Project != nil {
		fmt.Printf("Project:  %s\n", t.Project.Name)
	}
	if t.Sprint != nil {
		fmt.Printf("Sprint:   %s\n", t.Sprint.Name)
	}
	if t.Backlog != nil {
		fmt.Printf("Backlog:  %s\n", t.Backlog.Name)
	}
	if t.Weight != nil {
		fmt.Printf("Story Points: %d\n", *t.Weight)
	}
	if t.DueDate != nil {
		fmt.Printf("Due Date: %s\n", *t.DueDate)
	}
	if len(t.Labels) > 0 {
		names := make([]string, len(t.Labels))
		for i, l := range t.Labels {
			names[i] = l.Name
		}
		fmt.Printf("Labels:   %s\n", strings.Join(names, ", "))
	}
	fmt.Printf("Created:  %s\n", t.CreatedAt)
	fmt.Printf("Created By: %s %s\n", t.CreatedBy.FirstName, t.CreatedBy.LastName)

	return nil
}

// ---------------------------------------------------------------------------
// ticket create
// ---------------------------------------------------------------------------

var ticketCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket",
	RunE:  runTicketCreate,
}

func runTicketCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardName, _ := cmd.Flags().GetString("board")
	if boardName == "" {
		boardName = cfgDefaultBoard
	}
	if boardName == "" {
		return fmt.Errorf("board is required. Use --board or set a default board")
	}

	boardID, err := cache.ResolveBoard(boardName, c)
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")
	ticketType, _ := cmd.Flags().GetString("type")
	priority, _ := cmd.Flags().GetString("priority")
	status, _ := cmd.Flags().GetString("status")

	req := client.TicketCreateRequest{
		Title:    title,
		Type:     ticketType,
		Priority: priority,
		Status:   status,
	}

	if v, _ := cmd.Flags().GetString("description"); v != "" {
		req.Description = &v
	}
	if v, _ := cmd.Flags().GetString("assignee"); v != "" {
		req.AssigneeIDs = strings.Split(v, ",")
	}
	if v, _ := cmd.Flags().GetString("label"); v != "" {
		req.LabelIDs = strings.Split(v, ",")
	}
	if v, _ := cmd.Flags().GetString("project"); v != "" {
		req.ProjectID = &v
	}
	if v, _ := cmd.Flags().GetInt("story-points"); cmd.Flags().Changed("story-points") {
		req.Weight = &v
	}
	if v, _ := cmd.Flags().GetString("due-date"); v != "" {
		req.DueDate = &v
	}

	// Sprint/backlog placement
	if v, _ := cmd.Flags().GetString("sprint"); v != "" {
		req.SprintID = &v
	}
	if v, _ := cmd.Flags().GetString("backlog"); v != "" {
		req.BacklogID = &v
	}

	// Auto-resolve backlog if neither sprint nor backlog specified
	if req.SprintID == nil && req.BacklogID == nil {
		backlogID, resolveErr := resolveBacklog(boardID, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to auto-resolve backlog for board: %w. Use --backlog or --sprint explicitly", resolveErr)
		}
		if backlogID != "" {
			req.BacklogID = &backlogID
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets", boardID)
	body, err := c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to create ticket: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse ticket response: %w", err)
	}

	fmt.Printf("Created ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
	return nil
}

// resolveBacklog fetches the board's backlog ID.
func resolveBacklog(boardID string, c *client.KaizenClient) (string, error) {
	path := fmt.Sprintf("/kaizen/boards/%s/backlog", boardID)
	body, err := c.Get(path)
	if err != nil {
		return "", err
	}

	var resp client.APIResponse[client.Backlog]
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	return resp.Data.ID, nil
}

// ---------------------------------------------------------------------------
// ticket update
// ---------------------------------------------------------------------------

var ticketUpdateCmd = &cobra.Command{
	Use:   "update <board> <ticketId>",
	Short: "Update a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketUpdate,
}

func runTicketUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	req := client.TicketUpdateRequest{}
	hasChanges := false

	if cmd.Flags().Changed("title") {
		v, _ := cmd.Flags().GetString("title")
		req.Title = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		req.Description = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("type") {
		v, _ := cmd.Flags().GetString("type")
		req.Type = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("status") {
		v, _ := cmd.Flags().GetString("status")
		req.Status = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("priority") {
		v, _ := cmd.Flags().GetString("priority")
		req.Priority = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("sprint") {
		v, _ := cmd.Flags().GetString("sprint")
		req.SprintID = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("backlog") {
		v, _ := cmd.Flags().GetString("backlog")
		req.BacklogID = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("project") {
		v, _ := cmd.Flags().GetString("project")
		req.ProjectID = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("assignee") {
		v, _ := cmd.Flags().GetString("assignee")
		req.AssigneeIDs = strings.Split(v, ",")
		hasChanges = true
	}
	if cmd.Flags().Changed("label") {
		v, _ := cmd.Flags().GetString("label")
		req.LabelIDs = strings.Split(v, ",")
		hasChanges = true
	}
	if cmd.Flags().Changed("story-points") {
		v, _ := cmd.Flags().GetInt("story-points")
		req.Weight = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("due-date") {
		v, _ := cmd.Flags().GetString("due-date")
		req.DueDate = &v
		hasChanges = true
	}
	if cmd.Flags().Changed("percentage") {
		v, _ := cmd.Flags().GetInt("percentage")
		req.Percentage = &v
		hasChanges = true
	}

	if !hasChanges {
		return fmt.Errorf("no fields specified to update")
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, args[1])
	body, err := c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse ticket response: %w", err)
	}

	fmt.Printf("Updated ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
	return nil
}

// ---------------------------------------------------------------------------
// ticket delete
// ---------------------------------------------------------------------------

var ticketDeleteCmd = &cobra.Command{
	Use:   "delete <board> <ticketId>",
	Short: "Delete a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketDelete,
}

func runTicketDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, args[1])
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete ticket: %w", err)
	}

	fmt.Printf("Deleted ticket %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// ticket restore
// ---------------------------------------------------------------------------

var ticketRestoreCmd = &cobra.Command{
	Use:   "restore <board> <ticketId>",
	Short: "Restore a deleted ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketRestore,
}

func runTicketRestore(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/restore", boardID, args[1])
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to restore ticket: %w", err)
	}

	fmt.Printf("Restored ticket %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// ticket move
// ---------------------------------------------------------------------------

var ticketMoveCmd = &cobra.Command{
	Use:   "move <board> <ticketId>",
	Short: "Move a ticket to another board, sprint, or backlog",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketMove,
}

func runTicketMove(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	req := client.TicketMoveRequest{}
	if v, _ := cmd.Flags().GetString("target-board"); v != "" {
		targetBoardID, resolveErr := cache.ResolveBoard(v, c)
		if resolveErr != nil {
			return resolveErr
		}
		req.TargetBoardID = &targetBoardID
	}
	if v, _ := cmd.Flags().GetString("target-sprint"); v != "" {
		req.TargetSprintID = &v
	}
	if v, _ := cmd.Flags().GetString("target-backlog"); v != "" {
		req.TargetBacklogID = &v
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/move", boardID, args[1])
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to move ticket: %w", err)
	}

	fmt.Printf("Moved ticket %s\n", args[1])
	return nil
}

// ---------------------------------------------------------------------------
// ticket bulk-move
// ---------------------------------------------------------------------------

var ticketBulkMoveCmd = &cobra.Command{
	Use:   "bulk-move <board>",
	Short: "Move multiple tickets at once",
	Args:  cobra.ExactArgs(1),
	RunE:  runTicketBulkMove,
}

func runTicketBulkMove(cmd *cobra.Command, args []string) error {
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

	req := client.BulkMoveRequest{
		TicketIDs: strings.Split(tickets, ","),
	}
	if v, _ := cmd.Flags().GetString("target-sprint"); v != "" {
		req.TargetSprintID = &v
	}
	if v, _ := cmd.Flags().GetString("target-backlog"); v != "" {
		req.TargetBacklogID = &v
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/bulk-move", boardID)
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to bulk move tickets: %w", err)
	}

	fmt.Printf("Moved %d tickets\n", len(req.TicketIDs))
	return nil
}

// ---------------------------------------------------------------------------
// ticket order
// ---------------------------------------------------------------------------

var ticketOrderCmd = &cobra.Command{
	Use:   "order <board> <ticketId>",
	Short: "Reorder a ticket within a sprint or backlog",
	Args:  cobra.ExactArgs(2),
	RunE:  runTicketOrder,
}

func runTicketOrder(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	order, _ := cmd.Flags().GetInt("order")

	req := client.TicketOrderRequest{
		Order: order,
	}
	if v, _ := cmd.Flags().GetString("sprint"); v != "" {
		req.SprintID = &v
	}
	if v, _ := cmd.Flags().GetString("backlog"); v != "" {
		req.BacklogID = &v
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/order", boardID, args[1])
	_, err = c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to reorder ticket: %w", err)
	}

	fmt.Printf("Reordered ticket %s to position %d\n", args[1], order)
	return nil
}

// ---------------------------------------------------------------------------
// init — register all subcommands
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(ticketCmd)

	// ticket list
	ticketCmd.AddCommand(ticketListCmd)
	ticketListCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketListCmd.Flags().String("status", "", "Filter by status (comma-separated)")
	ticketListCmd.Flags().String("assignee", "", "Filter by assignee UUIDs (comma-separated)")
	ticketListCmd.Flags().String("label", "", "Filter by label UUIDs (comma-separated)")
	ticketListCmd.Flags().String("search", "", "Search tickets by title")
	ticketListCmd.Flags().String("sprint", "", "Filter by sprint ID")
	ticketListCmd.Flags().String("backlog", "", "Filter by backlog ID")
	ticketListCmd.Flags().Int("page", 0, "Page number")
	ticketListCmd.Flags().Int("amount", 100, "Number of tickets per page")
	ticketListCmd.Flags().String("sort-by", "", "Sort field")
	ticketListCmd.Flags().String("sort-dir", "", "Sort direction (ASC or DESC)")

	// ticket all
	ticketCmd.AddCommand(ticketAllCmd)
	ticketAllCmd.Flags().String("status", "", "Filter by status (comma-separated)")
	ticketAllCmd.Flags().String("assignee", "", "Filter by assignee UUIDs (comma-separated)")
	ticketAllCmd.Flags().String("search", "", "Search tickets by title")
	ticketAllCmd.Flags().Int("page", 0, "Page number")
	ticketAllCmd.Flags().Int("amount", 100, "Number of tickets per page")

	// ticket mine
	ticketCmd.AddCommand(ticketMineCmd)
	ticketMineCmd.Flags().String("status", "", "Filter by status (comma-separated)")
	ticketMineCmd.Flags().String("search", "", "Search tickets by title")
	ticketMineCmd.Flags().Int("page", 0, "Page number")
	ticketMineCmd.Flags().Int("amount", 100, "Number of tickets per page")

	// ticket get
	ticketCmd.AddCommand(ticketGetCmd)

	// ticket create
	ticketCmd.AddCommand(ticketCreateCmd)
	ticketCreateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketCreateCmd.Flags().String("title", "", "Ticket title (required)")
	ticketCreateCmd.Flags().String("type", "", "Ticket type: TASK or INCIDENT (required)")
	ticketCreateCmd.Flags().String("priority", "", "Priority: LOW, MEDIUM, HIGH, or CRITICAL (required)")
	ticketCreateCmd.Flags().String("status", "", "Status: TODO, IN_PROGRESS, IN_REVIEW, or DONE (required)")
	ticketCreateCmd.Flags().String("description", "", "Ticket description")
	ticketCreateCmd.Flags().String("assignee", "", "Assignee UUIDs (comma-separated)")
	ticketCreateCmd.Flags().String("label", "", "Label UUIDs (comma-separated)")
	ticketCreateCmd.Flags().String("project", "", "Project ID")
	ticketCreateCmd.Flags().String("sprint", "", "Sprint ID")
	ticketCreateCmd.Flags().String("backlog", "", "Backlog ID")
	ticketCreateCmd.Flags().Int("story-points", 0, "Story points (weight)")
	ticketCreateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	_ = ticketCreateCmd.MarkFlagRequired("title")
	_ = ticketCreateCmd.MarkFlagRequired("type")
	_ = ticketCreateCmd.MarkFlagRequired("priority")
	_ = ticketCreateCmd.MarkFlagRequired("status")

	// ticket update
	ticketCmd.AddCommand(ticketUpdateCmd)
	ticketUpdateCmd.Flags().String("title", "", "New title")
	ticketUpdateCmd.Flags().String("description", "", "New description")
	ticketUpdateCmd.Flags().String("type", "", "New type: TASK or INCIDENT")
	ticketUpdateCmd.Flags().String("status", "", "New status: TODO, IN_PROGRESS, IN_REVIEW, or DONE")
	ticketUpdateCmd.Flags().String("priority", "", "New priority: LOW, MEDIUM, HIGH, or CRITICAL")
	ticketUpdateCmd.Flags().String("sprint", "", "Sprint ID")
	ticketUpdateCmd.Flags().String("backlog", "", "Backlog ID")
	ticketUpdateCmd.Flags().String("project", "", "Project ID")
	ticketUpdateCmd.Flags().String("assignee", "", "Assignee UUIDs (comma-separated)")
	ticketUpdateCmd.Flags().String("label", "", "Label UUIDs (comma-separated)")
	ticketUpdateCmd.Flags().Int("story-points", 0, "Story points (weight)")
	ticketUpdateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	ticketUpdateCmd.Flags().Int("percentage", 0, "Completion percentage")

	// ticket delete
	ticketCmd.AddCommand(ticketDeleteCmd)

	// ticket restore
	ticketCmd.AddCommand(ticketRestoreCmd)

	// ticket move
	ticketCmd.AddCommand(ticketMoveCmd)
	ticketMoveCmd.Flags().String("target-board", "", "Target board name or ID")
	ticketMoveCmd.Flags().String("target-sprint", "", "Target sprint ID")
	ticketMoveCmd.Flags().String("target-backlog", "", "Target backlog ID")

	// ticket bulk-move
	ticketCmd.AddCommand(ticketBulkMoveCmd)
	ticketBulkMoveCmd.Flags().String("tickets", "", "Comma-separated ticket IDs (required)")
	ticketBulkMoveCmd.Flags().String("target-sprint", "", "Target sprint ID")
	ticketBulkMoveCmd.Flags().String("target-backlog", "", "Target backlog ID")

	// ticket order
	ticketCmd.AddCommand(ticketOrderCmd)
	ticketOrderCmd.Flags().Int("order", 0, "New display order (required)")
	ticketOrderCmd.Flags().String("sprint", "", "Sprint ID context")
	ticketOrderCmd.Flags().String("backlog", "", "Backlog ID context")
	_ = ticketOrderCmd.MarkFlagRequired("order")
}
