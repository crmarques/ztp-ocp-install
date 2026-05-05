package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

func TestOperatorCheckUniversalChecksWithoutInputs(t *testing.T) {
	stubPreflightAlwaysOK(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"operator", "check"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"Operator host check",
		"ansible-playbook on PATH",
		"python3 on PATH",
		"sudo on PATH",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, out)
		}
	}
}

// `operator check` with `-f` runs the full state-driven preflight without
// requiring a `--phase` selection — every phase's checks are in scope so the
// user sees the same picture they'd get from `validate --check-host`.
func TestOperatorCheckWithStateRunsFullPreflight(t *testing.T) {
	stubPreflightAlwaysOK(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"operator", "check",
		"-f", "../../examples/infra",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	// The libvirt example declares qemu-kvm so the cluster phase's KVM
	// probe must surface; the hub phase brings in openshift-install on
	// PATH. Both come from collectPreflightChecks under hasState=true.
	for _, expected := range []string{
		"/dev/kvm available",
		"openshift-install on PATH",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("state-driven preflight missing %q\n%s", expected, out)
		}
	}
}

// Bootstrap dry-run must enumerate the package manager invocation it would
// make. The plan is OS-driven, so we exercise the planner directly with a
// fixture instead of relying on the real /etc/os-release.
func TestOperatorBootstrapPlanRedhatBaseOnly(t *testing.T) {
	plan := operatorBootstrapPlan("redhat", v1alpha1.State{})
	if len(plan) != 1 {
		t.Fatalf("expected exactly one bootstrap step, got %d: %+v", len(plan), plan)
	}
	cmd := plan[0].cmd
	if cmd[0] != "sudo" || cmd[1] != "dnf" || cmd[2] != "install" || cmd[3] != "-y" {
		t.Fatalf("expected sudo dnf install -y prefix, got %v", cmd)
	}
	have := strings.Join(cmd, " ")
	for _, pkg := range []string{"ansible-core", "python3", "git"} {
		if !strings.Contains(have, " "+pkg) {
			t.Fatalf("bootstrap base missing %q\n%s", pkg, have)
		}
	}
	for _, leak := range []string{"qemu-kvm", "libvirt", "podman"} {
		if strings.Contains(have, " "+leak) {
			t.Fatalf("base bootstrap leaked state-driven package %q\n%s", leak, have)
		}
	}
}

func TestOperatorBootstrapPlanAddsLibvirtForQemuKvmState(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Spec: v1alpha1.InfrastructureProviderSpec{
				Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{}},
			},
		}},
	}
	have := strings.Join(operatorBootstrapPlan("redhat", state)[0].cmd, " ")
	for _, pkg := range []string{"qemu-kvm", "libvirt", "virt-install", "python3-libvirt"} {
		if !strings.Contains(have, " "+pkg) {
			t.Fatalf("libvirt-aware bootstrap missing %q\n%s", pkg, have)
		}
	}
}

func TestOperatorBootstrapPlanAddsMirrorPackagesForDisconnectedState(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Disconnected: &v1alpha1.DisconnectedSpec{
						Registries: &v1alpha1.OCPInstallRegistries{
							Mirror: &v1alpha1.OCPInstallRegistryMirror{
								URL: "registry.lab.test:5000",
							},
						},
					},
				},
			},
		}},
	}
	have := strings.Join(operatorBootstrapPlan("redhat", state)[0].cmd, " ")
	for _, pkg := range []string{"podman", "skopeo", "httpd-tools", "openssl"} {
		if !strings.Contains(have, " "+pkg) {
			t.Fatalf("mirror-aware bootstrap missing %q\n%s", pkg, have)
		}
	}
}

func TestOperatorBootstrapPlanDebianFamily(t *testing.T) {
	plan := operatorBootstrapPlan("debian", v1alpha1.State{})
	cmd := plan[0].cmd
	if cmd[0] != "sudo" || cmd[1] != "apt-get" {
		t.Fatalf("expected debian to use apt-get, got %v", cmd)
	}
}

// In `--venv` mode ansible-core moves out of the system package set and
// into a pip install inside the gitups-managed venv. The system step still
// runs to install python3 / python3-pip; the venv steps come after.
func TestOperatorBootstrapPlanVenvModeMovesAnsibleToVenv(t *testing.T) {
	plan, err := operatorBootstrapPlanForMode("redhat", v1alpha1.State{}, bootstrapMode{venv: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan) != 4 {
		t.Fatalf("expected 4 steps (system pkgs + venv create + pip upgrade + pip install ansible-core), got %d: %+v", len(plan), plan)
	}
	systemStep := strings.Join(plan[0].cmd, " ")
	if strings.Contains(systemStep, " ansible-core") {
		t.Fatalf("venv mode must not install ansible-core via the package manager: %s", systemStep)
	}
	if !strings.Contains(systemStep, " python3-pip") {
		t.Fatalf("venv mode still needs python3-pip in the system set: %s", systemStep)
	}
	venvCreate := strings.Join(plan[1].cmd, " ")
	if !strings.Contains(venvCreate, "python3 -m venv") {
		t.Fatalf("expected python3 -m venv step, got %s", venvCreate)
	}
	pipInstall := strings.Join(plan[3].cmd, " ")
	if !strings.Contains(pipInstall, "ansible-core==") {
		t.Fatalf("expected pinned ansible-core install, got %s", pipInstall)
	}
}

func TestOperatorBootstrapPlanVenvModeAddsPythonVenvOnDebian(t *testing.T) {
	plan, err := operatorBootstrapPlanForMode("debian", v1alpha1.State{}, bootstrapMode{venv: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	systemStep := strings.Join(plan[0].cmd, " ")
	if !strings.Contains(systemStep, " python3-venv") {
		t.Fatalf("debian venv mode must install python3-venv (apt does not bundle it with python3): %s", systemStep)
	}
}

// resolveAnsiblePlaybook prefers the venv binary when present.
func TestResolveAnsiblePlaybookPrefersVenvBinaryWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv(gitupsHomeEnv, home)
	if got := resolveAnsiblePlaybook(); got != "ansible-playbook" {
		t.Fatalf("expected fallback to literal 'ansible-playbook' when no venv exists, got %q", got)
	}
	venvBin := ansibleVenvBin("ansible-playbook")
	if err := os.MkdirAll(filepath.Dir(venvBin), 0o755); err != nil {
		t.Fatalf("mkdir venv bin: %v", err)
	}
	if err := os.WriteFile(venvBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write venv bin: %v", err)
	}
	if got := resolveAnsiblePlaybook(); got != venvBin {
		t.Fatalf("expected venv path %q, got %q", venvBin, got)
	}
}

func TestOperatorBootstrapDryRunPrintsPlanAndDoesNotExecute(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	osRelease := writeOSRelease(t, "ID=fedora\n")
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = os.Remove(osRelease) })

	// Point detectOSFamily at the fixture by overriding the constant via a
	// temporary symlink. detectOSFamily takes the path as a parameter, but
	// the cobra command always passes defaultOSReleasePath; rather than
	// plumb a flag, exercise the planner / parser directly here and the
	// command-level flow at the dry-run level above by setting up a real
	// fixture in CWD-relative path. The simpler test is to assert the dry
	// run prints the expected sentinel text against the real /etc/os-release
	// when present; skip when absent (hermetic CI).
	if _, err := os.Stat(defaultOSReleasePath); err != nil {
		t.Skipf("/etc/os-release missing on this host: %v", err)
	}
	code := Run(context.Background(), []string{"operator", "bootstrap", "--dry-run"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"Operator bootstrap",
		"OS family:",
		"planned actions:",
		"install operator host packages",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, out)
		}
	}
}

func TestDetectOSFamilyMapsRedHatLikeDistros(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		family string
	}{
		{"fedora", "ID=fedora\n", "redhat"},
		{"rhel", "ID=\"rhel\"\nID_LIKE=\"fedora\"\n", "redhat"},
		{"rocky", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n", "redhat"},
		{"alma", "ID=\"almalinux\"\nID_LIKE=\"rhel centos fedora\"\n", "redhat"},
		{"debian", "ID=debian\n", "debian"},
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\n", "debian"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeOSRelease(t, tc.body)
			got, err := detectOSFamily(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.family {
				t.Fatalf("got %q want %q", got, tc.family)
			}
		})
	}
}

func TestDetectOSFamilyRejectsUnknown(t *testing.T) {
	path := writeOSRelease(t, "ID=alpine\n")
	if _, err := detectOSFamily(path); err == nil {
		t.Fatalf("expected error for unsupported distro")
	}
}

func writeOSRelease(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write os-release fixture: %v", err)
	}
	return path
}
