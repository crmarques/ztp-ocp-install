package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// preserves AddCommand order in --help so workflow commands render in usage order
func init() { cobra.EnableCommandSorting = false }

const (
	groupWorkflow = "workflow"
	groupGeneral  = "general"
)

func newRootCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "bootwright",
		Short: "GitOps-driven OpenShift fleet provisioning",
		Long: "Bootwright renders, validates, and converges versioned desired-state YAML\n" +
			"to drive OpenShift cluster lifecycle.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return failErr(2, err)
	})
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddGroup(
		&cobra.Group{ID: groupWorkflow, Title: "Workflow Commands:"},
		&cobra.Group{ID: groupGeneral, Title: "General Commands:"},
	)
	root.SetHelpCommandGroupID(groupGeneral)
	root.SetCompletionCommandGroupID(groupGeneral)

	addWorkflow(root,
		newInitCmd(stdout),
		newSecretCmd(stdout, stderr),
		newCheckCmd(stdout, stderr),
		newStatusCmd(stdout),
		newExpandCmd(),
		newFillCmd(),
		newPlanCmd(),
		newRenderCmd(stdout, stderr),
		newPushCmd(),
		newApplyCmd(stdin, stdout, stderr),
		newWaitCmd(),
		newDestroyCmd(stdin, stdout, stderr),
	)
	return root
}

func addWorkflow(parent *cobra.Command, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupWorkflow
		parent.AddCommand(c)
	}
}

type commonFlags struct {
	files    []string
	stateDir string
}

func addCommonFlags(cmd *cobra.Command) *commonFlags {
	cf := &commonFlags{stateDir: defaultStateDir()}
	cmd.Flags().StringArrayVarP(&cf.files, "file", "f", nil, "Bootwright YAML file or directory; may be repeated (default: <state-dir>/clusters-bootstrap.git/*/bootwright)")
	cmd.Flags().StringVar(&cf.stateDir, "state-dir", cf.stateDir, "generated state directory (env: BOOTWRIGHT_STATE_DIR)")
	return cf
}

func showSubcommandFlagsInHelp(cmd *cobra.Command) {
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		defaultHelp(c, args)
		if c != cmd {
			return
		}
		merged := pflag.NewFlagSet("subcommand", pflag.ContinueOnError)
		for _, sub := range c.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			sub.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if f.Name == "help" || merged.Lookup(f.Name) != nil {
					return
				}
				merged.AddFlag(f)
			})
		}
		usages := merged.FlagUsages()
		if usages == "" {
			return
		}
		fmt.Fprintf(c.OutOrStdout(), "\nSubcommand Flags:\n%s", usages)
	})
}
