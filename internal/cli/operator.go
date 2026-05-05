package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
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
	return cmd
}

func newSetupControllerCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun  bool
		yes     bool
		venv    bool
		minimal bool
	)
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Install controller prerequisites with a minimal package footprint",
		Long: "Installs the system packages required to run `gitups apply` on this host.\n" +
			"With -f, the package set is widened to include dependencies the supplied\n" +
			"state declares. Use --minimal to ignore state-driven extras. By default,\n" +
			"ansible-core is installed into a gitups-managed venv under <gitups-home>\n" +
			"instead of relying on a preinstalled system CLI.",
		Args: cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print bootstrap commands without executing them")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the bootstrap confirmation prompt")
	cmd.Flags().BoolVar(&venv, "venv", true, "install ansible-core into a gitups-managed venv instead of via the system package manager")
	cmd.Flags().BoolVar(&minimal, "minimal", false, "install only baseline controller packages even when -f is supplied")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		family, err := detectOSFamily(defaultOSReleasePath)
		if err != nil {
			return failErr(1, err)
		}
		var state v1alpha1.State
		if len(cf.files) > 0 && !minimal {
			loaded, err := infra.LoadNormalizeValidate(cf.files)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		plan, err := controllerBootstrapPlanForMode(family, state, bootstrapMode{venv: venv})
		if err != nil {
			return failErr(1, err)
		}
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
		if dryRun {
			return nil
		}
		if !yes && !confirm(stdin, stdout, "Continue with bootstrap? [y/N]: ") {
			return failErr(1, errors.New("bootstrap aborted"))
		}
		return runBootstrapPlan(c.Context(), stdin, stdout, stderr, plan)
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
	printOK(stdout, "controller is ready", "")
	return nil
}

type bootstrapStep struct {
	label string
	cmd   []string
}

// controllerBootstrapPlanForMode composes the bootstrap steps for the detected
// OS family and the chosen runtime mode. State-driven extras only appear
// when the supplied state declares the matching capability — running
// bootstrap with no -f keeps the install surface minimal so a stateless
// controller can still run `gitups validate` and `gitups plan` without
// paying for libvirt/podman.
//
// In `--venv` mode, ansible-core is removed from the system package set
// and instead pip-installed into a gitups-managed venv. python3 plus
// python3-pip stay in the system set because the venv needs them to bootstrap.
func controllerBootstrapPlanForMode(family string, state v1alpha1.State, mode bootstrapMode) ([]bootstrapStep, error) {
	packages := basePackages(family)
	if mode.venv {
		packages = filterOut(packages, "ansible-core")
		packages = appendUnique(packages, basePackagesForVenv(family)...)
	}
	if stateNeedsQemuKvm(state) {
		packages = append(packages, libvirtPackages(family)...)
	}
	if stateNeedsMirrorRegistry(state) {
		packages = append(packages, mirrorRegistryPackages(family)...)
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
// callers that need the venv path use operatorBootstrapPlanForMode directly.
func controllerBootstrapPlan(family string, state v1alpha1.State) []bootstrapStep {
	steps, _ := controllerBootstrapPlanForMode(family, state, bootstrapMode{})
	return steps
}

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
		return []string{"ansible-core", "python3", "python3-pip", "sudo", "git"}
	case "debian":
		return []string{"ansible-core", "python3", "python3-pip", "sudo", "git"}
	}
	return nil
}

func libvirtPackages(family string) []string {
	switch family {
	case "redhat":
		return []string{"qemu-kvm", "libvirt", "virt-install", "python3-libvirt"}
	case "debian":
		return []string{"qemu-kvm", "libvirt-clients", "libvirt-daemon-system", "virtinst", "python3-libvirt"}
	}
	return nil
}

func mirrorRegistryPackages(family string) []string {
	switch family {
	case "redhat":
		return []string{"podman", "skopeo", "httpd-tools", "ca-certificates", "openssl"}
	case "debian":
		return []string{"podman", "skopeo", "apache2-utils", "ca-certificates", "openssl"}
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

// stateNeedsMirrorRegistry returns true when at least one Environment opts
// in to a mirror registry, regardless of cluster role. Mirror tooling is
// installed in the setup because the registry runs on the controller host
// (or a co-located provider host) and pulls release images before any apply.
func stateNeedsMirrorRegistry(state v1alpha1.State) bool {
	for _, env := range state.Environments {
		if registries := v1alpha1.OCPInstallRegistriesOf(env); registries != nil && registries.Mirror != nil {
			return true
		}
	}
	return false
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
