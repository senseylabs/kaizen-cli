package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// fetchSprints fetches sprints for a board, using the same cache key pattern as sprint.go.
func fetchSprints(boardID string, c *client.KaizenClient) ([]client.Sprint, error) {
	cacheKey := fmt.Sprintf("sprints:%s", boardID)
	sprintsTTL := 15 * time.Minute

	// Try cache first
	if cached, ok := cache.Get(cacheKey, sprintsTTL); ok {
		var sprints []client.Sprint
		if json.Unmarshal(cached, &sprints) == nil {
			return sprints, nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/sprints", boardID)
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sprints: %w", err)
	}

	var resp client.APIResponse[[]client.Sprint]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse sprints response: %w", err)
	}

	// Cache the sprints
	_ = cache.Set(cacheKey, resp.Data)

	return resp.Data, nil
}

// resolveSprint fetches sprints for a board and resolves based on the rules:
//   - sprintName="" → return "", "", nil (use backlog)
//   - sprintName="sprint" (keyword only) → find ACTIVE sprint; if none active, use latest sprint (by createdAt)
//   - sprintName="Sprint 1" → find by name, prefer ACTIVE, fallback to first match
//
// Returns (sprintID, sprintDisplayName, error).
func resolveSprint(boardID string, sprintName string, c *client.KaizenClient) (string, string, error) {
	if sprintName == "" {
		return "", "", nil
	}

	sprints, err := fetchSprints(boardID, c)
	if err != nil {
		return "", "", err
	}

	if len(sprints) == 0 {
		return "", "", fmt.Errorf("no sprints found on this board")
	}

	// Keyword "sprint" (no specific name) → find ACTIVE, fallback to latest
	if strings.EqualFold(sprintName, "sprint") {
		for _, s := range sprints {
			if s.Status == "ACTIVE" {
				return s.ID, fmt.Sprintf("%s (%s)", s.Name, s.Status), nil
			}
		}
		// No active sprint — use latest by createdAt (sprints are typically ordered, take the last one)
		latest := sprints[0]
		for _, s := range sprints[1:] {
			if s.CreatedAt > latest.CreatedAt {
				latest = s
			}
		}
		return latest.ID, fmt.Sprintf("%s (%s)", latest.Name, latest.Status), nil
	}

	// Specific name provided → find by name, prefer ACTIVE, fallback to first match
	var firstMatch *client.Sprint
	for i := range sprints {
		if strings.EqualFold(sprints[i].Name, sprintName) {
			if sprints[i].Status == "ACTIVE" {
				return sprints[i].ID, fmt.Sprintf("%s (%s)", sprints[i].Name, sprints[i].Status), nil
			}
			if firstMatch == nil {
				firstMatch = &sprints[i]
			}
		}
	}

	if firstMatch != nil {
		return firstMatch.ID, fmt.Sprintf("%s (%s)", firstMatch.Name, firstMatch.Status), nil
	}

	// No match found — list available sprint names
	names := make([]string, len(sprints))
	for i, s := range sprints {
		names[i] = s.Name
	}
	return "", "", fmt.Errorf("sprint %q not found. Available sprints: %s", sprintName, strings.Join(names, ", "))
}

// resolveBacklogID fetches the board's backlog ID.
func resolveBacklogID(boardID string, c *client.KaizenClient) (string, error) {
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

// sprintStatusOrder returns the sort priority for sprint statuses.
// ACTIVE first, then PLANNED, then COMPLETED, then everything else.
func sprintStatusOrder(status string) int {
	switch status {
	case "ACTIVE":
		return 0
	case "PLANNED":
		return 1
	case "COMPLETED":
		return 2
	default:
		return 3
	}
}

// formatSprintDate converts a date string like "2025-06-15" to "Jun 15".
func formatSprintDate(dateStr *string) string {
	if dateStr == nil {
		return "\u2014"
	}
	t, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		return *dateStr
	}
	return t.Format("Jan 02")
}

// statusColor returns the ANSI color code for a sprint status.
func statusColor(status string) string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return ""
	}
	switch status {
	case "ACTIVE":
		return "\033[32m" // green
	case "PLANNED":
		return "\033[33m" // yellow
	case "COMPLETED":
		return "\033[90m" // dim/gray
	default:
		return ""
	}
}

// statusReset returns the ANSI reset code if the terminal supports colors.
func statusReset() string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return ""
	}
	return "\033[0m"
}

// promptSprintSelection shows an interactive numbered list of sprints and lets the user pick one.
// Returns the selected sprint's ID and display name.
func promptSprintSelection(sprints []client.Sprint) (string, string, error) {
	// Sort: ACTIVE first, then PLANNED, then COMPLETED
	sorted := make([]client.Sprint, len(sprints))
	copy(sorted, sprints)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sprintStatusOrder(sorted[i].Status) < sprintStatusOrder(sorted[j].Status)
	})

	fmt.Println("Select a sprint:")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, s := range sorted {
		dateRange := fmt.Sprintf("%s \u2013 %s", formatSprintDate(s.StartDate), formatSprintDate(s.EndDate))
		color := statusColor(s.Status)
		reset := statusReset()
		_, _ = fmt.Fprintf(w, "  %d\t%s\t%s%s%s\t%s\n", i+1, s.Name, color, s.Status, reset, dateRange)
	}
	_ = w.Flush()
	fmt.Println()

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	for {
		_, _ = fmt.Fprintf(os.Stdout, "Enter number (or 'q' to quit): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if strings.EqualFold(input, "q") {
			return "", "", fmt.Errorf("user cancelled")
		}

		num, parseErr := strconv.Atoi(input)
		if parseErr != nil || num < 1 || num > len(sorted) {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(sorted))
			continue
		}

		selected := sorted[num-1]
		displayName := fmt.Sprintf("%s (%s)", selected.Name, selected.Status)
		return selected.ID, displayName, nil
	}
}

// uuidPattern matches standard UUID format.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID checks if a string looks like a UUID.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// resolveDefaultBoard resolves the board name from --board flag or default config.
func resolveDefaultBoard(cmd *cobra.Command, c *client.KaizenClient) (string, error) {
	boardName, _ := cmd.Flags().GetString("board")
	if boardName == "" {
		boardName = cfgDefaultBoard
	}
	if boardName == "" {
		return "", fmt.Errorf("board is required. Use --board or set a default board")
	}
	return cache.ResolveBoard(boardName, c)
}

// resolveTicketByKey resolves a ticket key (e.g. "SEN-42") to a ticket ID.
// If the input looks like a UUID, returns it as-is.
// Otherwise, searches for the ticket by key on the board.
func resolveTicketByKey(boardID string, key string, c *client.KaizenClient) (string, error) {
	if isUUID(key) {
		return key, nil
	}

	// Search by key
	params := url.Values{}
	params.Set("search", key)
	params.Set("page", "0")
	params.Set("amount", "50")

	path := fmt.Sprintf("/kaizen/boards/%s/tickets?%s", boardID, params.Encode())
	body, err := c.Get(path)
	if err != nil {
		return "", fmt.Errorf("failed to search for ticket %q: %w", key, err)
	}

	var resp client.APIResponse[client.PaginatedResponse[client.Ticket]]
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse ticket search response: %w", err)
	}

	for _, t := range resp.Data.Content {
		if strings.EqualFold(t.Key, key) {
			return t.ID, nil
		}
	}

	return "", fmt.Errorf("ticket %q not found on this board", key)
}

// promptTicketSelection shows a numbered list of tickets and lets the user pick one.
// Returns the selected ticket's ID.
func promptTicketSelection(tickets []client.Ticket, totalPages int, currentPage int) (string, error) {
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	reset := promptReset()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, t := range tickets {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = t.Assignees[0].FirstName + " " + t.Assignees[0].LastName
		}
		title := t.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		_, _ = fmt.Fprintf(w, "  %s%d%s\t%s\t%s\t%s\t%s\t%s\n", dim, i+1, reset, t.Key, title, t.Status, t.Priority, assignee)
	}
	_ = w.Flush()
	fmt.Println()

	if totalPages > 1 {
		_, _ = fmt.Fprintf(os.Stdout, "%sPage %d/%d%s  ", dim, currentPage+1, totalPages, reset)
		if currentPage > 0 {
			_, _ = fmt.Fprintf(os.Stdout, "'p' prev  ")
		}
		if currentPage < totalPages-1 {
			_, _ = fmt.Fprintf(os.Stdout, "'n' next  ")
		}
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		_, _ = fmt.Fprintf(os.Stdout, "%sSelect ticket (or 'q' to quit):%s ", cyan, reset)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if strings.EqualFold(input, "q") {
			return "", fmt.Errorf("user cancelled")
		}
		if strings.EqualFold(input, "n") && currentPage < totalPages-1 {
			return "__next__", nil
		}
		if strings.EqualFold(input, "p") && currentPage > 0 {
			return "__prev__", nil
		}

		num, parseErr := strconv.Atoi(input)
		if parseErr != nil || num < 1 || num > len(tickets) {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(tickets))
			continue
		}

		return tickets[num-1].ID, nil
	}
}

// browseAndSelectTicket runs the full interactive flow:
// 1. "Where to look?" (Backlog/Sprint)
// 2. Fetch tickets
// 3. Show numbered list with pagination
// 4. User picks a ticket
// Returns (boardID, ticketID, error).
func browseAndSelectTicket(boardID string, c *client.KaizenClient) (string, string, error) {
	idx, err := promptSingleSelect("Where to look?", []string{"Backlog", "Sprint"})
	if err != nil {
		return "", "", fmt.Errorf("user cancelled")
	}

	params := url.Values{}
	params.Set("amount", "20")

	header := "Backlog"

	if idx == 0 {
		// Backlog
		backlogID, resolveErr := resolveBacklogID(boardID, c)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		params.Set("backlogId", backlogID)
	} else {
		// Sprint
		sprints, fetchErr := fetchSprints(boardID, c)
		if fetchErr != nil {
			return "", "", fmt.Errorf("failed to fetch sprints: %w", fetchErr)
		}
		if len(sprints) == 0 {
			return "", "", fmt.Errorf("no sprints found on this board")
		}
		sprintID, displayName, selectErr := promptSprintSelection(sprints)
		if selectErr != nil {
			return "", "", fmt.Errorf("user cancelled")
		}
		params.Set("sprintId", sprintID)
		header = fmt.Sprintf("Sprint: %s", displayName)
	}

	// Paginated ticket selection loop
	currentPage := 0
	for {
		params.Set("page", strconv.Itoa(currentPage))

		path := fmt.Sprintf("/kaizen/boards/%s/tickets?%s", boardID, params.Encode())
		body, fetchErr := c.Get(path)
		if fetchErr != nil {
			return "", "", fmt.Errorf("failed to fetch tickets: %w", fetchErr)
		}

		var resp client.APIResponse[client.PaginatedResponse[client.Ticket]]
		if parseErr := json.Unmarshal(body, &resp); parseErr != nil {
			return "", "", fmt.Errorf("failed to parse tickets: %w", parseErr)
		}

		if len(resp.Data.Content) == 0 {
			return "", "", fmt.Errorf("no tickets found in %s", header)
		}

		fmt.Println()
		fmt.Println(header)
		fmt.Println()

		ticketID, selectErr := promptTicketSelection(resp.Data.Content, resp.Data.TotalPages, currentPage)
		if selectErr != nil {
			return "", "", selectErr
		}

		if ticketID == "__next__" {
			currentPage++
			_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
			continue
		}
		if ticketID == "__prev__" {
			currentPage--
			_, _ = fmt.Fprintf(os.Stdout, "\033[2J\033[H")
			continue
		}

		return boardID, ticketID, nil
	}
}

// resolveSprintArg resolves sprint from positional args.
// If args empty + interactive → promptSprintSelection()
// If args provided → resolveSprint() by name
// Returns (sprintID, sprintDisplayName, error).
func resolveSprintArg(boardID string, args []string, c *client.KaizenClient) (string, string, error) {
	return resolveSprintArgFiltered(boardID, args, "", c)
}

// resolveSprintArgFiltered is like resolveSprintArg but only shows sprints with matching status.
// statusFilter can be "ACTIVE", "PLANNED", etc. Empty string means no filter.
func resolveSprintArgFiltered(boardID string, args []string, statusFilter string, c *client.KaizenClient) (string, string, error) {
	sprintName := strings.TrimSpace(strings.Join(args, " "))

	if sprintName != "" {
		return resolveSprint(boardID, sprintName, c)
	}

	if !isInteractive() {
		return "", "", fmt.Errorf("sprint name is required in non-interactive mode")
	}

	sprints, err := fetchSprints(boardID, c)
	if err != nil {
		return "", "", err
	}
	if len(sprints) == 0 {
		return "", "", fmt.Errorf("no sprints found on this board")
	}

	return promptSprintSelectionFiltered(sprints, statusFilter)
}

// promptSprintSelectionFiltered shows sprint list filtered by status.
func promptSprintSelectionFiltered(sprints []client.Sprint, statusFilter string) (string, string, error) {
	if statusFilter == "" {
		return promptSprintSelection(sprints)
	}

	var filtered []client.Sprint
	for _, s := range sprints {
		if strings.EqualFold(s.Status, statusFilter) {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return "", "", fmt.Errorf("no %s sprints found", statusFilter)
	}

	return promptSprintSelection(filtered)
}

// promptCommentSelection shows a numbered list of comments and lets user pick one.
// Returns the selected comment's ID.
func promptCommentSelection(comments []client.Comment) (string, error) {
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	reset := promptReset()

	fmt.Println("Select a comment:")
	fmt.Println()

	for i, comment := range comments {
		author := comment.AuthorFirstName + " " + comment.AuthorLastName
		content := comment.Content
		if len(content) > 60 {
			content = content[:57] + "..."
		}
		_, _ = fmt.Fprintf(os.Stdout, "  %s%d%s  %s (%s): %s\n", dim, i+1, reset, author, comment.CreatedAt, content)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		_, _ = fmt.Fprintf(os.Stdout, "%sSelect comment (or 'q' to quit):%s ", cyan, reset)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if strings.EqualFold(input, "q") {
			return "", fmt.Errorf("user cancelled")
		}

		num, parseErr := strconv.Atoi(input)
		if parseErr != nil || num < 1 || num > len(comments) {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(comments))
			continue
		}

		return comments[num-1].ID, nil
	}
}

// fetchComments fetches comments for a ticket.
func fetchComments(boardID string, ticketID string, c *client.KaizenClient) ([]client.Comment, error) {
	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments", boardID, ticketID)
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	var resp client.APIResponse[[]client.Comment]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse comments response: %w", err)
	}

	return resp.Data, nil
}

// promptPostTicketAction waits for user input after showing tickets.
// Returns "back" if user presses 'b', "quit" if 'q' or Ctrl+C.
func promptPostTicketAction() string {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "quit"
	}

	fmt.Println()
	_, _ = fmt.Fprintf(os.Stdout, "Press 'b' to go back to sprint list, or 'q' to quit: ")

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to buffered read
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if strings.EqualFold(input, "b") {
			return "back"
		}
		return "quit"
	}
	defer func() {
		_ = term.Restore(fd, oldState)
		fmt.Println() // newline after raw input
	}()

	buf := make([]byte, 1)
	for {
		_, readErr := os.Stdin.Read(buf)
		if readErr != nil {
			return "quit"
		}
		switch buf[0] {
		case 'b', 'B':
			return "back"
		case 'q', 'Q', 3: // 3 = Ctrl+C
			return "quit"
		}
	}
}
