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
	Use:   "list <board> <ticketId>",
	Short: "List comments on a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runCommentList,
}

func runCommentList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments", boardID, args[1])
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
	fmt.Fprintln(w, "ID\tAUTHOR\tDATE\tCONTENT")
	for _, comment := range resp.Data {
		content := comment.Content
		if len(content) > 60 {
			content = content[:57] + "..."
		}
		author := comment.AuthorFirstName + " " + comment.AuthorLastName
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			comment.ID,
			author,
			comment.CreatedAt.Format("2006-01-02 15:04"),
			content,
		)
	}
	w.Flush()

	return nil
}

// ---------------------------------------------------------------------------
// comment add
// ---------------------------------------------------------------------------

var commentAddCmd = &cobra.Command{
	Use:   "add <board> <ticketId>",
	Short: "Add a comment to a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runCommentAdd,
}

func runCommentAdd(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	content, _ := cmd.Flags().GetString("content")
	req := client.CommentCreateRequest{
		Content: content,
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments", boardID, args[1])
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
	Use:   "update <board> <ticketId> <commentId>",
	Short: "Update a comment",
	Args:  cobra.ExactArgs(3),
	RunE:  runCommentUpdate,
}

func runCommentUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	content, _ := cmd.Flags().GetString("content")
	req := client.CommentUpdateRequest{
		Content: content,
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments/%s", boardID, args[1], args[2])
	body, err := c.Put(path, req)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Updated comment %s\n", args[2])
	return nil
}

// ---------------------------------------------------------------------------
// comment delete
// ---------------------------------------------------------------------------

var commentDeleteCmd = &cobra.Command{
	Use:   "delete <board> <ticketId> <commentId>",
	Short: "Delete a comment",
	Args:  cobra.ExactArgs(3),
	RunE:  runCommentDelete,
}

func runCommentDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/kaizen/boards/%s/tickets/%s/comments/%s", boardID, args[1], args[2])
	_, err = c.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	fmt.Printf("Deleted comment %s\n", args[2])
	return nil
}

// ---------------------------------------------------------------------------
// init — register all subcommands
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(commentCmd)

	// comment list
	commentCmd.AddCommand(commentListCmd)

	// comment add
	commentCmd.AddCommand(commentAddCmd)
	commentAddCmd.Flags().String("content", "", "Comment content (required)")
	_ = commentAddCmd.MarkFlagRequired("content")

	// comment update
	commentCmd.AddCommand(commentUpdateCmd)
	commentUpdateCmd.Flags().String("content", "", "New comment content (required)")
	_ = commentUpdateCmd.MarkFlagRequired("content")

	// comment delete
	commentCmd.AddCommand(commentDeleteCmd)
}
