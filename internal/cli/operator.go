package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/embedded"
	"github.com/crmarques/bootwright/internal/provisioning/render"
)

func ansibleCorePinnedVersion() (string, error) {
	for _, pin := range render.ComponentPins(v1alpha1.State{}) {
		if pin.Name == "ansible-core" {
			return pin.Version, nil
		}
	}
	return "", fmt.Errorf("ansible-core pin missing from render.ComponentPins")
}

type bootstrapStep struct {
	label string
	cmd   []string
}

func resolvePython312() (string, bool) {
	for _, bin := range []string{"python3.12", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		out, err := exec.Command(bin, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		major, minor, err := parsePythonVersion(strings.TrimSpace(string(out)))
		if err != nil {
			continue
		}
		if major > 3 || (major == 3 && minor >= 12) {
			return bin, true
		}
	}
	return "", false
}

func python312InstallCmd(preserveProxyEnv bool) []string {
	type pkgMgr struct {
		bin  string
		args []string
	}
	for _, pm := range []pkgMgr{
		{"dnf", []string{"dnf", "install", "-y", "python3.12"}},
		{"apt-get", []string{"apt-get", "install", "-y", "python3.12"}},
	} {
		if _, err := exec.LookPath(pm.bin); err != nil {
			continue
		}
		if os.Getuid() == 0 {
			return pm.args
		}
		if _, err := exec.LookPath("sudo"); err == nil {
			return sudoPackageInstallCmd(pm.args, preserveProxyEnv)
		}
		return pm.args
	}
	return nil
}

func sudoPackageInstallCmd(args []string, preserveProxyEnv bool) []string {
	out := []string{"sudo"}
	if preserveProxyEnv {
		out = append(out, "--preserve-env="+sudoPreservedProxyVars)
	}
	return append(out, args...)
}

const sudoPreservedProxyVars = "HTTP_PROXY,HTTPS_PROXY,NO_PROXY,http_proxy,https_proxy,no_proxy"

func controllerBootstrapPlan(preserveProxyEnv bool) ([]bootstrapStep, error) {
	pin, err := ansibleCorePinnedVersion()
	if err != nil {
		return nil, err
	}
	python, found := resolvePython312()
	var steps []bootstrapStep
	if !found {
		installCmd := python312InstallCmd(preserveProxyEnv)
		if installCmd == nil {
			return nil, fmt.Errorf("python3.12 not found; install it manually or ensure dnf or apt-get is available")
		}
		label := "install python3.12"
		if installCmd[0] == "sudo" {
			label += " (requires sudo)"
		}
		steps = append(steps, bootstrapStep{
			label: label,
			cmd:   installCmd,
		})
		python = "python3.12"
	}
	return append(steps,
		bootstrapStep{
			label: "create ansible-core venv at " + ansibleVenvDir(),
			cmd:   []string{python, "-m", "venv", ansibleVenvDir()},
		},
		bootstrapStep{
			label: "upgrade pip in venv",
			cmd:   []string{ansibleVenvBin("pip"), "install", "--upgrade", "pip"},
		},
		bootstrapStep{
			label: "install ansible-core==" + pin + " into venv",
			cmd:   []string{ansibleVenvBin("pip"), "install", "ansible-core==" + pin},
		},
	), nil
}

func runBootstrapPlan(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, plan []bootstrapStep, extraEnv map[string]string) error {
	for _, step := range plan {
		fmt.Fprintf(stdout, "\n>>> %s\n", step.label)
		run := exec.CommandContext(ctx, step.cmd[0], step.cmd[1:]...)
		run.Stdout = stdout
		run.Stderr = stderr
		run.Stdin = stdin
		run.Env = mergeBootstrapEnv(os.Environ(), extraEnv)
		if err := run.Run(); err != nil {
			return failErr(1, fmt.Errorf("%s: %w", step.label, err))
		}
	}
	return nil
}

type controllerCLIInstallSpec struct {
	OCPReleaseVersion string
	InstallDir        string
	StateDir          string
	Executable        string
}

func planControllerCLIInstall(state v1alpha1.State, stateDir string, installDir string) *controllerCLIInstallSpec {
	version := strings.TrimSpace(stateOpenshiftReleaseVersion(state))
	if version == "" {
		return nil
	}
	return &controllerCLIInstallSpec{
		OCPReleaseVersion: version,
		InstallDir:        installDir,
		StateDir:          stateDir,
		Executable:        ansibleVenvBin("ansible-playbook"),
	}
}

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

func (s controllerCLIInstallSpec) PlannedCommand() []string {
	bundleDir := filepath.Join(s.StateDir, ansibleBundleDirName)
	return []string{
		s.Executable,
		"-i", filepath.Join(bundleDir, controllerCLILocalInventory),
		filepath.Join(bundleDir, "playbooks", "targets", "bastion", "apply-clis.yml"),
		"-e", "bootwright_openshift_release_version=" + s.OCPReleaseVersion,
		"-e", "bootwright_clis_install_dir=" + s.InstallDir,
	}
}

func runControllerCLIInstall(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, spec controllerCLIInstallSpec, extraEnv map[string]string) error {
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
		filepath.Join(bundleDirAbs, "playbooks", "targets", "bastion", "apply-clis.yml"),
		"-e", "bootwright_openshift_release_version=" + spec.OCPReleaseVersion,
		"-e", "bootwright_clis_install_dir=" + spec.InstallDir,
	}
	fmt.Fprintf(stdout, "\n>>> install OCP CLIs %s into %s\n", spec.OCPReleaseVersion, spec.InstallDir)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	ansibleEnv := map[string]string{
		"ANSIBLE_CONFIG":           filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
		"ANSIBLE_ROLES_PATH":       embedded.RolesPath(bundleDir),
		"ANSIBLE_COLLECTIONS_PATH": filepath.Join(bundleDir, embedded.CollectionsRelPath),
		"ANSIBLE_FILTER_PLUGINS":   filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
	}
	for k, v := range extraEnv {
		ansibleEnv[k] = v
	}
	cmd.Env = mergeBootstrapEnv(os.Environ(), ansibleEnv)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run controller-clis playbook: %w", err)
	}
	return nil
}

const controllerCLILocalInventory = "_setup-controller-localhost.ini"
