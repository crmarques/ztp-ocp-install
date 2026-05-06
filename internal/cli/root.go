package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
