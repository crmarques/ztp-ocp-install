package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// newRootCmd assembles the gitups command tree. SilenceUsage/SilenceErrors
// keep cobra from printing its own banners; Run() formats and prints
// errors itself so test assertions on stderr stay deterministic.
func newRootCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "gitups",
		Short: "GitOps-driven OpenShift fleet provisioning",
		Long: "Gitups renders, validates, and converges versioned desired-state YAML\n" +
			"to drive OpenShift hub and managed-cluster lifecycle.",
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
		newValidateCmd(stdout, stderr),
		newPlanCmd(stdout),
		newRenderCmd(stdout),
		newApplyCmd(stdin, stdout, stderr),
		newDestroyCmd(stdout, stderr),
		newStatusCmd(stdout),
		newDiffCmd(stdout),
		newRegistryCmd(stdout, stderr),
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
