package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Manage board backlogs",
}

var backlogGetCmd = &cobra.Command{
	Use:   "get [board]",
	Short: "Get the backlog for a board",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBacklogGet,
}

var backlogAddTicketCmd = &cobra.Command{
	Use:   "add-ticket <board> <ticketId>",
	Short: "Add a ticket to a board's backlog",
	Args:  cobra.ExactArgs(2),
	RunE:  runBacklogAddTicket,
}

func init() {
	rootCmd.AddCommand(backlogCmd)

	backlogCmd.AddCommand(backlogGetCmd)
	backlogCmd.AddCommand(backlogAddTicketCmd)
}

func resolveBoard(cmd *cobra.Command, args []string, c *client.KaizenClient) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return cache.ResolveBoard(args[0], c)
	}
	if cfgDefaultBoard != "" {
		return cache.ResolveBoard(cfgDefaultBoard, c)
	}
	return "", fmt.Errorf("board is required. Use a positional argument or set a default board")
}

func runBacklogGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := resolveBoard(cmd, args, c)
	if err != nil {
		return err
	}

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/backlog", boardID))
	if err != nil {
		return fmt.Errorf("failed to get backlog: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Backlog]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse backlog response: %w", err)
	}

	backlog := resp.Data

	if len(backlog.Tickets) == 0 {
		fmt.Println("Backlog is empty.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KEY\tTITLE\tSTATUS\tPRIORITY\tTYPE")
	for _, t := range backlog.Tickets {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Key, t.Title, t.Status, t.Priority, t.Type)
	}
	_ = w.Flush()

	return nil
}

func runBacklogAddTicket(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, cfgClientSecret, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	ticketID := args[1]

	_, err = c.Post(fmt.Sprintf("/kaizen/boards/%s/backlog/tickets/%s", boardID, ticketID), nil)
	if err != nil {
		return fmt.Errorf("failed to add ticket to backlog: %w", err)
	}

	fmt.Printf("Ticket %s added to backlog.\n", ticketID)
	return nil
}
