package cmd

import (
	"bufio"
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
	"golang.org/x/term"
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
	Use:   "list [sprint|<sprint-name>]",
	Short: "List tickets on a board (backlog by default, or specify sprint)",
	Long: `List tickets on a board.

Without arguments, lists backlog tickets.
With "sprint", lists tickets from the active sprint (or latest if none active).
With a sprint name (e.g. "Sprint 1"), lists tickets from that sprint.`,
	Args: cobra.ArbitraryArgs,
	RunE: runTicketList,
}

func runTicketList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
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

	// Resolve sprint/backlog from positional args
	header := "Backlog"
	sprintArg := strings.TrimSpace(strings.Join(args, " "))

	if sprintArg == "" {
		if !cfgJSON && isInteractive() {
			// Interactive mode: ask where to look
			return runInteractiveTicketList(boardID, params, c)
		}
		// Non-interactive fallback: use backlog
		backlogID, resolveErr := resolveBacklogID(boardID, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve backlog: %w", resolveErr)
		}
		params.Set("backlogId", backlogID)
	} else if strings.EqualFold(sprintArg, "sprint") && !cfgJSON {
		// Interactive sprint picker mode
		return runInteractiveSprintPicker(boardID, params, c)
	} else {
		// Args present → resolve sprint
		sprintID, displayName, resolveErr := resolveSprint(boardID, sprintArg, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve sprint: %w", resolveErr)
		}
		params.Set("sprintId", sprintID)
		header = fmt.Sprintf("Sprint: %s", displayName)
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

	return fetchAndPrintTickets(boardID, header, params, c)
}

// fetchAndPrintTickets fetches tickets for a board with the given params and prints them as a table.
func fetchAndPrintTickets(boardID string, header string, params url.Values, c *client.KaizenClient) error {
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

	// Print context header
	fmt.Println(header)
	fmt.Println()

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

// runInteractiveTicketList runs the top-level interactive menu for "kaizen ticket list" (no args).
// It shows a "Where to look?" prompt and delegates to backlog or sprint flows.
func runInteractiveTicketList(boardID string, baseParams url.Values, c *client.KaizenClient) error {
	for {
		idx, err := promptSingleSelect("Where to look?", []string{"Backlog", "Sprint"})
		if err != nil {
			return nil // user cancelled
		}

		if idx == 0 {
			// Backlog
			backlogID, resolveErr := resolveBacklogID(boardID, c)
			if resolveErr != nil {
				return resolveErr
			}
			ticketParams := cloneParams(baseParams)
			ticketParams.Set("backlogId", backlogID)
			setDefaultPagination(ticketParams)

			if printErr := fetchAndPrintTickets(boardID, "Backlog", ticketParams, c); printErr != nil {
				return printErr
			}

			action := promptPostTicketAction()
			if action != "back" {
				return nil
			}
			_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
		} else {
			// Sprint — delegate to sprint picker with back-to-menu support
			sprints, fetchErr := fetchSprints(boardID, c)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch sprints: %w", fetchErr)
			}
			if len(sprints) == 0 {
				fmt.Println("No sprints found.")
				continue
			}

		sprintLoop:
			for {
				sprintID, displayName, selectErr := promptSprintSelection(sprints)
				if selectErr != nil {
					break sprintLoop // back to "Where to look?"
				}

				ticketParams := cloneParams(baseParams)
				ticketParams.Set("sprintId", sprintID)
				setDefaultPagination(ticketParams)

				header := fmt.Sprintf("Sprint: %s", displayName)
				if printErr := fetchAndPrintTickets(boardID, header, ticketParams, c); printErr != nil {
					return printErr
				}

				action := promptPostTicketAction()
				if action != "back" {
					return nil
				}
				// Clear screen and show sprint list again
				_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
			}

			// Clear screen and show "Where to look?" again
			_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
		}
	}
}

// runInteractiveSprintPicker runs the interactive sprint selection loop.
// It shows a list of sprints, lets the user pick one, displays tickets, and allows going back.
func runInteractiveSprintPicker(boardID string, baseParams url.Values, c *client.KaizenClient) error {
	sprints, err := fetchSprints(boardID, c)
	if err != nil {
		return fmt.Errorf("failed to fetch sprints: %w", err)
	}
	if len(sprints) == 0 {
		fmt.Println("No sprints found on this board.")
		return nil
	}

	for {
		sprintID, displayName, selectErr := promptSprintSelection(sprints)
		if selectErr != nil {
			return nil // user cancelled, exit cleanly
		}

		ticketParams := cloneParams(baseParams)
		ticketParams.Set("sprintId", sprintID)
		setDefaultPagination(ticketParams)

		header := fmt.Sprintf("Sprint: %s", displayName)
		if printErr := fetchAndPrintTickets(boardID, header, ticketParams, c); printErr != nil {
			return printErr
		}

		action := promptPostTicketAction()
		if action != "back" {
			return nil
		}

		// Clear screen and show sprint list again
		_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
	}
}

// cloneParams creates a shallow copy of url.Values.
func cloneParams(src url.Values) url.Values {
	dst := url.Values{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// setDefaultPagination sets page and amount defaults if not already present.
func setDefaultPagination(params url.Values) {
	if params.Get("page") == "" {
		params.Set("page", "0")
	}
	if params.Get("amount") == "" {
		params.Set("amount", "100")
	}
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
	Use:   "get [ticketKey]",
	Short: "Get a ticket by key (e.g. SEN-42) or browse interactively",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketGet,
}

func runTicketGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string

	if len(args) > 0 {
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, ticketID)
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

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")

	// If title is not provided and we're in an interactive terminal, enter interactive mode
	if title == "" && isInteractive() && term.IsTerminal(int(os.Stdout.Fd())) {
		return runTicketCreateInteractive(cmd, boardID, c)
	}

	// Non-interactive: require flags
	if title == "" {
		return fmt.Errorf("--title is required (or run without flags for interactive mode)")
	}
	ticketType, _ := cmd.Flags().GetString("type")
	if ticketType == "" {
		return fmt.Errorf("--type is required (or run without flags for interactive mode)")
	}
	priority, _ := cmd.Flags().GetString("priority")
	if priority == "" {
		return fmt.Errorf("--priority is required (or run without flags for interactive mode)")
	}
	status, _ := cmd.Flags().GetString("status")
	if status == "" {
		return fmt.Errorf("--status is required (or run without flags for interactive mode)")
	}

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
		sprintID, _, resolveErr := resolveSprint(boardID, v, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve sprint: %w", resolveErr)
		}
		req.SprintID = &sprintID
	}
	if v, _ := cmd.Flags().GetString("backlog"); v != "" {
		req.BacklogID = &v
	}

	// Auto-resolve backlog if neither sprint nor backlog specified
	if req.SprintID == nil && req.BacklogID == nil {
		backlogID, resolveErr := resolveBacklogID(boardID, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to auto-resolve backlog for board: %w. Use --backlog or --sprint explicitly", resolveErr)
		}
		if backlogID != "" {
			req.BacklogID = &backlogID
		}
	}

	return submitTicketCreate(boardID, req, c)
}

// submitTicketCreate sends the create request and prints the result.
func submitTicketCreate(boardID string, req client.TicketCreateRequest, c *client.KaizenClient) error {
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

	fmt.Printf("\nCreated ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
	return nil
}

// runTicketCreateInteractive prompts the user for all ticket fields interactively.
func runTicketCreateInteractive(_ *cobra.Command, boardID string, c *client.KaizenClient) error {
	fmt.Println()

	// Title (required)
	title, err := promptTextRequired("Title")
	if err != nil {
		return err
	}

	req := client.TicketCreateRequest{
		Title: title,
	}

	// Description (optional)
	wantDesc, err := promptYesNo("Description (optional)")
	if err != nil {
		return err
	}
	if wantDesc {
		desc, descErr := promptText("Description")
		if descErr != nil {
			return descErr
		}
		if desc != "" {
			req.Description = &desc
		}
	}

	// Type (required)
	typeOptions := []string{"TASK", "INCIDENT"}
	typeIdx, err := promptSingleSelect("Type", typeOptions)
	if err != nil {
		return err
	}
	req.Type = typeOptions[typeIdx]

	// Priority (required) — depends on ticket type
	var priorityOptions []string
	if req.Type == "INCIDENT" {
		priorityOptions = []string{"P1", "P2", "P3"}
	} else {
		priorityOptions = []string{"LOWEST", "LOW", "MEDIUM", "HIGH", "HIGHEST"}
	}
	priorityIdx, err := promptSingleSelect("Priority", priorityOptions)
	if err != nil {
		return err
	}
	req.Priority = priorityOptions[priorityIdx]

	// Status (required)
	statusOptions := []string{"TODO", "IN_PROGRESS", "IN_REVIEW", "DONE"}
	statusIdx, err := promptSingleSelect("Status", statusOptions)
	if err != nil {
		return err
	}
	req.Status = statusOptions[statusIdx]

	// Placement: Backlog or Sprint
	placementOptions := []string{"Backlog", "Sprint"}
	placementIdx, err := promptSingleSelect("Placement", placementOptions)
	if err != nil {
		return err
	}
	if placementIdx == 1 {
		// Sprint — use interactive sprint picker
		sprints, sprintErr := fetchSprints(boardID, c)
		if sprintErr != nil {
			return fmt.Errorf("failed to fetch sprints: %w", sprintErr)
		}
		if len(sprints) == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "No sprints found. Using backlog instead.\n")
			backlogID, resolveErr := resolveBacklogID(boardID, c)
			if resolveErr != nil {
				return fmt.Errorf("failed to resolve backlog: %w", resolveErr)
			}
			req.BacklogID = &backlogID
		} else {
			sprintID, _, selectErr := promptSprintSelection(sprints)
			if selectErr != nil {
				return selectErr
			}
			req.SprintID = &sprintID
		}
	} else {
		backlogID, resolveErr := resolveBacklogID(boardID, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve backlog: %w", resolveErr)
		}
		req.BacklogID = &backlogID
	}

	// Assignees (optional)
	wantAssignees, err := promptYesNo("Assignees")
	if err != nil {
		return err
	}
	if wantAssignees {
		members, fetchErr := fetchMembers(boardID, c)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch members: %w", fetchErr)
		}
		if len(members) == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "  No members found on this board.\n")
		} else {
			memberOptions := make([]SelectOption, len(members))
			for i, m := range members {
				memberOptions[i] = SelectOption{
					Display: fmt.Sprintf("%s %s (%s)", m.FirstName, m.LastName, m.Email),
					Value:   m.UserID,
				}
			}
			assigneeIDs, selectErr := promptMultiSelectWithValues("Assignees", memberOptions)
			if selectErr != nil {
				return selectErr
			}
			if len(assigneeIDs) > 0 {
				req.AssigneeIDs = assigneeIDs
			}
		}
	}

	// Labels (optional)
	wantLabels, err := promptYesNo("Labels")
	if err != nil {
		return err
	}
	if wantLabels {
		labels, fetchErr := fetchLabels(boardID, c)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch labels: %w", fetchErr)
		}
		if len(labels) == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "  No labels found on this board.\n")
		} else {
			labelOptions := make([]SelectOption, len(labels))
			for i, l := range labels {
				display := l.Name
				if l.Color != nil {
					display = fmt.Sprintf("%s (%s)", l.Name, *l.Color)
				}
				labelOptions[i] = SelectOption{
					Display: display,
					Value:   l.ID,
				}
			}
			labelIDs, selectErr := promptMultiSelectWithValues("Labels", labelOptions)
			if selectErr != nil {
				return selectErr
			}
			if len(labelIDs) > 0 {
				req.LabelIDs = labelIDs
			}
		}
	}

	// Project (optional)
	wantProject, err := promptYesNo("Project")
	if err != nil {
		return err
	}
	if wantProject {
		projects, fetchErr := fetchProjects(boardID, c)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch projects: %w", fetchErr)
		}
		if len(projects) == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "  No projects found on this board.\n")
		} else {
			projectOptions := make([]SelectOption, len(projects))
			for i, p := range projects {
				projectOptions[i] = SelectOption{
					Display: p.Name,
					Value:   p.ID,
				}
			}
			projectID, selectErr := promptSingleSelectWithValues("Project", projectOptions)
			if selectErr != nil {
				return selectErr
			}
			req.ProjectID = &projectID
		}
	}

	// Story Points (optional)
	wantPoints, err := promptYesNo("Story Points")
	if err != nil {
		return err
	}
	if wantPoints {
		points, pointsErr := promptInt("Story Points")
		if pointsErr != nil {
			return pointsErr
		}
		if points > 0 {
			req.Weight = &points
		}
	}

	// Due Date (optional)
	wantDueDate, err := promptYesNo("Due Date")
	if err != nil {
		return err
	}
	if wantDueDate {
		dueDate, dateErr := promptDate("Due Date")
		if dateErr != nil {
			return dateErr
		}
		if dueDate != "" {
			req.DueDate = &dueDate
		}
	}

	return submitTicketCreate(boardID, req, c)
}

// ---------------------------------------------------------------------------
// ticket update
// ---------------------------------------------------------------------------

var ticketUpdateCmd = &cobra.Command{
	Use:   "update [ticketKey]",
	Short: "Update a ticket by key (e.g. SEN-42) or browse interactively",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketUpdate,
}

func runTicketUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string

	if len(args) > 0 {
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
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
		sprintID, _, resolveErr := resolveSprint(boardID, v, c)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve sprint: %w", resolveErr)
		}
		req.SprintID = &sprintID
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

	// If no flags were changed and we're in an interactive terminal, enter interactive mode
	if !hasChanges {
		if isInteractive() && term.IsTerminal(int(os.Stdout.Fd())) {
			return runTicketUpdateInteractive(boardID, ticketID, c)
		}
		return fmt.Errorf("no fields specified to update")
	}

	return submitTicketUpdate(boardID, ticketID, req, c)
}

// submitTicketUpdate sends the update request and prints the result.
func submitTicketUpdate(boardID string, ticketID string, req client.TicketUpdateRequest, c *client.KaizenClient) error {
	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, ticketID)
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

	fmt.Printf("\nUpdated ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
	return nil
}

// runTicketUpdateInteractive prompts the user to select which fields to update.
func runTicketUpdateInteractive(boardID string, ticketID string, c *client.KaizenClient) error {
	fmt.Println()

	updateFields := []string{
		"Title",
		"Description",
		"Type",
		"Status",
		"Priority",
		"Assignees",
		"Labels",
		"Sprint/Backlog",
		"Project",
		"Story Points",
		"Due Date",
		"Percentage",
	}

	req := client.TicketUpdateRequest{}
	hasChanges := false

	for {
		label := "What would you like to update?"
		if hasChanges {
			label = "What else would you like to update?"
		}

		// Use multi-select style: pick one at a time, 'd' when done
		cyan := promptColor("\033[36m")
		dim := promptColor("\033[90m")
		reset := promptReset()

		_, _ = fmt.Fprintf(os.Stdout, "%s%s%s\n", cyan, label, reset)
		for i, f := range updateFields {
			_, _ = fmt.Fprintf(os.Stdout, "  %s%d%s  %s\n", dim, i+1, reset, f)
		}

		reader := bufio.NewReader(os.Stdin)
		_, _ = fmt.Fprintf(os.Stdout, "Select (or 'd' when done): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
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
		case 1: // Title
			val, promptErr := promptTextRequired("Title")
			if promptErr != nil {
				return promptErr
			}
			req.Title = &val
			hasChanges = true

		case 2: // Description
			val, promptErr := promptText("Description")
			if promptErr != nil {
				return promptErr
			}
			req.Description = &val
			hasChanges = true

		case 3: // Type
			typeOptions := []string{"TASK", "INCIDENT"}
			idx, promptErr := promptSingleSelect("Type", typeOptions)
			if promptErr != nil {
				return promptErr
			}
			req.Type = &typeOptions[idx]
			hasChanges = true

		case 4: // Status
			statusOptions := []string{"TODO", "IN_PROGRESS", "IN_REVIEW", "DONE"}
			idx, promptErr := promptSingleSelect("Status", statusOptions)
			if promptErr != nil {
				return promptErr
			}
			req.Status = &statusOptions[idx]
			hasChanges = true

		case 5: // Priority
			// Ask for ticket type first to show correct priority options
			typeForPriority := []string{"TASK", "INCIDENT"}
			typeIdx, typeErr := promptSingleSelect("What is the ticket type?", typeForPriority)
			if typeErr != nil {
				return typeErr
			}
			var priorityOptions []string
			if typeForPriority[typeIdx] == "INCIDENT" {
				priorityOptions = []string{"P1", "P2", "P3"}
			} else {
				priorityOptions = []string{"LOWEST", "LOW", "MEDIUM", "HIGH", "HIGHEST"}
			}
			idx, promptErr := promptSingleSelect("Priority", priorityOptions)
			if promptErr != nil {
				return promptErr
			}
			req.Priority = &priorityOptions[idx]
			hasChanges = true

		case 6: // Assignees
			members, fetchErr := fetchMembers(boardID, c)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch members: %w", fetchErr)
			}
			if len(members) == 0 {
				_, _ = fmt.Fprintf(os.Stdout, "  No members found on this board.\n")
			} else {
				memberOptions := make([]SelectOption, len(members))
				for i, m := range members {
					memberOptions[i] = SelectOption{
						Display: fmt.Sprintf("%s %s (%s)", m.FirstName, m.LastName, m.Email),
						Value:   m.UserID,
					}
				}
				assigneeIDs, selectErr := promptMultiSelectWithValues("Assignees", memberOptions)
				if selectErr != nil {
					return selectErr
				}
				req.AssigneeIDs = assigneeIDs
				hasChanges = true
			}

		case 7: // Labels
			labels, fetchErr := fetchLabels(boardID, c)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch labels: %w", fetchErr)
			}
			if len(labels) == 0 {
				_, _ = fmt.Fprintf(os.Stdout, "  No labels found on this board.\n")
			} else {
				labelOptions := make([]SelectOption, len(labels))
				for i, l := range labels {
					display := l.Name
					if l.Color != nil {
						display = fmt.Sprintf("%s (%s)", l.Name, *l.Color)
					}
					labelOptions[i] = SelectOption{
						Display: display,
						Value:   l.ID,
					}
				}
				labelIDs, selectErr := promptMultiSelectWithValues("Labels", labelOptions)
				if selectErr != nil {
					return selectErr
				}
				req.LabelIDs = labelIDs
				hasChanges = true
			}

		case 8: // Sprint/Backlog
			placementOptions := []string{"Backlog", "Sprint"}
			placementIdx, promptErr := promptSingleSelect("Placement", placementOptions)
			if promptErr != nil {
				return promptErr
			}
			if placementIdx == 1 {
				sprints, sprintErr := fetchSprints(boardID, c)
				if sprintErr != nil {
					return fmt.Errorf("failed to fetch sprints: %w", sprintErr)
				}
				if len(sprints) == 0 {
					_, _ = fmt.Fprintf(os.Stdout, "  No sprints found.\n")
				} else {
					sprintID, _, selectErr := promptSprintSelection(sprints)
					if selectErr != nil {
						return selectErr
					}
					req.SprintID = &sprintID
					hasChanges = true
				}
			} else {
				backlogID, resolveErr := resolveBacklogID(boardID, c)
				if resolveErr != nil {
					return fmt.Errorf("failed to resolve backlog: %w", resolveErr)
				}
				req.BacklogID = &backlogID
				hasChanges = true
			}

		case 9: // Project
			projects, fetchErr := fetchProjects(boardID, c)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch projects: %w", fetchErr)
			}
			if len(projects) == 0 {
				_, _ = fmt.Fprintf(os.Stdout, "  No projects found on this board.\n")
			} else {
				projectOptions := make([]SelectOption, len(projects))
				for i, p := range projects {
					projectOptions[i] = SelectOption{
						Display: p.Name,
						Value:   p.ID,
					}
				}
				projectID, selectErr := promptSingleSelectWithValues("Project", projectOptions)
				if selectErr != nil {
					return selectErr
				}
				req.ProjectID = &projectID
				hasChanges = true
			}

		case 10: // Story Points
			points, promptErr := promptInt("Story Points")
			if promptErr != nil {
				return promptErr
			}
			req.Weight = &points
			hasChanges = true

		case 11: // Due Date
			dueDate, promptErr := promptDate("Due Date")
			if promptErr != nil {
				return promptErr
			}
			if dueDate != "" {
				req.DueDate = &dueDate
				hasChanges = true
			}

		case 12: // Percentage
			pct, promptErr := promptInt("Percentage")
			if promptErr != nil {
				return promptErr
			}
			req.Percentage = &pct
			hasChanges = true
		}

		fmt.Println()
	}

	if !hasChanges {
		fmt.Println("No changes selected.")
		return nil
	}

	return submitTicketUpdate(boardID, ticketID, req, c)
}

// ---------------------------------------------------------------------------
// ticket delete
// ---------------------------------------------------------------------------

var ticketDeleteCmd = &cobra.Command{
	Use:   "delete [ticketKey]",
	Short: "Delete a ticket by key (e.g. SEN-42) or browse interactively",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketDelete,
}

func runTicketDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string
	ticketDisplay := ""

	if len(args) > 0 {
		ticketDisplay = args[0]
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
		ticketDisplay = ticketID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
	}

	if isInteractive() {
		confirmed, _ := promptYesNo(fmt.Sprintf("Delete ticket %s", ticketDisplay))
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s", boardID, ticketID)
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete ticket: %w", err)
	}

	fmt.Printf("Deleted ticket %s\n", ticketDisplay)
	return nil
}

// ---------------------------------------------------------------------------
// ticket restore
// ---------------------------------------------------------------------------

var ticketRestoreCmd = &cobra.Command{
	Use:   "restore [ticketKey]",
	Short: "Restore a deleted ticket by key (e.g. SEN-42) or browse interactively",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketRestore,
}

func runTicketRestore(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string

	if len(args) > 0 {
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/restore", boardID, ticketID)
	_, err = c.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to restore ticket: %w", err)
	}

	fmt.Printf("Restored ticket %s\n", ticketID)
	return nil
}

// ---------------------------------------------------------------------------
// ticket move
// ---------------------------------------------------------------------------

var ticketMoveCmd = &cobra.Command{
	Use:   "move [ticketKey]",
	Short: "Move a ticket to another board, sprint, or backlog",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketMove,
}

func runTicketMove(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string

	if len(args) > 0 {
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
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

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/move", boardID, ticketID)
	_, err = c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to move ticket: %w", err)
	}

	fmt.Printf("Moved ticket %s\n", ticketID)
	return nil
}

// ---------------------------------------------------------------------------
// ticket bulk-move
// ---------------------------------------------------------------------------

var ticketBulkMoveCmd = &cobra.Command{
	Use:   "bulk-move",
	Short: "Move multiple tickets at once",
	Args:  cobra.NoArgs,
	RunE:  runTicketBulkMove,
}

func runTicketBulkMove(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
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
	Use:   "order [ticketKey]",
	Short: "Reorder a ticket within a sprint or backlog",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTicketOrder,
}

func runTicketOrder(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	var ticketID string

	if len(args) > 0 {
		ticketID, err = resolveTicketByKey(boardID, args[0], c)
		if err != nil {
			return err
		}
	} else if isInteractive() {
		var selectedBoardID string
		selectedBoardID, ticketID, err = browseAndSelectTicket(boardID, c)
		if err != nil {
			return nil // user cancelled
		}
		boardID = selectedBoardID
	} else {
		return fmt.Errorf("ticket key is required in non-interactive mode")
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

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/order", boardID, ticketID)
	_, err = c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to reorder ticket: %w", err)
	}

	fmt.Printf("Reordered ticket %s to position %d\n", ticketID, order)
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
	ticketGetCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// ticket create
	ticketCmd.AddCommand(ticketCreateCmd)
	ticketCreateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketCreateCmd.Flags().String("title", "", "Ticket title (required)")
	ticketCreateCmd.Flags().String("type", "", "Ticket type: TASK or INCIDENT (required)")
	ticketCreateCmd.Flags().String("priority", "", "Priority: LOWEST, LOW, MEDIUM, HIGH, HIGHEST (task) or P1, P2, P3 (incident)")
	ticketCreateCmd.Flags().String("status", "", "Status: TODO, IN_PROGRESS, IN_REVIEW, or DONE (required)")
	ticketCreateCmd.Flags().String("description", "", "Ticket description")
	ticketCreateCmd.Flags().String("assignee", "", "Assignee UUIDs (comma-separated)")
	ticketCreateCmd.Flags().String("label", "", "Label UUIDs (comma-separated)")
	ticketCreateCmd.Flags().String("project", "", "Project ID")
	ticketCreateCmd.Flags().String("sprint", "", "Sprint name (or 'sprint' for active/latest)")
	ticketCreateCmd.Flags().String("backlog", "", "Backlog ID")
	ticketCreateCmd.Flags().Int("story-points", 0, "Story points (weight)")
	ticketCreateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	// Note: title, type, priority, status are no longer marked as required.
	// When not provided via flags and stdin is a terminal, interactive mode is used.

	// ticket update
	ticketCmd.AddCommand(ticketUpdateCmd)
	ticketUpdateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketUpdateCmd.Flags().String("title", "", "New title")
	ticketUpdateCmd.Flags().String("description", "", "New description")
	ticketUpdateCmd.Flags().String("type", "", "New type: TASK or INCIDENT")
	ticketUpdateCmd.Flags().String("status", "", "New status: TODO, IN_PROGRESS, IN_REVIEW, or DONE")
	ticketUpdateCmd.Flags().String("priority", "", "New priority: LOWEST, LOW, MEDIUM, HIGH, HIGHEST (task) or P1, P2, P3 (incident)")
	ticketUpdateCmd.Flags().String("sprint", "", "Sprint name (or 'sprint' for active/latest)")
	ticketUpdateCmd.Flags().String("backlog", "", "Backlog ID")
	ticketUpdateCmd.Flags().String("project", "", "Project ID")
	ticketUpdateCmd.Flags().String("assignee", "", "Assignee UUIDs (comma-separated)")
	ticketUpdateCmd.Flags().String("label", "", "Label UUIDs (comma-separated)")
	ticketUpdateCmd.Flags().Int("story-points", 0, "Story points (weight)")
	ticketUpdateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	ticketUpdateCmd.Flags().Int("percentage", 0, "Completion percentage")

	// ticket delete
	ticketCmd.AddCommand(ticketDeleteCmd)
	ticketDeleteCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// ticket restore
	ticketCmd.AddCommand(ticketRestoreCmd)
	ticketRestoreCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// ticket move
	ticketCmd.AddCommand(ticketMoveCmd)
	ticketMoveCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketMoveCmd.Flags().String("target-board", "", "Target board name or ID")
	ticketMoveCmd.Flags().String("target-sprint", "", "Target sprint ID")
	ticketMoveCmd.Flags().String("target-backlog", "", "Target backlog ID")

	// ticket bulk-move
	ticketCmd.AddCommand(ticketBulkMoveCmd)
	ticketBulkMoveCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketBulkMoveCmd.Flags().String("tickets", "", "Comma-separated ticket IDs (required)")
	ticketBulkMoveCmd.Flags().String("target-sprint", "", "Target sprint ID")
	ticketBulkMoveCmd.Flags().String("target-backlog", "", "Target backlog ID")

	// ticket order
	ticketCmd.AddCommand(ticketOrderCmd)
	ticketOrderCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	ticketOrderCmd.Flags().Int("order", 0, "New display order (required)")
	ticketOrderCmd.Flags().String("sprint", "", "Sprint ID context")
	ticketOrderCmd.Flags().String("backlog", "", "Backlog ID context")
	_ = ticketOrderCmd.MarkFlagRequired("order")
}
