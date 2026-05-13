package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/ansible"
	"github.com/crmarques/gitups/internal/provisioning/render"
	"github.com/crmarques/gitups/internal/workflow"
)

func newCheckCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <target>",
		Short: "Validate desired state and prerequisites",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		retargetCommand(newBastionCheckCmd(stdout, stderr), "bastion", "Verify bastion dependencies"),
		retargetCommand(newScopeCheckCmd(infraScope, stdout, stderr), "infra", "Check infrastructure hosts and substrate"),
		retargetCommand(newScopeCheckCmd(clustersScope, stdout, stderr), "clusters", "Check cluster install prerequisites"),
		newHubCheckCmd(stdout),
		newGitopsCheckCmd(),
		newCheckAllCmd(stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newApplyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <target>",
		Short: "Apply a provisioning target",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		retargetCommand(newBastionApplyCmd(stdin, stdout, stderr), "bastion", "Install bastion prerequisites"),
		retargetCommand(newScopeApplyCmd(infraScope, stdin, stdout, stderr), "infra", "Converge infrastructure hosts and substrate"),
		retargetCommand(newScopeApplyCmd(clustersScope, stdin, stdout, stderr), "clusters", "Install OpenShift clusters"),
		newHubApplyCmd(stdout),
		newGitopsApplyCmd(),
		retargetCommand(newScopeApplyCmd(allScope, stdin, stdout, stderr), "all", "Apply infrastructure, OpenShift clusters, and hub components"),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newRenderCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render <target>",
		Short: "Render generated artifacts",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newRenderClusterInstallFilesCmd(stdout, stderr),
		newGitopsRenderCmd(),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func retargetCommand(cmd *cobra.Command, use, short string) *cobra.Command {
	cmd.Use = use
	cmd.Short = short
	return cmd
}

func newCheckAllCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		executable   string
		secretsDir   string
		hostStateDir string
		dryRun       bool
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "all",
		Short: "Check all provisioning prerequisites",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible preflight command without executing it")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "all check")
		if err := runBastionChecks(stdout, stderr, state, hostStateDir); err != nil {
			return err
		}
		if err := runScopeHostCheck(stdout, stderr, state, allScope.phases(), secretsDir, hostStateDir); err != nil {
			return err
		}
		if err := runHubCheck(stdout, state); err != nil {
			return err
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		_, err = workflow.Run(c.Context(), workflow.RunOptions{
			State:             state,
			StateDir:          cf.stateDir,
			SecretsDir:        secretsDir,
			HostStateDir:      hostStateDir,
			Executable:        executable,
			BundleDir:         bundleDir,
			Playbook:          "playbooks/checks/preflight.yml",
			ArtifactsBaseName: "preflight-all",
			DryRun:            dryRun,
			Label:             "all check",
		}, runner, stdout)
		if err != nil {
			return failErr(1, err)
		}
		return nil
	}
	return cmd
}

func newHubCheckCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Check hub cluster selection",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "hub check")
		return runHubCheck(stdout, state)
	}
	return cmd
}

func newHubApplyCmd(stdout io.Writer) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Apply reserved hub components",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the hub apply plan without executing it")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "hub apply")
		if err := runHubCheck(stdout, state); err != nil {
			return err
		}
		printHubComponentsPlan(stdout, dryRun)
		return nil
	}
	return cmd
}

func newRenderClusterInstallFilesCmd(stdout io.Writer, _ io.Writer) *cobra.Command {
	var (
		secretsDir     string
		clusterScope   string
		resolveSecrets bool
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "installer",
		Short: "Render install-config.yaml and agent-config.yaml",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&clusterScope, "scope", "", "comma-separated OCPCluster names to render")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
	cmd.Flags().BoolVar(&resolveSecrets, "resolve-secrets", false, "also write effective install-config.yaml/agent-config.yaml under each cluster's runtime installer directory (state-dir/runtime/<cluster>/installer/) with secret material inlined for direct openshift-install consumption (mode 0600)")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		names, err := clusterNamesForTarget(state, "all", clusterScope)
		if err != nil {
			return failErr(1, err)
		}
		state = filterStateToClusters(state, names)
		result, err := workflow.RenderOnly(cf.stateDir, secretsDir, state)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "installer render")
		printInstallerFiles(stdout, result)
		if resolveSecrets {
			resolved, err := workflow.ResolveInstaller(cf.stateDir, secretsDir, state)
			if err != nil {
				return failErr(1, err)
			}
			printEffectiveInstallerFiles(stdout, resolved)
		}
		return nil
	}
	return cmd
}

func runHubCheck(stdout io.Writer, state v1alpha1.State) error {
	names, err := clusterNamesForTarget(state, "hub", "")
	if err != nil {
		return failErr(1, err)
	}
	if len(names) != 1 {
		return failf(1, "expected exactly one hub cluster, found %d (%s)", len(names), strings.Join(names, ", "))
	}
	printOK(stdout, "hub cluster", names[0])
	return nil
}

func printInstallerFiles(stdout io.Writer, result render.Result) {
	for _, asset := range result.InstallerAssets {
		fmt.Fprintf(stdout, "- %s\n", asset.InstallConfigPath)
		fmt.Fprintf(stdout, "- %s\n", asset.AgentConfigPath)
	}
}

func printEffectiveInstallerFiles(stdout io.Writer, result render.Result) {
	printSubtitle(stdout, "effective (secrets inlined):")
	for _, asset := range result.InstallerAssets {
		fmt.Fprintf(stdout, "- %s\n", asset.EffectiveInstallConfigPath)
		fmt.Fprintf(stdout, "- %s\n", asset.EffectiveAgentConfigPath)
	}
}

func printHubComponentsPlan(stdout io.Writer, dryRun bool) {
	if dryRun {
		fmt.Fprintln(stdout, "hub components: no declarative hub component schema is implemented yet; no hub changes would run")
		return
	}
	fmt.Fprintln(stdout, "hub components: no declarative hub component schema is implemented yet; nothing to apply")
}
