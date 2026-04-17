package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Parent command
// ---------------------------------------------------------------------------

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage ticket comments",
	Long:  "List, add, update, and delete comments on a Kaizen ticket.",
}

// ---------------------------------------------------------------------------
// comment list
// ---------------------------------------------------------------------------

var commentListCmd = &cobra.Command{
	Use:   "list [ticketKey]",
	Short: "List comments on a ticket",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCommentList,
}

func runCommentList(cmd *cobra.Command, args []string) error {
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

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments", boardID, ticketID)
	body, err := c.Get(path)
	if err != nil {
		return fmt.Errorf("failed to list comments: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[[]client.Comment]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse comments response: %w", err)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No comments found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tAUTHOR\tDATE\tCONTENT")
	for _, comment := range resp.Data {
		content := comment.Content
		if len(content) > 60 {
			content = content[:57] + "..."
		}
		author := comment.AuthorFirstName + " " + comment.AuthorLastName
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			comment.ID,
			author,
			comment.CreatedAt,
			content,
		)
	}
	_ = w.Flush()

	return nil
}

// ---------------------------------------------------------------------------
// comment add
// ---------------------------------------------------------------------------

var commentAddCmd = &cobra.Command{
	Use:   "add [ticketKey]",
	Short: "Add a comment to a ticket",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCommentAdd,
}

func runCommentAdd(cmd *cobra.Command, args []string) error {
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

	content, _ := cmd.Flags().GetString("content")
	if content == "" && isInteractive() {
		content, err = promptTextRequired("Comment")
		if err != nil {
			return err
		}
	}
	if content == "" {
		return fmt.Errorf("--content is required")
	}

	req := client.CommentCreateRequest{
		Content: content,
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments", boardID, ticketID)
	body, err := c.Post(path, req)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Comment]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse comment response: %w", err)
	}

	fmt.Printf("Added comment %s\n", resp.Data.ID)
	return nil
}

// ---------------------------------------------------------------------------
// comment update
// ---------------------------------------------------------------------------

var commentUpdateCmd = &cobra.Command{
	Use:   "update [ticketKey]",
	Short: "Update a comment",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCommentUpdate,
}

func runCommentUpdate(cmd *cobra.Command, args []string) error {
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

	commentID, _ := cmd.Flags().GetString("comment-id")
	if commentID == "" && isInteractive() {
		// Fetch comments and let user pick
		comments, fetchErr := fetchComments(boardID, ticketID, c)
		if fetchErr != nil {
			return fetchErr
		}
		if len(comments) == 0 {
			fmt.Println("No comments found on this ticket.")
			return nil
		}
		commentID, err = promptCommentSelection(comments)
		if err != nil {
			return nil // user cancelled
		}
	}
	if commentID == "" {
		return fmt.Errorf("--comment-id is required in non-interactive mode")
	}

	content, _ := cmd.Flags().GetString("content")
	if content == "" && isInteractive() {
		content, err = promptTextRequired("New content")
		if err != nil {
			return err
		}
	}
	if content == "" {
		return fmt.Errorf("--content is required")
	}

	req := client.CommentUpdateRequest{
		Content: content,
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments/%s", boardID, ticketID, commentID)
	body, err := c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Updated comment %s\n", commentID)
	return nil
}

// ---------------------------------------------------------------------------
// comment delete
// ---------------------------------------------------------------------------

var commentDeleteCmd = &cobra.Command{
	Use:   "delete [ticketKey]",
	Short: "Delete a comment",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCommentDelete,
}

func runCommentDelete(cmd *cobra.Command, args []string) error {
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

	commentID, _ := cmd.Flags().GetString("comment-id")
	if commentID == "" && isInteractive() {
		// Fetch comments and let user pick
		comments, fetchErr := fetchComments(boardID, ticketID, c)
		if fetchErr != nil {
			return fetchErr
		}
		if len(comments) == 0 {
			fmt.Println("No comments found on this ticket.")
			return nil
		}
		commentID, err = promptCommentSelection(comments)
		if err != nil {
			return nil // user cancelled
		}
	}
	if commentID == "" {
		return fmt.Errorf("--comment-id is required in non-interactive mode")
	}

	if isInteractive() {
		confirmed, _ := promptYesNo("Delete this comment")
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments/%s", boardID, ticketID, commentID)
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	fmt.Printf("Deleted comment %s\n", commentID)
	return nil
}

// ---------------------------------------------------------------------------
// init — register all subcommands
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(commentCmd)

	// comment list
	commentCmd.AddCommand(commentListCmd)
	commentListCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	// comment add
	commentCmd.AddCommand(commentAddCmd)
	commentAddCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	commentAddCmd.Flags().String("content", "", "Comment content")

	// comment update
	commentCmd.AddCommand(commentUpdateCmd)
	commentUpdateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	commentUpdateCmd.Flags().String("comment-id", "", "Comment ID to update")
	commentUpdateCmd.Flags().String("content", "", "New comment content")

	// comment delete
	commentCmd.AddCommand(commentDeleteCmd)
	commentDeleteCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	commentDeleteCmd.Flags().String("comment-id", "", "Comment ID to delete")
}
