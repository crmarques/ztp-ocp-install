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

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/ansible"
	"github.com/crmarques/ztp-ocp-install-lab/internal/embedded"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
	"github.com/crmarques/ztp-ocp-install-lab/internal/render"
)

func newApplyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Converge desired state by workflow scope",
		Long: "Converges one explicit workflow scope.\n" +
			"Use `apply infra` for provider and substrate preparation and\n" +
			"`apply ocp` to run openshift-install agent against the cluster nodes.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newApplyScopeCmd("infra", stdin, stdout, stderr),
		newApplyScopeCmd("ocp", stdin, stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newApplyScopeCmd(scope string, stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		check         bool
		askBecomePass bool
		yes           bool
		executable    string
		secretsDir    string
		hostStateDir  string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   scope,
		Short: applyScopeShort(scope),
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&check, "check", false, "pass --check to ansible-playbook")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when gitups runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the apply confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		selected, err := phasesForApplyScope(scope)
		if err != nil {
			return failErr(1, err)
		}
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		if err := ensureApplySupported(state); err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Apply")
		if !dryRun {
			if err := runApplyHostCheck(stdout, stderr, state, selected, secretsDir, hostStateDir); err != nil {
				return err
			}
		}
		printApplySummary(stdout, selected, askBecomePass, dryRun)
		if !dryRun && !yes {
			if !confirm(stdin, stdout, "Continue with apply? [y/N]: ") {
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
		workflow, err := applyWorkflow(scope)
		if err != nil {
			return failErr(1, err)
		}
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		ctx := c.Context()
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, workflow.Playbook),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs:     pairs,
			ArtifactsDir:      filepath.Join(result.ArtifactsDir, workflow.ArtifactsDir),
			Check:             check,
			AskBecomePass:     askBecomePass,
		}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [workflow=%s]: %s\n", workflow.Name, shellQuote(command))
			return nil
		}
		printWorkflowStart(stdout, workflow.Name, selected, askBecomePass)
		if err := runner.Run(ctx, spec); err != nil {
			return failErr(1, err)
		}
		return nil
	}
	return cmd
}

func newDestroyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Reverse apply by workflow scope",
		Long: "Destroys one explicit workflow scope. `destroy all` runs the ocp,\n" +
			"cluster substrate, and provider teardowns in reverse order.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newDestroyScopeCmd("infra", stdin, stdout, stderr),
		newDestroyScopeCmd("ocp", stdin, stdout, stderr),
		newDestroyScopeCmd("all", stdin, stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newDestroyScopeCmd(scope string, _ io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		askBecomePass bool
		yes           bool
		executable    string
		secretsDir    string
		hostStateDir  string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   scope,
		Short: destroyScopeShort(scope),
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the Ansible commands without executing them")
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", askBecomePassDefault(), "prompt for the Ansible become password; defaults to false when gitups runs as root, true otherwise")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	var keepStateDir bool
	cmd.Flags().BoolVar(&keepStateDir, "keep-state-dir", false, "keep --state-dir after `destroy all` succeeds")
	var keepMirroredImages bool
	cmd.Flags().BoolVar(&keepMirroredImages, "keep-mirrored-images", false, "keep the mirror-registry data volume on the provider host so the next apply does not have to re-pull from public registries")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		selected, err := phasesForDestroyScope(scope)
		if err != nil {
			return failErr(1, err)
		}
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		if err := ensureApplySupported(state); err != nil {
			return failErr(1, err)
		}
		if !dryRun && !yes {
			return failErr(1, errors.New("destroy refused: pass --yes to confirm teardown, or --dry-run to preview"))
		}
		printTitle(stdout, "Destroy")
		printDestroySummary(stdout, selected, askBecomePass, dryRun)
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
		if keepMirroredImages {
			pairs = append(pairs, "gitups_keep_mirrored_images=true")
		}
		workflow, err := destroyWorkflow(scope, selected)
		if err != nil {
			return failErr(1, err)
		}
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		ctx := c.Context()
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, workflow.Playbook),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs:     pairs,
			ArtifactsDir:      filepath.Join(result.ArtifactsDir, workflow.ArtifactsDir),
			AskBecomePass:     askBecomePass,
		}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [workflow=%s destroy]: %s\n", workflow.Name, shellQuote(command))
		} else {
			printWorkflowStart(stdout, workflow.Name, selected, askBecomePass)
			if err := runner.Run(ctx, spec); err != nil {
				return failErr(1, err)
			}
		}
		if dryRun {
			if scope == "all" && !keepStateDir {
				fmt.Fprintf(stdout, "dry-run: would remove state-dir: %s\n", stateDirAbs)
			}
			return nil
		}
		if scope != "all" || keepStateDir {
			return nil
		}
		if err := os.RemoveAll(stateDirAbs); err != nil {
			return failErr(1, fmt.Errorf("remove state-dir %s: %w", stateDirAbs, err))
		}
		fmt.Fprintf(stdout, "removed state-dir: %s\n", stateDirAbs)
		return nil
	}
	return cmd
}

func applyScopeShort(scope string) string {
	switch scope {
	case "infra":
		return "Prepare provider services and per-cluster infrastructure substrate"
	case "ocp":
		return "Run openshift-install agent against the cluster nodes"
	default:
		return "Converge desired state"
	}
}

func destroyScopeShort(scope string) string {
	switch scope {
	case "infra":
		return "Destroy provider services and per-cluster infrastructure substrate"
	case "ocp":
		return "Destroy the openshift-install state for each OCPCluster"
	case "all":
		return "Destroy OCP install state, infrastructure substrate, and provider services"
	default:
		return "Destroy desired state"
	}
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

func resolvedOCPBinaryPairs(selected []Phase, hostStateDir string) []string {
	ocpSelected := false
	for _, p := range selected {
		if p.Name == "ocp" {
			ocpSelected = true
			break
		}
	}
	if !ocpSelected {
		return nil
	}
	path, err := defaultLookPath("openshift-install", openshiftInstallSearchDirs(hostStateDir))
	if err != nil {
		return nil
	}
	return []string{"gitups_openshift_install=" + path}
}

type phaseWorkflow struct {
	Name         string
	Playbook     string
	ArtifactsDir string
}

func applyWorkflow(scope string) (phaseWorkflow, error) {
	switch scope {
	case "infra":
		return phaseWorkflow{Name: "infra", Playbook: "playbooks/apply-infra.yml", ArtifactsDir: "infra"}, nil
	case "ocp":
		return phaseWorkflow{Name: "ocp", Playbook: "playbooks/apply-ocp.yml", ArtifactsDir: "ocp"}, nil
	default:
		return phaseWorkflow{}, fmt.Errorf("apply scope %q has no workflow playbook", scope)
	}
}

func destroyWorkflow(scope string, selected []Phase) (phaseWorkflow, error) {
	if len(selected) == 1 {
		p := selected[0]
		return phaseWorkflow{Name: p.Name, Playbook: p.DestroyPlaybook, ArtifactsDir: p.Name + "-destroy"}, nil
	}
	switch scope {
	case "infra":
		return phaseWorkflow{Name: "infra", Playbook: "playbooks/destroy-infra.yml", ArtifactsDir: "infra-destroy"}, nil
	case "all":
		return phaseWorkflow{Name: "all", Playbook: "playbooks/destroy-all.yml", ArtifactsDir: "all-destroy"}, nil
	default:
		return phaseWorkflow{}, fmt.Errorf("destroy scope %q has no workflow playbook", scope)
	}
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
