package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/gitups/internal/infra"
)

func TestProviderCheckRunsAnsiblePreflight(t *testing.T) {
	stubPreflightAlwaysOK(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stateDir := t.TempDir()
	code := Run(context.Background(), []string{
		"check", "infra",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"playbooks/preflight.yml",
		"dry-run ansible command [infra check]:",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("provider check missing %q\n%s", expected, out)
		}
	}
}

func TestClustersCheckRunsAnsiblePreflightWithOpenshiftCLIs(t *testing.T) {
	stubPreflightAlwaysOK(t)
	stubBastionLookPathAlwaysOK(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stateDir := t.TempDir()
	code := Run(context.Background(), []string{
		"check", "clusters",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"openshift-install on PATH",
		"playbooks/preflight.yml",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("clusters check missing %q\n%s", expected, out)
		}
	}
}

func TestBastionCheckAcceptsState(t *testing.T) {
	stubBastionLookPathAlwaysOK(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"check", "bastion",
		"-f", "../../test/e2e/old/libvirt-1-host-1-sno-hub",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"bastion check",
		"openshift-install on PATH",
		"bastion check: all",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, out)
		}
	}
}

func stubBastionLookPathAlwaysOK(t *testing.T) {
	t.Helper()
	origLookPath := defaultPreflightDeps.lookPath
	defaultPreflightDeps.lookPath = func(name string, _ []string) (string, error) {
		return filepath.Join("/usr/bin", name), nil
	}
	t.Cleanup(func() {
		defaultPreflightDeps.lookPath = origLookPath
	})
}

func TestControllerBootstrapPlanVenvModeStaysUserOwned(t *testing.T) {
	plan, err := controllerBootstrapPlan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan) != 3 && len(plan) != 4 {
		t.Fatalf("expected 3 or 4 steps, got %d: %+v", len(plan), plan)
	}
	venvSteps := plan
	if len(plan) == 4 {
		if plan[0].cmd[0] != "sudo" {
			t.Fatalf("4-step plan must start with a sudo python install: %+v", plan)
		}
		venvSteps = plan[1:]
	}
	for _, step := range venvSteps {
		if step.cmd[0] == "sudo" {
			t.Fatalf("venv setup steps must not require sudo: %+v", step)
		}
	}
	venvCreate := strings.Join(venvSteps[0].cmd, " ")
	if !strings.Contains(venvCreate, "-m venv") {
		t.Fatalf("expected python -m venv step, got %s", venvCreate)
	}
	pipInstall := strings.Join(venvSteps[2].cmd, " ")
	if !strings.Contains(pipInstall, "ansible-core==") {
		t.Fatalf("expected pinned ansible-core install, got %s", pipInstall)
	}
}

func TestResolveAnsiblePlaybookPrefersVenvBinaryWhenPresent(t *testing.T) {
	clearGitupsEnv(t)
	home := t.TempDir()
	t.Setenv(gitupsUserDirEnv, home)
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

func TestBastionApplyDryRunPrintsPlanAndDoesNotExecute(t *testing.T) {
	clearGitupsEnv(t)
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"apply", "bastion", "--dry-run"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"bastion apply",
		"planned actions:",
		"create ansible-core venv",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, out)
		}
	}
}

func TestBastionApplyDryRunSkipsCLIsWithoutState(t *testing.T) {
	clearGitupsEnv(t)
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"apply", "bastion", "--dry-run"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "skipping OCP CLIs") {
		t.Fatalf("dry run without -f should announce skipping OCP CLIs\n%s", out)
	}
}

func TestBastionApplyDryRunPlansCLIsFromState(t *testing.T) {
	clearGitupsEnv(t)
	t.Setenv("HOME", t.TempDir())
	wantInstallDir := defaultControllerCLIInstallDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stateDir := t.TempDir()
	code := Run(context.Background(), []string{
		"apply", "bastion",
		"-f", "../../test/e2e/old/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected ok, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"install OCP CLIs",
		"setup-controller-clis.yml",
		"gitups_openshift_release_version=4.21.12",
		"gitups_clis_install_dir=" + wantInstallDir,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("CLI dry-run missing %q\n%s", expected, out)
		}
	}
	for _, leak := range []string{"qemu-kvm", "libvirt", "podman"} {
		if strings.Contains(out, " "+leak) {
			t.Fatalf("controller dry-run leaked provider package %q\n%s", leak, out)
		}
	}
}

func TestStateOpenshiftReleaseVersionPicksFirstNonEmpty(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if got := stateOpenshiftReleaseVersion(state); got != "4.21.12" {
		t.Fatalf("got %q want 4.21.12", got)
	}
}
