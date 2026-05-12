package cli

import "github.com/spf13/cobra"

// These top-level verb parents host gitops-only operations as `gitops <name>`
// children. Each parent exists so the verb sits at the top of `--help`
// alongside the provisioning verbs, rather than buried under a `gitops`
// namespace.

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

func newDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <target>",
		Short: "Tear down a previously applied target",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newGitopsDestroyCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}
