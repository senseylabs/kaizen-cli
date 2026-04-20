package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage board projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects for a board",
	Args:  cobra.NoArgs,
	RunE:  runProjectList,
}

var projectGetCmd = &cobra.Command{
	Use:   "get <projectId>",
	Short: "Get a project by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectGet,
}

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	Args:  cobra.NoArgs,
	RunE:  runProjectCreate,
}

var projectUpdateCmd = &cobra.Command{
	Use:   "update <projectId>",
	Short: "Update a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectUpdate,
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <projectId>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectDelete,
}

func init() {
	rootCmd.AddCommand(projectCmd)

	projectCmd.AddCommand(projectListCmd)
	projectListCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	projectCmd.AddCommand(projectGetCmd)
	projectGetCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	projectCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	projectCreateCmd.Flags().String("name", "", "Project name (required)")
	projectCreateCmd.Flags().String("description", "", "Project description")
	projectCreateCmd.Flags().String("color", "", "Project color")

	projectCmd.AddCommand(projectUpdateCmd)
	projectUpdateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	projectUpdateCmd.Flags().String("name", "", "Project name")
	projectUpdateCmd.Flags().String("description", "", "Project description")
	projectUpdateCmd.Flags().String("color", "", "Project color")

	projectCmd.AddCommand(projectDeleteCmd)
	projectDeleteCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
}

func runProjectList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/projects", boardID))
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	var resp client.APIResponse[[]client.Project]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse projects response: %w", err)
	}

	if cfgJSON {
		out, _ := json.MarshalIndent(resp.Data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tCOLOR\tID")
	for _, p := range resp.Data {
		color := ""
		if p.Color != nil {
			color = *p.Color
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, color, p.ID)
	}
	_ = w.Flush()

	return nil
}

func runProjectGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	projectID := args[0]

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/projects/%s", boardID, projectID))
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Project]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse project response: %w", err)
	}

	p := resp.Data
	fmt.Printf("Name:  %s\n", p.Name)
	fmt.Printf("ID:    %s\n", p.ID)
	if p.Color != nil {
		fmt.Printf("Color: %s\n", *p.Color)
	}

	return nil
}

func runProjectCreate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		return fmt.Errorf("--name is required")
	}

	payload := client.ProjectCreateRequest{
		Name: name,
	}

	if cmd.Flags().Changed("color") {
		color, _ := cmd.Flags().GetString("color")
		payload.Color = &color
	}

	body, err := c.Post(fmt.Sprintf("/kaizen/boards/%s/projects", boardID), payload)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	var resp client.APIResponse[client.Project]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Project created: %s (%s)\n", resp.Data.Name, resp.Data.ID)
	return nil
}

func runProjectUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	projectID := args[0]

	payload := client.ProjectUpdateRequest{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		payload.Name = &v
	}
	if cmd.Flags().Changed("color") {
		v, _ := cmd.Flags().GetString("color")
		payload.Color = &v
	}

	body, err := c.Put(fmt.Sprintf("/kaizen/boards/%s/projects/%s", boardID, projectID), payload)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Project %s updated.\n", projectID)
	return nil
}

func runProjectDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	projectID := args[0]

	if isInteractive() {
		confirmed, _ := promptYesNo(fmt.Sprintf("Delete project %s", projectID))
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	_, err = c.Delete(fmt.Sprintf("/kaizen/boards/%s/projects/%s", boardID, projectID))
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	fmt.Printf("Project %s deleted.\n", projectID)
	return nil
}
