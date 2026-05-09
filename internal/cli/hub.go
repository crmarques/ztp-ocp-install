package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newHubCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Install and configure hub-cluster components on clusters with role: hub",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newHubSubCmd("check", stdout),
		newHubSubCmd("apply", stdout),
		newHubSubCmd("destroy", stdout),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newHubSubCmd(action string, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   action,
		Short: "Reserved: hub-cluster " + action + " is not yet implemented",
		Args:  cobra.NoArgs,
	}
	_ = addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		printTitle(stdout, "hub "+action)
		fmt.Fprintln(stdout, "hub scope is reserved for clusters that declare role: hub; no hub components are implemented yet")
		return nil
	}
	return cmd
}
