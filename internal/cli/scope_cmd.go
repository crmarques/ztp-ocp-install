package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/ansible"
	"github.com/crmarques/gitups/internal/embedded"
	"github.com/crmarques/gitups/internal/render"
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
		Short: "Run preflight checks for the " + scope.name + " scope",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
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
		result, err := render.All(cf.stateDir, secretsDir, state)
		if err != nil {
			return failErr(1, err)
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		stateDirAbs, err := filepath.Abs(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		secretsDirAbs, err := filepath.Abs(secretsDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := filepath.Abs(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, "playbooks/preflight.yml"),
			Limit:             ansibleLimitForScope(scope.name),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs: []string{
				"gitups_state_dir=" + stateDirAbs,
				"gitups_secrets_dir=" + secretsDirAbs,
				"gitups_host_state_dir=" + hostStateDirAbs,
			},
			ArtifactsDir: filepath.Join(result.ArtifactsDir, "preflight-"+scope.name),
		}
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [%s check]: %s\n", scope.name, shellQuote(command))
			return nil
		}
		return runner.Run(c.Context(), spec)
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
		Short: "Converge the " + scope.name + " scope",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&check, "check", false, "pass --check to ansible-playbook")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when gitups runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the apply confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
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
		result, err := render.All(cf.stateDir, secretsDir, state)
		if err != nil {
			return failErr(1, err)
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		printRenderResult(stdout, result)
		fmt.Fprintf(stdout, "ansible bundle: %s\n", bundleDir)
		stateDirAbs, err := filepath.Abs(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		secretsDirAbs, err := filepath.Abs(secretsDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := filepath.Abs(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		pairs := []string{
			"gitups_state_dir=" + stateDirAbs,
			"gitups_secrets_dir=" + secretsDirAbs,
			"gitups_host_state_dir=" + hostStateDirAbs,
		}
		pairs = append(pairs, resolvedOCPBinaryPairs(selected, hostStateDirAbs)...)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, scope.applyPlaybook),
			Limit:             ansibleLimitForScope(scope.name),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs:     pairs,
			ArtifactsDir:      filepath.Join(result.ArtifactsDir, scope.artifactsBaseName),
			Check:             check,
			AskBecomePass:     askBecomePass,
		}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [%s apply]: %s\n", scope.name, shellQuote(command))
			if scope.applyHubComponents {
				printHubComponentsPlan(stdout, true)
			}
			return nil
		}
		printWorkflowStart(stdout, scope.name, selected, askBecomePass)
		if err := runner.Run(c.Context(), spec); err != nil {
			return failErr(1, err)
		}
		if scope.applyHubComponents {
			printHubComponentsPlan(stdout, false)
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
		Short: "Tear down the " + scope.name + " scope",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&check, "check", false, "pass --check to ansible-playbook")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when gitups runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
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
		result, err := render.All(cf.stateDir, secretsDir, state)
		if err != nil {
			return failErr(1, err)
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		printRenderResult(stdout, result)
		fmt.Fprintf(stdout, "ansible bundle: %s\n", bundleDir)
		stateDirAbs, err := filepath.Abs(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		secretsDirAbs, err := filepath.Abs(secretsDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := filepath.Abs(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		pairs := []string{
			"gitups_state_dir=" + stateDirAbs,
			"gitups_secrets_dir=" + secretsDirAbs,
			"gitups_host_state_dir=" + hostStateDirAbs,
		}
		pairs = append(pairs, resolvedOCPBinaryPairs(selected, hostStateDirAbs)...)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, scope.destroyPlaybook),
			Limit:             ansibleLimitForScope(scope.name),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs:     pairs,
			ArtifactsDir:      filepath.Join(result.ArtifactsDir, scope.artifactsBaseName+"-destroy"),
			Check:             check,
			AskBecomePass:     askBecomePass,
		}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [%s destroy]: %s\n", scope.name, shellQuote(command))
			return nil
		}
		printWorkflowStart(stdout, scope.name+" destroy", selected, askBecomePass)
		if err := runner.Run(c.Context(), spec); err != nil {
			return failErr(1, err)
		}
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
		return "gitups_provider_hosts:gitups_infra_hosts"
	case "clusters":
		return "gitups_ocp_hosts"
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
	return []string{"gitups_openshift_install=" + path}
}

func printApplySummary(w io.Writer, selected []Phase, askBecomePass bool, dryRun bool) {
	printWorkflowSummary(w, "apply plan:", selected, askBecomePass, dryRun)
}

func printDestroySummary(w io.Writer, selected []Phase, askBecomePass bool, dryRun bool) {
	printWorkflowSummary(w, "destroy plan:", selected, askBecomePass, dryRun)
}

func printWorkflowSummary(w io.Writer, title string, selected []Phase, askBecomePass bool, dryRun bool) {
	fmt.Fprintln(w, title)
	rootPhases := 0
	for _, p := range selected {
		marker := ""
		if p.NeedsRoot {
			marker = " [root]"
			rootPhases++
		}
		fmt.Fprintf(w, "  - %s%s — %s\n", p.Name, marker, p.Description)
	}
	if rootPhases > 0 {
		switch {
		case dryRun:
			fmt.Fprintln(w, "[root] phases require sudo escalation; this is a dry run, no commands execute.")
		case askBecomePass && rootPhases > 1:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; ansible will prompt once for the BECOME (sudo) password and reuse it for this workflow.")
		case askBecomePass:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; ansible will prompt for the BECOME (sudo) password.")
		case os.Geteuid() == 0:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; gitups is running as root, no BECOME password prompt needed.")
		default:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; --ask-become-pass=false requires passwordless sudo or an already-root connection user.")
		}
	}
}

func printWorkflowStart(w io.Writer, workflowName string, selected []Phase, askBecomePass bool) {
	if len(selected) == 1 {
		printPhaseStart(w, selected[0], askBecomePass)
		return
	}
	if rootPhaseCount(selected) > 0 {
		fmt.Fprintf(w, "\n>>> running workflow %q [root] — phases: %s\n", workflowName, phaseList(selected))
		if askBecomePass {
			fmt.Fprintln(w, ">>> ansible may prompt once for the sudo (BECOME) password for this workflow.")
		}
		return
	}
	fmt.Fprintf(w, "\n>>> running workflow %q — phases: %s\n", workflowName, phaseList(selected))
}

func printPhaseStart(w io.Writer, phase Phase, askBecomePass bool) {
	if phase.NeedsRoot && askBecomePass {
		fmt.Fprintf(w, "\n>>> running phase %q [root] — %s\n>>> ansible may prompt for the sudo (BECOME) password for this phase.\n", phase.Name, phase.Description)
		return
	}
	fmt.Fprintf(w, "\n>>> running phase %q — %s\n", phase.Name, phase.Description)
}

func rootPhaseCount(selected []Phase) int {
	count := 0
	for _, p := range selected {
		if p.NeedsRoot {
			count++
		}
	}
	return count
}

func phaseList(selected []Phase) string {
	names := make([]string, 0, len(selected))
	for _, p := range selected {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

var askBecomePassDefault = func() bool { return os.Geteuid() != 0 }

func confirm(in io.Reader, prompt io.Writer, message string) bool {
	if in == nil {
		return false
	}
	fmt.Fprint(prompt, message)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
