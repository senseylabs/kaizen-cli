package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var labelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage board labels",
}

var labelListCmd = &cobra.Command{
	Use:   "list [board]",
	Short: "List labels for a board",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLabelList,
}

var labelCreateCmd = &cobra.Command{
	Use:   "create [board]",
	Short: "Create a new label",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLabelCreate,
}

var labelUpdateCmd = &cobra.Command{
	Use:   "update <board> <labelId>",
	Short: "Update a label",
	Args:  cobra.ExactArgs(2),
	RunE:  runLabelUpdate,
}

var labelDeleteCmd = &cobra.Command{
	Use:   "delete <board> <labelId>",
	Short: "Delete a label",
	Args:  cobra.ExactArgs(2),
	RunE:  runLabelDelete,
}

func init() {
	rootCmd.AddCommand(labelCmd)

	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelCreateCmd)
	labelCmd.AddCommand(labelUpdateCmd)
	labelCmd.AddCommand(labelDeleteCmd)

	labelListCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh data")

	labelCreateCmd.Flags().String("name", "", "Label name (required)")
	labelCreateCmd.Flags().String("color", "", "Label color")

	labelUpdateCmd.Flags().String("name", "", "Label name")
	labelUpdateCmd.Flags().String("color", "", "Label color")
}

func runLabelList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveBoard(cmd, args, c)
	if err != nil {
		return err
	}

	refresh, _ := cmd.Flags().GetBool("refresh")
	labelsTTL := 30 * time.Minute
	cacheKey := fmt.Sprintf("labels:%s", boardID)

	if !refresh {
		if cached, ok := cache.Get(cacheKey, labelsTTL); ok {
			if cfgJSON {
				fmt.Println(string(cached))
				return nil
			}
			var labels []client.Label
			if err := json.Unmarshal(cached, &labels); err == nil {
				printLabelTable(labels)
				return nil
			}
		}
	}

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/labels", boardID))
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	var resp client.APIResponse[[]client.Label]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse labels response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	if cfgJSON {
		out, _ := json.MarshalIndent(resp.Data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	printLabelTable(resp.Data)
	return nil
}

func printLabelTable(labels []client.Label) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOLOR")
	for _, l := range labels {
		color := ""
		if l.Color != nil {
			color = *l.Color
		}
		fmt.Fprintf(w, "%s\t%s\n", l.Name, color)
	}
	w.Flush()
}

func runLabelCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveBoard(cmd, args, c)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		return fmt.Errorf("--name is required")
	}

	payload := client.LabelCreateRequest{
		Name: name,
	}

	if cmd.Flags().Changed("color") {
		color, _ := cmd.Flags().GetString("color")
		payload.Color = &color
	}

	body, err := c.Post(fmt.Sprintf("/kaizen/boards/%s/labels", boardID), payload)
	if err != nil {
		return fmt.Errorf("failed to create label: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("labels:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Label]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Label created: %s (%s)\n", resp.Data.Name, resp.Data.ID)
	return nil
}

func runLabelUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	labelID := args[1]

	payload := client.LabelUpdateRequest{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		payload.Name = &v
	}
	if cmd.Flags().Changed("color") {
		v, _ := cmd.Flags().GetString("color")
		payload.Color = &v
	}

	body, err := c.Put(fmt.Sprintf("/kaizen/boards/%s/labels/%s", boardID, labelID), payload)
	if err != nil {
		return fmt.Errorf("failed to update label: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("labels:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Label %s updated.\n", labelID)
	return nil
}

func runLabelDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	labelID := args[1]

	_, err = c.Delete(fmt.Sprintf("/kaizen/boards/%s/labels/%s", boardID, labelID))
	if err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("labels:%s", boardID))

	fmt.Printf("Label %s deleted.\n", labelID)
	return nil
}
