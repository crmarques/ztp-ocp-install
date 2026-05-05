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
			"Use `apply infra` for provider and substrate preparation, `apply hub`\n" +
			"for hub installation, and `apply clusters` once managed-cluster\n" +
			"GitOps publication is implemented.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newApplyScopeCmd("infra", stdin, stdout, stderr),
		newApplyScopeCmd("hub", stdin, stdout, stderr),
		newApplyScopeCmd("clusters", stdin, stdout, stderr),
	)
	return cmd
}

func newApplyScopeCmd(scope string, stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		check         bool
		askBecomePass bool
		yes           bool
		executable    string
		extraVars     []string
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
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", true, "prompt once per phase for the sudo (BECOME) password (default true; pass --ask-become-pass=false on hosts with passwordless sudo or when wrapping gitups in sudo)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the apply confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.Flags().StringArrayVar(&extraVars, "extra-var", nil, "extra ansible variable in key=value form; may be repeated (passed via -e)")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		for _, v := range extraVars {
			if !strings.Contains(v, "=") {
				return failf(2, "--extra-var %q must be key=value", v)
			}
		}
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
		result, err := render.All(cf.stateDir, state)
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
		pairs = append(pairs, resolvedHubBinaryPairs(selected, hostStateDirAbs)...)
		pairs = append(pairs, extraVars...)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		ctx := c.Context()
		for _, phase := range selected {
			spec := ansible.RunSpec{
				Executable:        executable,
				AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
				RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
				CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
				FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
				Inventory:         result.InventoryPath,
				Playbook:          filepath.Join(bundleDir, phase.ApplyPlaybook),
				ExtraVars:         result.VarsPath,
				ExtraVarPairs:     pairs,
				ArtifactsDir:      filepath.Join(result.ArtifactsDir, phase.Name),
				Check:             check,
				AskBecomePass:     askBecomePass,
			}
			command := runner.Command(spec)
			if dryRun {
				fmt.Fprintf(stdout, "dry-run ansible command [phase=%s]: %s\n", phase.Name, shellQuote(command))
				continue
			}
			printPhaseStart(stdout, phase, askBecomePass)
			if err := runner.Run(ctx, spec); err != nil {
				return failErr(1, err)
			}
		}
		return nil
	}
	return cmd
}

func newDestroyCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Reverse apply by workflow scope",
		Long: "Destroys one explicit workflow scope. `destroy all` runs hub,\n" +
			"cluster substrate, and provider teardown in reverse order.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newDestroyScopeCmd("infra", stdout, stderr),
		newDestroyScopeCmd("hub", stdout, stderr),
		newDestroyScopeCmd("clusters", stdout, stderr),
		newDestroyScopeCmd("all", stdout, stderr),
	)
	return cmd
}

func newDestroyScopeCmd(scope string, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		askBecomePass bool
		yes           bool
		executable    string
		extraVars     []string
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
	cmd.Flags().BoolVar(&askBecomePass, "ask-become-pass", false, "ask for the Ansible become password")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.Flags().StringArrayVar(&extraVars, "extra-var", nil, "extra ansible variable in key=value form; may be repeated (passed via -e)")
	var keepStateDir bool
	cmd.Flags().BoolVar(&keepStateDir, "keep-state-dir", false, "keep --state-dir after `destroy all` succeeds")
	var keepMirroredImages bool
	cmd.Flags().BoolVar(&keepMirroredImages, "keep-mirrored-images", false, "keep the mirror-registry data volume on the provider host so the next apply does not have to re-pull from public registries")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		for _, v := range extraVars {
			if !strings.Contains(v, "=") {
				return failf(2, "--extra-var %q must be key=value", v)
			}
		}
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
		result, err := render.All(cf.stateDir, state)
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
		pairs = append(pairs, extraVars...)
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		ctx := c.Context()
		for _, phase := range selected {
			spec := ansible.RunSpec{
				Executable:        executable,
				AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
				RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
				CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
				FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
				Inventory:         result.InventoryPath,
				Playbook:          filepath.Join(bundleDir, phase.DestroyPlaybook),
				ExtraVars:         result.VarsPath,
				ExtraVarPairs:     pairs,
				ArtifactsDir:      filepath.Join(result.ArtifactsDir, phase.Name+"-destroy"),
				AskBecomePass:     askBecomePass,
			}
			command := runner.Command(spec)
			if dryRun {
				fmt.Fprintf(stdout, "dry-run ansible command [phase=%s destroy]: %s\n", phase.Name, shellQuote(command))
				continue
			}
			if err := runner.Run(ctx, spec); err != nil {
				return failErr(1, err)
			}
		}
		// `destroy all` owns the entire state-dir teardown: rendered artifacts,
		// extracted ansible bundle, and per-phase ansible logs all live under
		// stateDirAbs and refer to infrastructure that no longer exists. Scoped
		// destroys leave state in place because the remaining scopes still need it.
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
	case "hub":
		return "Install the hub OpenShift cluster"
	case "clusters":
		return "Publish managed-cluster desired state through the hub"
	default:
		return "Converge desired state"
	}
}

func destroyScopeShort(scope string) string {
	switch scope {
	case "infra":
		return "Destroy provider services and per-cluster infrastructure substrate"
	case "hub":
		return "Destroy the hub OpenShift cluster"
	case "clusters":
		return "Unpublish managed-cluster desired state through the hub"
	case "all":
		return "Destroy hub, infrastructure substrate, and provider services"
	default:
		return "Destroy desired state"
	}
}

// runApplyHostCheck enforces the spec rule that apply must run a
// `validate --check-host` pass before mutating anything. Failures abort
// before render, before phase prompt, and before any ansible execution.
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

// ----- helpers reused by apply + destroy --------------------------------------

// resolvedHubBinaryPairs returns extra-var pairs for binaries the hub phase
// needs as absolute paths, so the ansible role can locate them under sudo's
// reduced PATH. Silent on miss: preflight is the place that surfaces missing
// binaries; if the user has overridden via --extra-var, that wins because it
// is appended after.
func resolvedHubBinaryPairs(selected []Phase, hostStateDir string) []string {
	hubSelected := false
	for _, p := range selected {
		if p.Name == "hub" {
			hubSelected = true
			break
		}
	}
	if !hubSelected {
		return nil
	}
	path, err := defaultLookPath("openshift-install", openshiftInstallSearchDirs(hostStateDir))
	if err != nil {
		return nil
	}
	return []string{"gitups_openshift_install=" + path}
}

// printApplySummary lists the phases apply will run, marks which need root
// escalation, and tells the user how that escalation will be obtained so the
// confirmation prompt is informed.
func printApplySummary(w io.Writer, selected []Phase, askBecomePass bool, dryRun bool) {
	fmt.Fprintln(w, "apply plan:")
	needsRoot := false
	for _, p := range selected {
		marker := ""
		if p.NeedsRoot {
			marker = " [root]"
			needsRoot = true
		}
		fmt.Fprintf(w, "  - %s%s — %s\n", p.Name, marker, p.Description)
	}
	if needsRoot {
		switch {
		case dryRun:
			fmt.Fprintln(w, "[root] phases require sudo escalation; this is a dry run, no commands execute.")
		case askBecomePass:
			fmt.Fprintln(w, "[root] phases require sudo escalation; ansible will prompt for the BECOME (sudo) password once per phase.")
		default:
			fmt.Fprintln(w, "[root] phases require sudo escalation; --ask-become-pass is disabled — host must allow passwordless sudo or gitups must be wrapped in sudo.")
		}
	}
}

// printPhaseStart announces the phase that is about to run so the user knows
// which step the upcoming sudo (BECOME) password prompt is authorising.
func printPhaseStart(w io.Writer, phase Phase, askBecomePass bool) {
	if phase.NeedsRoot && askBecomePass {
		fmt.Fprintf(w, "\n>>> running phase %q [root] — %s\n>>> ansible will now prompt for the sudo (BECOME) password for this phase.\n", phase.Name, phase.Description)
		return
	}
	fmt.Fprintf(w, "\n>>> running phase %q — %s\n", phase.Name, phase.Description)
}

// confirm reads a single line from stdin and returns true for "y"/"yes".
// Returns false when stdin is nil or unreadable so non-interactive callers
// must opt in via --yes.
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
