package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Manage board backlogs",
}

var backlogGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get the backlog for a board",
	Args:  cobra.NoArgs,
	RunE:  runBacklogGet,
}

var backlogAddTicketCmd = &cobra.Command{
	Use:   "add-ticket <ticketId>",
	Short: "Add a ticket to a board's backlog",
	Args:  cobra.ExactArgs(1),
	RunE:  runBacklogAddTicket,
}

func init() {
	rootCmd.AddCommand(backlogCmd)

	backlogCmd.AddCommand(backlogGetCmd)
	backlogGetCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	backlogCmd.AddCommand(backlogAddTicketCmd)
	backlogAddTicketCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
}

func runBacklogGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
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

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	ticketID := args[0]

	_, err = c.Post(fmt.Sprintf("/kaizen/boards/%s/backlog/tickets/%s", boardID, ticketID), nil)
	if err != nil {
		return fmt.Errorf("failed to add ticket to backlog: %w", err)
	}

	fmt.Printf("Ticket %s added to backlog.\n", ticketID)
	return nil
}
