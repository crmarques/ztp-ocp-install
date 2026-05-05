package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/embedded"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
	"github.com/crmarques/ztp-ocp-install-lab/internal/render"
)

// ansibleCorePinnedVersion returns the ansible-core version recorded in the
// component pins. The bootstrap command uses this to drive the pip install
// inside the managed venv so the runtime version always matches what the
// rendered lock file declares.
func ansibleCorePinnedVersion() (string, error) {
	for _, pin := range render.ComponentPins(v1alpha1.State{}) {
		if pin.Name == "ansible-core" {
			return pin.Version, nil
		}
	}
	return "", fmt.Errorf("ansible-core pin missing from render.ComponentPins")
}

// `setup` groups commands that prepare the controller running gitups. The
// intent is that a fresh machine starts with only the gitups binary plus
// user-authored desired-state YAML; `setup controller` installs the minimal
// pinned dependencies needed for the selected workflow.
func newSetupCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Prepare the controller machine running gitups",
		Long: "Subcommands provision the controller machine running gitups.\n" +
			"`setup controller` installs the minimal packages and managed ansible-core runtime\n" +
			"needed for `gitups preflight`, `gitups plan`, and `gitups apply`.",
	}
	cmd.AddCommand(
		newSetupControllerCmd(stdin, stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newSetupControllerCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun        bool
		yes           bool
		venv          bool
		cliInstallDir string
	)
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Install controller-only prerequisites",
		Long: "Installs only the dependencies that gitups itself runs on the control\n" +
			"host: a small package set, a gitups-managed ansible-core venv, and the\n" +
			"OpenShift CLIs `oc`, `kubectl`, and `openshift-install`.\n\n" +
			"Provider-side dependencies (libvirt, qemu-kvm, podman) are never\n" +
			"installed on the controller — they belong to the provider host's own\n" +
			"preparation. When -f is supplied the OCP release version is read from\n" +
			"the state and an embedded ansible playbook downloads `oc`, `kubectl`,\n" +
			"and `openshift-install` from mirror.openshift.com (no token required)\n" +
			"into the chosen install directory.\n\n" +
			"Run as root (`sudo gitups setup controller`) or have NOPASSWD sudo\n" +
			"configured for this user; gitups never reads or stores a sudo\n" +
			"password.",
		Args: cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print bootstrap commands without executing them")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the bootstrap confirmation prompt")
	cmd.Flags().BoolVar(&venv, "venv", true, "install ansible-core into a gitups-managed venv instead of via the system package manager")
	cmd.Flags().StringVar(&cliInstallDir, "cli-install-dir", "/usr/local/bin", "directory the OCP CLI installer playbook writes oc, kubectl, and openshift-install into")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		family, err := detectOSFamily(defaultOSReleasePath)
		if err != nil {
			return failErr(1, err)
		}
		var state v1alpha1.State
		if len(cf.files) > 0 {
			loaded, err := infra.LoadNormalizeValidate(cf.files)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		plan, err := controllerBootstrapPlanForMode(family, bootstrapMode{venv: venv})
		if err != nil {
			return failErr(1, err)
		}
		cliSpec := planControllerCLIInstall(state, cf.stateDir, cliInstallDir, venv)

		printTitle(stdout, "Controller setup")
		fmt.Fprintf(stdout, "OS family: %s\n", family)
		if venv {
			fmt.Fprintf(stdout, "ansible-core target: managed venv at %s\n", ansibleVenvDir())
		} else {
			fmt.Fprintln(stdout, "ansible-core target: system package manager")
		}
		printSubtitle(stdout, "planned actions:")
		for _, step := range plan {
			fmt.Fprintf(stdout, "- %s\n  $ %s\n", step.label, shellQuote(step.cmd))
		}
		switch {
		case cliSpec != nil:
			fmt.Fprintf(stdout, "- install OCP CLIs (oc, kubectl, openshift-install) %s into %s\n  $ %s\n",
				cliSpec.OCPReleaseVersion, cliSpec.InstallDir, shellQuote(cliSpec.PlannedCommand()))
		case len(cf.files) > 0:
			fmt.Fprintln(stdout, "- skipping OCP CLIs: no openshift.release.version declared in state")
		default:
			fmt.Fprintln(stdout, "- skipping OCP CLIs: pass -f <state-dir> so the release version is known")
		}
		if dryRun {
			return nil
		}
		if !yes && !confirm(stdin, stdout, "Continue with bootstrap? [y/N]: ") {
			return failErr(1, errors.New("bootstrap aborted"))
		}
		if err := ensureSudoReady(); err != nil {
			return failErr(1, err)
		}
		if err := runBootstrapPlan(c.Context(), stdin, stdout, stderr, plan); err != nil {
			return err
		}
		if cliSpec != nil {
			if err := runControllerCLIInstall(c.Context(), stdin, stdout, stderr, *cliSpec); err != nil {
				return failErr(1, err)
			}
		}
		printOK(stdout, "controller is ready", "")
		return nil
	}
	return cmd
}

type bootstrapMode struct {
	venv bool
}

func runBootstrapPlan(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, plan []bootstrapStep) error {
	for _, step := range plan {
		fmt.Fprintf(stdout, "\n>>> %s\n", step.label)
		run := exec.CommandContext(ctx, step.cmd[0], step.cmd[1:]...)
		run.Stdout = stdout
		run.Stderr = stderr
		run.Stdin = stdin
		if err := run.Run(); err != nil {
			return failErr(1, fmt.Errorf("%s: %w", step.label, err))
		}
	}
	return nil
}

type bootstrapStep struct {
	label string
	cmd   []string
}

// controllerBootstrapPlanForMode composes the bootstrap steps for the detected
// OS family and the chosen runtime mode. The package set is intentionally
// controller-only: ansible-core runtime, python3, sudo, git. Provider-side
// packages (libvirt, qemu-kvm, podman, skopeo) are never installed by this
// command — they belong to the provider host's own setup, even when the
// controller and the provider happen to share a machine.
//
// In `--venv` mode, ansible-core is removed from the system package set
// and instead pip-installed into a gitups-managed venv. python3 plus
// python3-pip stay in the system set because the venv needs them to bootstrap.
func controllerBootstrapPlanForMode(family string, mode bootstrapMode) ([]bootstrapStep, error) {
	packages := basePackages(family)
	if mode.venv {
		packages = filterOut(packages, "ansible-core")
		packages = appendUnique(packages, basePackagesForVenv(family)...)
	}
	packages = dedupe(packages)

	steps := []bootstrapStep{{
		label: "install controller packages",
		cmd:   installCommand(family, packages),
	}}
	if !mode.venv {
		return steps, nil
	}
	pin, err := ansibleCorePinnedVersion()
	if err != nil {
		return nil, err
	}
	steps = append(steps,
		bootstrapStep{
			label: "create ansible-core venv at " + ansibleVenvDir(),
			cmd:   []string{"python3", "-m", "venv", ansibleVenvDir()},
		},
		bootstrapStep{
			label: "upgrade pip in venv",
			cmd:   []string{ansibleVenvBin("pip"), "install", "--upgrade", "pip"},
		},
		bootstrapStep{
			label: "install ansible-core==" + pin + " into venv",
			cmd:   []string{ansibleVenvBin("pip"), "install", "ansible-core==" + pin},
		},
	)
	return steps, nil
}

// controllerBootstrapPlan is the convenience entry the system-package mode uses;
// callers that need the venv path use controllerBootstrapPlanForMode directly.
func controllerBootstrapPlan(family string) []bootstrapStep {
	steps, _ := controllerBootstrapPlanForMode(family, bootstrapMode{})
	return steps
}

// controllerCLIInstallSpec describes the ansible-driven step that installs
// oc, kubectl, and openshift-install on the controller from
// mirror.openshift.com. Created by planControllerCLIInstall when the supplied
// state declares an openshift release version; otherwise the step is skipped.
type controllerCLIInstallSpec struct {
	OCPReleaseVersion string
	InstallDir        string
	StateDir          string
	Executable        string
}

func planControllerCLIInstall(state v1alpha1.State, stateDir string, installDir string, venv bool) *controllerCLIInstallSpec {
	version := strings.TrimSpace(stateOpenshiftReleaseVersion(state))
	if version == "" {
		return nil
	}
	exe := "ansible-playbook"
	if venv {
		exe = ansibleVenvBin("ansible-playbook")
	}
	return &controllerCLIInstallSpec{
		OCPReleaseVersion: version,
		InstallDir:        installDir,
		StateDir:          stateDir,
		Executable:        exe,
	}
}

// stateOpenshiftReleaseVersion returns the first non-empty
// Environment.spec.openshift.release.version declared in the state. The CLI
// installer needs a concrete x.y.z to fetch tarballs from mirror.openshift.com,
// so a `channel`-only release is treated as "no version" and the step is
// skipped.
func stateOpenshiftReleaseVersion(state v1alpha1.State) string {
	for _, env := range state.Environments {
		if env.Spec.OpenShift.Release == nil {
			continue
		}
		if v := strings.TrimSpace(env.Spec.OpenShift.Release.Version); v != "" {
			return v
		}
	}
	return ""
}

// PlannedCommand returns the ansible-playbook invocation displayed in the
// dry-run plan. The bundle path is computed from the configured state-dir
// and will exist by the time runControllerCLIInstall actually executes the
// command (which extracts the embedded bundle first). Sudo escalation is
// expected to come from running the gitups process under sudo or from
// NOPASSWD on the controller — the playbook's per-task `become: true`
// stays on the inventory's local host and finds an already-root process
// (or a passwordless sudo) when it fires.
func (s controllerCLIInstallSpec) PlannedCommand() []string {
	bundleDir := filepath.Join(s.StateDir, ansibleBundleDirName)
	return []string{
		s.Executable,
		"-i", filepath.Join(bundleDir, controllerCLILocalInventory),
		filepath.Join(bundleDir, "playbooks", "setup-controller-clis.yml"),
		"-e", "gitups_openshift_release_version=" + s.OCPReleaseVersion,
		"-e", "gitups_clis_install_dir=" + s.InstallDir,
	}
}

// runControllerCLIInstall extracts the embedded ansible bundle, writes a
// localhost inventory next to it, and runs the setup-controller-clis playbook
// against the local host. The playbook is idempotent: it skips the download
// + install when the binary at the requested version already exists at the
// install directory.
func runControllerCLIInstall(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, spec controllerCLIInstallSpec) error {
	bundleDir, err := extractBundle(spec.StateDir)
	if err != nil {
		return err
	}
	inventoryPath := filepath.Join(bundleDir, controllerCLILocalInventory)
	inventory := "localhost ansible_connection=local ansible_python_interpreter=/usr/bin/python3\n"
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0o600); err != nil {
		return fmt.Errorf("write controller-clis inventory: %w", err)
	}
	bundleDirAbs := filepath.Join(spec.StateDir, ansibleBundleDirName)
	args := []string{
		spec.Executable,
		"-i", filepath.Join(bundleDirAbs, controllerCLILocalInventory),
		filepath.Join(bundleDirAbs, "playbooks", "setup-controller-clis.yml"),
		"-e", "gitups_openshift_release_version=" + spec.OCPReleaseVersion,
		"-e", "gitups_clis_install_dir=" + spec.InstallDir,
	}
	fmt.Fprintf(stdout, "\n>>> install OCP CLIs %s into %s\n", spec.OCPReleaseVersion, spec.InstallDir)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(),
		"ANSIBLE_CONFIG="+filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
		"ANSIBLE_ROLES_PATH="+filepath.Join(bundleDir, embedded.RolesRelPath),
		"ANSIBLE_COLLECTIONS_PATH="+filepath.Join(bundleDir, embedded.CollectionsRelPath),
		"ANSIBLE_FILTER_PLUGINS="+filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run controller-clis playbook: %w", err)
	}
	return nil
}

const controllerCLILocalInventory = "_setup-controller-localhost.ini"

// basePackagesForVenv adds the venv-creation prerequisite that the system
// package set otherwise omits. On Debian/Ubuntu, `python3 -m venv` requires
// the `python3-venv` apt package; Fedora/RHEL ship venv as part of python3.
func basePackagesForVenv(family string) []string {
	switch family {
	case "debian":
		return []string{"python3-venv"}
	}
	return nil
}

func filterOut(in []string, drop string) []string {
	out := in[:0]
	for _, item := range in {
		if item == drop {
			continue
		}
		out = append(out, item)
	}
	return out
}

func appendUnique(in []string, extras ...string) []string {
	seen := map[string]struct{}{}
	for _, item := range in {
		seen[item] = struct{}{}
	}
	for _, extra := range extras {
		if _, ok := seen[extra]; ok {
			continue
		}
		in = append(in, extra)
		seen[extra] = struct{}{}
	}
	return in
}

func basePackages(family string) []string {
	switch family {
	case "redhat":
		return []string{"ansible-core", "python3", "python3-pip", "sudo", "git", "tar"}
	case "debian":
		return []string{"ansible-core", "python3", "python3-pip", "sudo", "git", "tar"}
	}
	return nil
}

func installCommand(family string, packages []string) []string {
	args := []string{"sudo"}
	switch family {
	case "redhat":
		args = append(args, "dnf", "install", "-y")
	case "debian":
		args = append(args, "apt-get", "install", "-y")
	}
	return append(args, packages...)
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

const defaultOSReleasePath = "/etc/os-release"

// detectOSFamily reads /etc/os-release and maps ID / ID_LIKE onto the
// supported package-manager families. Path is parameterised so tests can
// drive the parser against fixture files without poking the real host.
func detectOSFamily(osReleasePath string) (string, error) {
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", osReleasePath, err)
	}
	id, idLike := parseOSRelease(string(data))
	haystack := strings.ToLower(id + " " + idLike)
	switch {
	case containsAny(haystack, "rhel", "fedora", "centos", "rocky", "almalinux", "alma"):
		return "redhat", nil
	case containsAny(haystack, "debian", "ubuntu"):
		return "debian", nil
	}
	return "", fmt.Errorf("unsupported OS family (id=%q id_like=%q); supported: redhat, debian", id, idLike)
}

func parseOSRelease(body string) (id string, idLike string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		case strings.HasPrefix(line, "ID_LIKE="):
			idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`)
		}
	}
	return id, idLike
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
