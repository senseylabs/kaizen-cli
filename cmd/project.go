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

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage board projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list [board]",
	Short: "List projects for a board",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProjectList,
}

var projectGetCmd = &cobra.Command{
	Use:   "get <board> <projectId>",
	Short: "Get a project by ID",
	Args:  cobra.ExactArgs(2),
	RunE:  runProjectGet,
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [board]",
	Short: "Create a new project",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProjectCreate,
}

var projectUpdateCmd = &cobra.Command{
	Use:   "update <board> <projectId>",
	Short: "Update a project",
	Args:  cobra.ExactArgs(2),
	RunE:  runProjectUpdate,
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <board> <projectId>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(2),
	RunE:  runProjectDelete,
}

func init() {
	rootCmd.AddCommand(projectCmd)

	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectUpdateCmd)
	projectCmd.AddCommand(projectDeleteCmd)

	projectCreateCmd.Flags().String("name", "", "Project name (required)")
	projectCreateCmd.Flags().String("description", "", "Project description")
	projectCreateCmd.Flags().String("prefix", "", "Project prefix")

	projectUpdateCmd.Flags().String("name", "", "Project name")
	projectUpdateCmd.Flags().String("description", "", "Project description")
	projectUpdateCmd.Flags().String("prefix", "", "Project prefix")
}

func runProjectList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveBoard(cmd, args, c)
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
	fmt.Fprintln(w, "NAME\tCOLOR\tID")
	for _, p := range resp.Data {
		color := ""
		if p.Color != nil {
			color = *p.Color
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, color, p.ID)
	}
	w.Flush()

	return nil
}

func runProjectGet(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	projectID := args[1]

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

	boardID, err := resolveBoard(cmd, args, c)
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

	if cmd.Flags().Changed("prefix") {
		color, _ := cmd.Flags().GetString("prefix")
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

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	projectID := args[1]

	payload := client.ProjectUpdateRequest{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		payload.Name = &v
	}
	if cmd.Flags().Changed("prefix") {
		v, _ := cmd.Flags().GetString("prefix")
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

	boardID, err := cache.ResolveBoard(args[0], c)
	if err != nil {
		return err
	}

	projectID := args[1]

	_, err = c.Delete(fmt.Sprintf("/kaizen/boards/%s/projects/%s", boardID, projectID))
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	fmt.Printf("Project %s deleted.\n", projectID)
	return nil
}
