package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/cache"
	"github.com/senseylabs/kaizen-cli/internal/client"
	"github.com/spf13/cobra"
)

var memberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage board members",
}

var memberListCmd = &cobra.Command{
	Use:   "list",
	Short: "List members of a board",
	Args:  cobra.NoArgs,
	RunE:  runMemberList,
}

var memberAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a member to a board",
	Args:  cobra.NoArgs,
	RunE:  runMemberAdd,
}

var memberRemoveCmd = &cobra.Command{
	Use:   "remove <userId>",
	Short: "Remove a member from a board",
	Args:  cobra.ExactArgs(1),
	RunE:  runMemberRemove,
}

var memberUpdateCmd = &cobra.Command{
	Use:   "update <userId>",
	Short: "Update a member's role",
	Args:  cobra.ExactArgs(1),
	RunE:  runMemberUpdate,
}

var memberSpecialtiesCmd = &cobra.Command{
	Use:   "specialties <userId>",
	Short: "Set a member's specialties",
	Args:  cobra.ExactArgs(1),
	RunE:  runMemberSpecialties,
}

func init() {
	rootCmd.AddCommand(memberCmd)

	memberCmd.AddCommand(memberListCmd)
	memberListCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	memberListCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh data")

	memberCmd.AddCommand(memberAddCmd)
	memberAddCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	memberAddCmd.Flags().String("user-id", "", "User ID to add (required)")
	memberAddCmd.Flags().String("role", "", "Member role (required)")

	memberCmd.AddCommand(memberRemoveCmd)
	memberRemoveCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")

	memberCmd.AddCommand(memberUpdateCmd)
	memberUpdateCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	memberUpdateCmd.Flags().String("role", "", "New role for the member")

	memberCmd.AddCommand(memberSpecialtiesCmd)
	memberSpecialtiesCmd.Flags().String("board", "", "Board name or ID (uses default if not set)")
	memberSpecialtiesCmd.Flags().String("specialties", "", "Comma-separated list of specialties")
}

func runMemberList(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	refresh, _ := cmd.Flags().GetBool("refresh")
	membersTTL := 15 * time.Minute
	cacheKey := fmt.Sprintf("members:%s", boardID)

	if !refresh {
		if cached, ok := cache.Get(cacheKey, membersTTL); ok {
			if cfgJSON {
				fmt.Println(string(cached))
				return nil
			}
			var members []client.BoardMember
			if err := json.Unmarshal(cached, &members); err == nil {
				printMemberTable(members)
				return nil
			}
		}
	}

	body, err := c.Get(fmt.Sprintf("/kaizen/boards/%s/members", boardID))
	if err != nil {
		return fmt.Errorf("failed to list members: %w", err)
	}

	var resp client.APIResponse[[]client.BoardMember]
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse members response: %w", err)
	}

	_ = cache.Set(cacheKey, resp.Data)

	if cfgJSON {
		out, _ := json.MarshalIndent(resp.Data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	printMemberTable(resp.Data)
	return nil
}

func printMemberTable(members []client.BoardMember) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tEMAIL\tROLE")
	for _, m := range members {
		name := fmt.Sprintf("%s %s", m.FirstName, m.LastName)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", name, m.Email, m.Role)
	}
	_ = w.Flush()
}

func runMemberAdd(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	userID, _ := cmd.Flags().GetString("user-id")
	role, _ := cmd.Flags().GetString("role")

	if userID == "" || role == "" {
		return fmt.Errorf("--user-id and --role are required")
	}

	payload := client.MemberAddRequest{
		UserID: userID,
		Role:   role,
	}

	body, err := c.Post(fmt.Sprintf("/kaizen/boards/%s/members", boardID), payload)
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("members:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Member %s added with role %s.\n", userID, role)
	return nil
}

func runMemberRemove(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	userID := args[0]

	if isInteractive() {
		confirmed, _ := promptYesNo(fmt.Sprintf("Remove member %s", userID))
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	_, err = c.Delete(fmt.Sprintf("/kaizen/boards/%s/members/%s", boardID, userID))
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("members:%s", boardID))

	fmt.Printf("Member %s removed.\n", userID)
	return nil
}

func runMemberUpdate(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	userID := args[0]

	role, _ := cmd.Flags().GetString("role")
	if role == "" {
		return fmt.Errorf("--role is required")
	}

	payload := client.MemberUpdateRequest{
		Role: role,
	}

	body, err := c.Put(fmt.Sprintf("/kaizen/boards/%s/members/%s", boardID, userID), payload)
	if err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("members:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Member %s role updated to %s.\n", userID, role)
	return nil
}

func runMemberSpecialties(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	c := client.NewKaizenClient(cfgAPIURL, cfgOrgID, resolveToken, cfgDebug)

	boardID, err := resolveDefaultBoard(cmd, c)
	if err != nil {
		return err
	}

	userID := args[0]

	specialtiesStr, _ := cmd.Flags().GetString("specialties")
	if specialtiesStr == "" {
		return fmt.Errorf("--specialties is required")
	}

	specialties := strings.Split(specialtiesStr, ",")
	for i := range specialties {
		specialties[i] = strings.TrimSpace(specialties[i])
	}

	body, err := c.Put(fmt.Sprintf("/kaizen/boards/%s/members/%s/specialties", boardID, userID), specialties)
	if err != nil {
		return fmt.Errorf("failed to update specialties: %w", err)
	}

	_ = cache.Delete(fmt.Sprintf("members:%s", boardID))

	if cfgJSON {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Specialties updated for member %s.\n", userID)
	return nil
}
