package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func newStatusCmd(stdout io.Writer) *cobra.Command {
	secretsDir := defaultSecretsDir()
	hostStateDir := defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "status [target]",
		Short: "Report workspace state and the suggested next command",
		Long: "Without a target, inspects the resolved state-dir and secrets-dir,\n" +
			"surfaces declared Environment/Provider/Infrastructure/OCPCluster\n" +
			"counts, reports which clusters have installer assets rendered, and\n" +
			"recommends the next command. With a target (e.g. `gitops <name>`),\n" +
			"reports drift between locally rendered artifacts and their sources.\n" +
			"Read-only.",
		Args: cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return runStatus(stdout, cf, secretsDir, hostStateDir)
	}
	cmd.AddCommand(newGitopsStatusCmd())
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func runStatus(stdout io.Writer, cf *commonFlags, secretsDir, hostStateDir string) error {
	printTitle(stdout, "status")

	printSubtitle(stdout, "workspace:")
	fmt.Fprintf(stdout, "  state-dir:        %s\n", cf.stateDir)
	fmt.Fprintf(stdout, "  secrets-dir:      %s\n", secretsDir)
	fmt.Fprintf(stdout, "  host-state-dir:   %s\n", hostStateDir)

	repo := bootstrapRepoDir(cf.stateDir)
	repoExists := dirExists(repo)
	if repoExists {
		printOK(stdout, "bootstrap repo", repo)
	} else {
		printFail(stdout, "bootstrap repo", repo+" (not initialized)")
	}

	state, loadErr := loadOptionalDesiredState(cf)
	stateLoaded := loadErr == nil && hasAnyState(state)
	source := stateSource(cf, repoExists)

	printSubtitle(stdout, "desired state:")
	fmt.Fprintf(stdout, "  source:           %s\n", source)
	switch {
	case loadErr != nil:
		printFail(stdout, "load", loadErr.Error())
	case !stateLoaded:
		printFail(stdout, "load", "no desired state found (run `gitups init workspace` or pass -f)")
	default:
		fmt.Fprintf(stdout, "  Environments:           %d\n", len(state.Environments))
		fmt.Fprintf(stdout, "  InfrastructureProviders: %d\n", len(state.InfrastructureProviders))
		fmt.Fprintf(stdout, "  ClusterInfrastructures: %d\n", len(state.ClusterInfrastructures))
		fmt.Fprintf(stdout, "  OCPClusters:            %d\n", len(state.OCPClusters))
	}

	if stateLoaded {
		printClusterStatus(stdout, state, cf.stateDir)
	}

	printSubtitle(stdout, "next:")
	for _, hint := range nextStepHints(repoExists, stateLoaded, state, cf.stateDir) {
		fmt.Fprintf(stdout, "  - %s\n", hint)
	}
	return nil
}

func printClusterStatus(stdout io.Writer, state v1alpha1.State, stateDir string) {
	if len(state.OCPClusters) == 0 {
		return
	}
	printSubtitle(stdout, "clusters:")
	names := make([]string, 0, len(state.OCPClusters))
	byName := map[string]v1alpha1.OCPCluster{}
	for _, ocp := range state.OCPClusters {
		names = append(names, ocp.Metadata.Name)
		byName[ocp.Metadata.Name] = ocp
	}
	sort.Strings(names)
	for _, name := range names {
		ocp := byName[name]
		role := ocp.Spec.Role
		if role == "" {
			role = "managed"
		}
		detail := fmt.Sprintf("role=%s topology=%s install=%s", role, ocp.Spec.Topology, ocp.Spec.Install.Method)
		printOK(stdout, name, detail)
		installer := filepath.Join(bootstrapRepoDir(stateDir), name, "openshift", "install-config.yaml")
		if fileExists(installer) {
			printOK(stdout, "  installer", installer)
		} else {
			printFail(stdout, "  installer", "not rendered")
		}
	}
}

func nextStepHints(repoExists, stateLoaded bool, state v1alpha1.State, stateDir string) []string {
	if !repoExists {
		return []string{"gitups init workspace --cluster-name <name> --provider <bare-metal|emulated-bare-metal|vsphere>"}
	}
	if !stateLoaded {
		return []string{
			"edit the scaffolded YAML under the bootstrap repo",
			"gitups check all",
		}
	}

	hints := []string{"gitups check bastion"}
	missingInstaller := clustersMissingInstaller(state, stateDir)
	if len(missingInstaller) > 0 {
		hints = append(hints,
			"gitups apply infra --dry-run",
			fmt.Sprintf("gitups render installer --scope %s", joinNames(missingInstaller)),
		)
		return hints
	}
	hints = append(hints,
		"gitups apply infra",
		"gitups apply clusters",
		"gitups apply hub",
	)
	return hints
}

func clustersMissingInstaller(state v1alpha1.State, stateDir string) []string {
	var missing []string
	for _, ocp := range state.OCPClusters {
		path := filepath.Join(bootstrapRepoDir(stateDir), ocp.Metadata.Name, "openshift", "install-config.yaml")
		if !fileExists(path) {
			missing = append(missing, ocp.Metadata.Name)
		}
	}
	sort.Strings(missing)
	return missing
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ","
		}
		out += n
	}
	return out
}

func stateSource(cf *commonFlags, repoExists bool) string {
	if len(cf.files) > 0 {
		return fmt.Sprintf("--file %v", cf.files)
	}
	if repoExists {
		return bootstrapRepoDir(cf.stateDir) + "/*/gitups"
	}
	return "(none)"
}

func hasAnyState(s v1alpha1.State) bool {
	return len(s.Environments)+len(s.InfrastructureProviders)+len(s.ClusterInfrastructures)+len(s.OCPClusters) > 0
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return false
	}
	return !info.IsDir()
}
