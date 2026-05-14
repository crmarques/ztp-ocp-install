package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/ansible"
	"github.com/crmarques/bootwright/internal/orchestrate/provisioning"
	"github.com/crmarques/bootwright/internal/workflow"
)

func newScopeCheckCmd(scope scopeSpec, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		executable   string
		secretsDir   string
		hostStateDir string
		dryRun       bool
		clusterScope string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check " + scope.name + " prerequisites",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the bootwright-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: BOOTWRIGHT_SECRETS_DIR)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible preflight command without executing it")
	if scope.name == "clusters" || scope.name == "infra" || scope.name == "all" {
		cmd.Flags().StringVar(&clusterScope, "scope", "", "comma-separated OCPCluster names to check (restricts the matching ClusterInfrastructure/Provider sets)")
	}
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		state, err = scopeState(state, scope.name, clusterScope)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, scope.name+" check")
		if err := runScopeHostCheck(stdout, stderr, state, scope.phases(), secretsDir, hostStateDir); err != nil {
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
			Limit:             ansibleLimitForScope(scope.name),
			ArtifactsBaseName: "preflight-" + scope.name,
			DryRun:            dryRun,
			Label:             scope.name + " check",
		}, runner, stdout)
		if err != nil {
			return failErr(1, err)
		}
		return nil
	}
	return cmd
}

func newScopeApplyCmd(scope scopeSpec, stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		check         bool
		askBecomePass bool
		yes           bool
		executable    string
		secretsDir    string
		hostStateDir  string
		clusterScope  string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply " + scope.name + " desired state",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&check, "check", false, "pass --check to ansible-playbook")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when bootwright runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the apply confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the bootwright-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: BOOTWRIGHT_SECRETS_DIR)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	if scope.name == "clusters" || scope.name == "infra" || scope.name == "all" {
		cmd.Flags().StringVar(&clusterScope, "scope", "", "comma-separated OCPCluster names to apply (restricts the matching ClusterInfrastructure/Provider sets)")
	}
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		state, err = scopeState(state, scope.name, clusterScope)
		if err != nil {
			return failErr(1, err)
		}
		if err := ensureApplySupported(state); err != nil {
			return failErr(1, err)
		}
		selected := scope.phases()
		printTitle(stdout, scope.name+" apply")
		if scope.applyHubComponents {
			if err := runHubCheck(stdout, state); err != nil {
				return err
			}
		}
		if !dryRun {
			if err := runApplyHostCheck(stdout, stderr, state, selected, secretsDir, hostStateDir); err != nil {
				return err
			}
		}
		printApplySummary(stdout, selected, askBecomePass, dryRun)
		if !dryRun && !yes {
			if !confirm(stdin, stdout, "Continue with apply? [y/N] (default: no): ") {
				return failErr(1, errors.New("apply aborted"))
			}
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := provisioning.AbsHostStateDir(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		pairs := resolvedOCPBinaryPairs(selected, hostStateDirAbs)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		if !dryRun {
			printWorkflowStart(stdout, scope.name, selected, askBecomePass)
		}
		runResult, err := workflow.Run(c.Context(), workflow.RunOptions{
			State:             state,
			StateDir:          cf.stateDir,
			SecretsDir:        secretsDir,
			HostStateDir:      hostStateDir,
			Executable:        executable,
			BundleDir:         bundleDir,
			Playbook:          scope.applyPlaybook,
			Limit:             ansibleLimitForScope(scope.name),
			ExtraVarPairs:     pairs,
			ArtifactsBaseName: scope.artifactsBaseName,
			Check:             check,
			AskBecomePass:     askBecomePass,
			DryRun:            dryRun,
			Label:             scope.name + " apply",
		}, runner, stdout)
		if err != nil {
			return failErr(1, err)
		}
		printRenderResult(stdout, runResult.Render)
		fmt.Fprintf(stdout, "ansible bundle: %s\n", bundleDir)
		if scope.applyHubComponents {
			printHubComponentsPlan(stdout, dryRun)
		}
		return nil
	}
	return cmd
}

func newScopeDestroyCmd(scope scopeSpec, stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		check         bool
		askBecomePass bool
		yes           bool
		executable    string
		secretsDir    string
		hostStateDir  string
		clusterScope  string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy " + scope.name + " runtime state",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&check, "check", false, "pass --check to ansible-playbook")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when bootwright runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the bootwright-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: BOOTWRIGHT_SECRETS_DIR)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	if scope.name == "clusters" || scope.name == "infra" {
		cmd.Flags().StringVar(&clusterScope, "scope", "", "comma-separated OCPCluster names to destroy (restricts the matching ClusterInfrastructure/Provider sets)")
	}
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := loadDesiredState(cf)
		if err != nil {
			return failErr(1, err)
		}
		state, err = scopeState(state, scope.name, clusterScope)
		if err != nil {
			return failErr(1, err)
		}
		selected := scope.phases()
		printTitle(stdout, scope.name+" destroy")
		printDestroySummary(stdout, selected, askBecomePass, dryRun)
		if !dryRun && !yes {
			if !confirm(stdin, stdout, "Continue with destroy? [y/N] (default: no): ") {
				return failErr(1, errors.New("destroy aborted"))
			}
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := provisioning.AbsHostStateDir(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		pairs := resolvedOCPBinaryPairs(selected, hostStateDirAbs)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		if !dryRun {
			printWorkflowStart(stdout, scope.name+" destroy", selected, askBecomePass)
		}
		runResult, err := workflow.Run(c.Context(), workflow.RunOptions{
			State:             state,
			StateDir:          cf.stateDir,
			SecretsDir:        secretsDir,
			HostStateDir:      hostStateDir,
			Executable:        executable,
			BundleDir:         bundleDir,
			Playbook:          scope.destroyPlaybook,
			Limit:             ansibleLimitForScope(scope.name),
			ExtraVarPairs:     pairs,
			ArtifactsBaseName: scope.artifactsBaseName + "-destroy",
			Check:             check,
			AskBecomePass:     askBecomePass,
			DryRun:            dryRun,
			Label:             scope.name + " destroy",
		}, runner, stdout)
		if err != nil {
			return failErr(1, err)
		}
		printRenderResult(stdout, runResult.Render)
		fmt.Fprintf(stdout, "ansible bundle: %s\n", bundleDir)
		return nil
	}
	return cmd
}

func runScopeHostCheck(stdout io.Writer, stderr io.Writer, state v1alpha1.State, selected []Phase, secretsDir, hostStateDir string) error {
	return runApplyHostCheck(stdout, stderr, state, selected, secretsDir, hostStateDir)
}

func runApplyHostCheck(stdout io.Writer, stderr io.Writer, state v1alpha1.State, selected []Phase, secretsDir, hostStateDir string) error {
	checks := collectPreflightChecks(state, selected, true, secretsDir, hostStateDir, defaultPreflightDeps)
	printSubtitle(stdout, "host check:")
	failed := 0
	for _, c := range checks {
		if c.ok {
			printOK(stdout, c.name, c.detail)
			continue
		}
		printFail(stdout, c.name, c.detail)
		failed++
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "host check: %d required check(s) failed\n", failed)
		return silentExit(1)
	}
	fmt.Fprintf(stdout, "host check: all %d check(s) passed\n", len(checks))
	return nil
}

func ansibleLimitForScope(name string) string {
	switch name {
	case "infra":
		return "bootwright_provider_hosts:bootwright_infra_hosts"
	case "clusters":
		return "bootwright_ocp_hosts"
	default:
		return ""
	}
}

func resolvedOCPBinaryPairs(selected []Phase, hostStateDir string) []string {
	clustersSelected := false
	for _, p := range selected {
		if p.Name == "clusters" {
			clustersSelected = true
			break
		}
	}
	if !clustersSelected {
		return nil
	}
	path, err := defaultLookPath("openshift-install", openshiftInstallSearchDirs(hostStateDir))
	if err != nil {
		return nil
	}
	return []string{"bootwright_openshift_install=" + path}
}
