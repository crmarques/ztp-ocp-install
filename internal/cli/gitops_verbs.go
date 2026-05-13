package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func newExpandCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expand <target>",
		Short: "Resolve and freeze package sources into spec.resolved",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsExpandCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newFillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fill <target>",
		Short: "Fill placeholders in expanded artifacts",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsFillCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan <target>",
		Short: "Print an apply plan without touching the cluster",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsPlanCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <target>",
		Short: "Publish rendered artifacts to a remote",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsPushCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <target>",
		Short: "Block until cluster-side reconciliation succeeds",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsWaitCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newDestroyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <target>",
		Short: "Tear down a previously applied target",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		retargetCommand(newScopeDestroyCmd(infraScope, stdin, stdout, stderr), "infra", "Tear down infrastructure hosts and substrate"),
		retargetCommand(newScopeDestroyCmd(clustersScope, stdin, stdout, stderr), "clusters", "Tear down OpenShift cluster install state"),
		newGitopsDestroyCmd(),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}
