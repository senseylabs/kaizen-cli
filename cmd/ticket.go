package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	var resp client.APIResponse[[]client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	// Print context header
	fmt.Println(header)
	fmt.Println()

	if len(resp.Data) == 0 {
		fmt.Println("No tickets found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE\tLOCATION")
	for _, t := range resp.Data {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		location := "—"
		if t.Sprint != nil {
			location = t.Sprint.Name
		} else if t.Backlog != nil {
			location = "Backlog"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee, location)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d tickets\n", len(resp.Data))

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	var resp client.APIResponse[[]client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No tickets found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE\tLOCATION")
	for _, t := range resp.Data {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		location := "—"
		if t.Sprint != nil {
			location = t.Sprint.Name
		} else if t.Backlog != nil {
			location = "Backlog"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee, location)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d tickets\n", len(resp.Data))

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	var resp client.APIResponse[[]client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse tickets response: %w", err)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No tickets assigned to you.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tASSIGNEE\tLOCATION")
	for _, t := range resp.Data {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		location := "—"
		if t.Sprint != nil {
			location = t.Sprint.Name
		} else if t.Backlog != nil {
			location = "Backlog"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.Key, title, t.Status, t.Priority, assignee, location)
	}
	_ = w.Flush()

	fmt.Printf("\nShowing %d tickets\n", len(resp.Data))

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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
	Example: `  # Interactive
  kaizen ticket create

  # With flags, attaching two local files
  kaizen ticket create --title "Login crash" --type TASK --priority HIGH --status TODO \
    --attach ./screenshot.png --attach ./server.log`,
	RunE: runTicketCreate,
}

func runTicketCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	// Attachments are validated first — before any network call and well before
	// the ticket POST — so a bad path fails fast and never leaves an orphaned
	// ticket behind.
	rawAttachments, _ := cmd.Flags().GetStringArray("attach")
	attachPaths, err := collectAttachmentPaths(rawAttachments)
	if err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")

	// If title is not provided and we're in an interactive terminal, enter interactive mode
	if title == "" && isInteractive() && term.IsTerminal(int(os.Stdout.Fd())) {
		return runTicketCreateInteractive(cmd, boardID, attachPaths, c)
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

	return submitTicketCreate(cmd, boardID, req, attachPaths, c)
}

// submitTicketCreate sends the create request, uploads any attachments, and
// prints the result.
//
// Ordering is deliberate: the ticket must exist before its attachments can be
// uploaded, because the storage API needs the new ticket's id as entityId. That
// means the upload can fail after the ticket is already committed, so the
// partial-failure path below reports the ticket key explicitly and still exits
// non-zero rather than swallowing the upload error.
func submitTicketCreate(cmd *cobra.Command, boardID string, req client.TicketCreateRequest, attachPaths []string, c *client.KaizenClient) error {
	// Last gate before the irreversible step. Sited here rather than alongside
	// the local --attach validation because it also has to cover the paths the
	// interactive flow collects, which are only known at this point.
	if err := preflightAttachments(attachPaths, c); err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets", boardID)
	body, err := c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to create ticket: %w", err)
	}

	// Without attachments the raw server body is still echoed verbatim, so
	// existing --json consumers see byte-identical output.
	if len(attachPaths) == 0 && cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Ticket]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse ticket response: %w", err)
	}

	if len(attachPaths) == 0 {
		fmt.Printf("\nCreated ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
		return nil
	}

	attachments, uploadErr := c.UploadAttachments(client.StorageEntityTypeTicket, resp.Data.ID, attachPaths)

	if cfgJSON {
		if printErr := printTicketCreateJSON(body, attachments, attachPaths, uploadErr); printErr != nil {
			// Join rather than return printErr alone: when the upload failed AND
			// rendering its JSON failed, uploadErr is the cause the user actually
			// needs and dropping it here would hide it completely.
			return errors.Join(uploadErr, printErr)
		}
	} else {
		fmt.Printf("\nCreated ticket %s: %s\n", resp.Data.Key, resp.Data.Title)
		if uploadErr == nil {
			fmt.Printf("Attached %d file(s): %s\n", len(attachments), strings.Join(attachmentFileNames(attachments), ", "))
		}
	}

	if uploadErr != nil {
		// The ticket is committed and only the upload failed. Spell that out on
		// stderr (stdout may be machine-read JSON) so the user does not assume
		// the whole command rolled back, then return the error to keep the exit
		// code non-zero. SilenceUsage stops cobra dumping the help text over a
		// message that is not a usage problem.
		cmd.SilenceUsage = true
		_, _ = fmt.Fprintf(os.Stderr, "\nTicket %s WAS created and is not lost — only the attachment upload failed.\n", resp.Data.Key)
		_, _ = fmt.Fprintf(os.Stderr, "Not attached (%d): %s\n", len(attachPaths), strings.Join(attachmentBaseNames(attachPaths), ", "))
		return fmt.Errorf("ticket %s created, but uploading its attachments failed: %w", resp.Data.Key, uploadErr)
	}

	return nil
}

// printTicketCreateJSON re-emits the server envelope with the attachment
// outcome merged in. The original keys are copied as raw JSON so the ticket
// payload survives untouched, and the failure path still produces a single
// valid JSON document on stdout.
func printTicketCreateJSON(body []byte, attachments []client.Attachment, attachPaths []string, uploadErr error) error {
	envelope := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("failed to parse ticket response: %w", err)
	}

	uploaded := attachments
	if uploaded == nil {
		uploaded = []client.Attachment{}
	}
	uploadedJSON, err := json.Marshal(uploaded)
	if err != nil {
		return fmt.Errorf("failed to encode attachments for JSON output: %w", err)
	}
	envelope["attachments"] = uploadedJSON

	if uploadErr != nil {
		failureJSON, marshalErr := json.Marshal(map[string]interface{}{
			"message": uploadErr.Error(),
			"files":   attachmentBaseNames(attachPaths),
		})
		if marshalErr != nil {
			return fmt.Errorf("failed to encode attachment error for JSON output: %w", marshalErr)
		}
		envelope["attachmentError"] = failureJSON
	}

	out, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to encode ticket response: %w", err)
	}

	fmt.Println(string(out))
	return nil
}

// attachmentFileNames lists the server-recorded names of uploaded attachments.
func attachmentFileNames(attachments []client.Attachment) []string {
	names := make([]string, len(attachments))
	for i, a := range attachments {
		names[i] = a.FileName
	}
	return names
}

// attachmentBaseNames lists the file names behind the supplied local paths.
func attachmentBaseNames(paths []string) []string {
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = filepath.Base(path)
	}
	return names
}

// collectAttachmentPaths normalises raw --attach values (trimming, expanding a
// leading ~) and validates every one of them against the storage API's limits.
// Returning an error here aborts before the ticket is created.
func collectAttachmentPaths(raw []string) ([]string, error) {
	paths := make([]string, 0, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		expanded, err := expandHomePath(trimmed)
		if err != nil {
			return nil, err
		}
		paths = append(paths, expanded)
	}

	if len(paths) == 0 {
		return nil, nil
	}

	if err := client.ValidateAttachmentPaths(paths); err != nil {
		return nil, err
	}

	return paths, nil
}

// preflightAttachments asks the server, per file, whether identical content is
// already stored, and refuses the whole command if so.
//
// The server's duplicate predicate is scoped to the entity TYPE, so a file
// byte-identical to one attached to any other ticket is rejected. That
// rejection would otherwise arrive from the upload, which by necessity runs
// after the ticket has been created -- leaving an orphaned ticket behind for a
// problem that was knowable up front. Naming the offending file here lets the
// user swap it out and re-run cleanly.
//
// The check fails OPEN, but only for errors that come from the SERVER. An
// unknown answer -- a transient 500, an exhausted 429 retry, or a deployment
// predating /storage/attachments/exists -- must not block ticket creation: this
// is a public Homebrew binary that can be pointed at any backend via --api-url
// or stored credentials, so turning unknown into a hard block would make
// --attach unusable against backends whose upload endpoint works fine.
// Deferring to the upload costs nothing there, since the upload is the
// authority on duplicates either way and submitTicketCreate already reports a
// post-create upload failure without losing the ticket.
//
// A LOCAL failure is the opposite case and still fails closed. Building the
// request body reads the file, so ErrAttachmentUnreadable means the file went
// away or lost its permissions since validation (a TOCTOU window: unmount,
// chmod, delete). That is a certainty, not an unknown -- the upload would fail
// on the very same file moments later, stranding the ticket that this check
// exists to protect. Rule 10 is satisfied on both branches: the error is either
// returned or surfaced on stderr, never swallowed.
func preflightAttachments(paths []string, c *client.KaizenClient) error {
	for _, path := range paths {
		exists, err := c.CheckAttachmentExists(client.StorageEntityTypeTicket, path)
		if err != nil {
			if errors.Is(err, client.ErrAttachmentUnreadable) {
				return fmt.Errorf("attachment %q can no longer be read: %w", filepath.Base(path), err)
			}
			_, _ = fmt.Fprintf(os.Stderr, "warning: could not pre-check attachment %q for duplicates (%v); continuing\n",
				filepath.Base(path), err)
			continue
		}
		if exists {
			return fmt.Errorf("attachment %q has the same content as a file already attached to a ticket; the server rejects duplicate content across all tickets, so this upload would fail", filepath.Base(path))
		}
	}
	return nil
}

// expandHomePath resolves a leading ~ so paths typed at an interactive prompt
// (where no shell expansion happens) behave like paths passed on the command line.
func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve the home directory for %q: %w", path, err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// promptAttachments collects optional attachment paths, one per line, until the
// user submits an empty line. Each entry is validated against the already
// accepted ones so batch-level limits (total size, duplicates) apply too; an
// invalid entry is reported and re-prompted instead of aborting the flow.
func promptAttachments() ([]string, error) {
	want, err := promptYesNo("Attachments")
	if err != nil {
		return nil, err
	}
	if !want {
		return nil, nil
	}

	var paths []string
	for {
		value, promptErr := promptText("File path (empty to finish)")
		if promptErr != nil {
			return nil, promptErr
		}
		if strings.TrimSpace(value) == "" {
			return paths, nil
		}

		candidate := make([]string, 0, len(paths)+1)
		candidate = append(candidate, paths...)
		candidate = append(candidate, value)

		accepted, validateErr := collectAttachmentPaths(candidate)
		if validateErr != nil {
			_, _ = fmt.Fprintf(os.Stdout, "  %v\n", validateErr)
			continue
		}
		paths = accepted
	}
}

// runTicketCreateInteractive prompts the user for all ticket fields interactively.
// attachPaths carries any already-validated --attach values so flags and prompts
// can be combined in one invocation.
func runTicketCreateInteractive(cmd *cobra.Command, boardID string, attachPaths []string, c *client.KaizenClient) error {
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

	// Attachments (optional). Skipped entirely when --attach already supplied
	// files, so the flag and the prompt never fight over the same list.
	if len(attachPaths) == 0 {
		prompted, attachErr := promptAttachments()
		if attachErr != nil {
			return attachErr
		}
		attachPaths = prompted
	}

	return submitTicketCreate(cmd, boardID, req, attachPaths, c)
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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

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
	ticketCreateCmd.Flags().StringArrayP("attach", "a", nil, "Attach a local file to the new ticket; repeat for multiple files (max 12MB per file and per upload)")
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
