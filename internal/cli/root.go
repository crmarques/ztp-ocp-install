package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newRootCmd assembles the gitups command tree. SilenceUsage/SilenceErrors
// keep cobra from printing its own banners; Run() formats and prints
// errors itself so test assertions on stderr stay deterministic.
func newRootCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "gitups",
		Short: "GitOps-driven OpenShift fleet provisioning",
		Long: "Gitups renders, validates, and converges versioned desired-state YAML\n" +
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

	root.AddCommand(
		newDoctorCmd(stdout, stderr),
		newSetupCmd(stdin, stdout, stderr),
		newInitCmd(stdout),
		newValidateCmd(stdout, stderr),
		newPreflightCmd(stdout, stderr),
		newPlanCmd(stdout),
		newApplyCmd(stdin, stdout, stderr),
		newDestroyCmd(stdin, stdout, stderr),
		newStatusCmd(stdout),
		newSecretsCmd(stdout, stderr),
	)
	return root
}

// commonFlags hold the -f and --state-dir pair shared by every subcommand.
type commonFlags struct {
	files    []string
	stateDir string
}

func addCommonFlags(cmd *cobra.Command) *commonFlags {
	stateDir := defaultStateDir()
	cf := &commonFlags{stateDir: stateDir}
	cmd.Flags().StringArrayVarP(&cf.files, "file", "f", nil, "Gitups YAML file or directory; may be repeated")
	cmd.Flags().StringVar(&cf.stateDir, "state-dir", stateDir, "generated state directory")
	return cf
}

// showSubcommandFlagsInHelp augments cmd's help so `<cmd> --help` lists the
// union of local flags from its subcommands. Cobra's default help on a
// command group only shows the parent's own flags, so a user running
// `gitups destroy --help` sees just `--help` and has to drill into a scope
// to discover `-f`, `--state-dir`, `--yes`, etc. Apply this only to groups
// whose subcommands share a coherent flag set; on heterogeneous groups
// (e.g. `secrets`) the union would be misleading.
func showSubcommandFlagsInHelp(cmd *cobra.Command) {
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		defaultHelp(c, args)
		// HelpFunc walks up the parent chain, so this closure also fires
		// for descendants. Only augment when help is requested on the
		// command we configured.
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
