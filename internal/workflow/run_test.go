package workflow

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/ansible"
)

// fakeRunner satisfies ansible.Runner without actually exec'ing.
type fakeRunner struct {
	runCalled  bool
	command    []string
	runReturns error
}

func (f *fakeRunner) Run(_ context.Context, spec ansible.RunSpec) error {
	f.runCalled = true
	f.command = f.Command(spec)
	return f.runReturns
}

func (f *fakeRunner) Command(spec ansible.RunSpec) []string {
	executable := spec.Executable
	if executable == "" {
		executable = "ansible-playbook"
	}
	return []string{executable, "-i", spec.Inventory, spec.Playbook}
}

// minimalState is the smallest state that satisfies render.All without
// triggering provider-closure validation in the installer renderer. It has
// no OCPClusters (so no installer renders) and an empty ClusterInfra list
// (so the per-cluster vars loop is a no-op). Sufficient for exercising the
// dispatch / dry-run / runner-execution paths of workflow.Run.
func minimalState() v1alpha1.State {
	return v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
		}},
	}
}

func TestRunDryRunDoesNotInvokeRunner(t *testing.T) {
	stateDir := t.TempDir()
	runner := &fakeRunner{}
	var out bytes.Buffer
	result, err := Run(context.Background(), RunOptions{
		State:             minimalState(),
		StateDir:          stateDir,
		SecretsDir:        t.TempDir(),
		HostStateDir:      "/var/lib/gitups",
		Executable:        "ansible-playbook",
		BundleDir:         t.TempDir(),
		Playbook:          "playbooks/checks/preflight.yml",
		ArtifactsBaseName: "preflight-test",
		DryRun:            true,
		Label:             "test check",
	}, runner, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runner.runCalled {
		t.Fatal("dry-run must not invoke runner.Run")
	}
	if result.Render.VarsPath == "" {
		t.Fatal("render should still emit a VarsPath even on dry-run")
	}
	if !strings.Contains(out.String(), "dry-run ansible command [test check]:") {
		t.Fatalf("expected dry-run echo with label, got %q", out.String())
	}
	if len(result.Command) == 0 || result.Command[0] != "ansible-playbook" {
		t.Fatalf("expected command[0]=ansible-playbook, got %v", result.Command)
	}
}

func TestRunExecutesRunnerWhenNotDryRun(t *testing.T) {
	stateDir := t.TempDir()
	runner := &fakeRunner{}
	var out bytes.Buffer
	if _, err := Run(context.Background(), RunOptions{
		State:             minimalState(),
		StateDir:          stateDir,
		SecretsDir:        t.TempDir(),
		HostStateDir:      "/var/lib/gitups",
		BundleDir:         t.TempDir(),
		Playbook:          "playbooks/targets/infra/apply.yml",
		ArtifactsBaseName: "infra",
		DryRun:            false,
	}, runner, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !runner.runCalled {
		t.Fatal("non-dry-run must invoke runner.Run")
	}
	if strings.Contains(out.String(), "dry-run") {
		t.Fatalf("non-dry-run must not echo dry-run line; got %q", out.String())
	}
}

func TestRunPropagatesRunnerError(t *testing.T) {
	runner := &fakeRunner{runReturns: errors.New("boom")}
	_, err := Run(context.Background(), RunOptions{
		State:             minimalState(),
		StateDir:          t.TempDir(),
		SecretsDir:        t.TempDir(),
		HostStateDir:      "/var/lib/gitups",
		BundleDir:         t.TempDir(),
		Playbook:          "playbooks/targets/infra/apply.yml",
		ArtifactsBaseName: "infra",
	}, runner, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Run must propagate runner error, got %v", err)
	}
}

func TestRunDryRunLabelFallsBackToPlaybook(t *testing.T) {
	runner := &fakeRunner{}
	var out bytes.Buffer
	_, err := Run(context.Background(), RunOptions{
		State:             minimalState(),
		StateDir:          t.TempDir(),
		SecretsDir:        t.TempDir(),
		HostStateDir:      "/var/lib/gitups",
		BundleDir:         t.TempDir(),
		Playbook:          "playbooks/checks/preflight.yml",
		ArtifactsBaseName: "preflight-test",
		DryRun:            true,
		// Label intentionally empty.
	}, runner, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "[playbooks/checks/preflight.yml]") {
		t.Fatalf("empty Label should fall back to Playbook, got %q", out.String())
	}
}

func TestRunRenderFailureSurfaces(t *testing.T) {
	// Pointing StateDir at a path that already exists as a *file* causes
	// MkdirAll to fail with a recognizable error.
	stateFile := t.TempDir() + "/not-a-dir"
	if err := writeStub(stateFile); err != nil {
		t.Fatalf("setup: %v", err)
	}
	runner := &fakeRunner{}
	_, err := Run(context.Background(), RunOptions{
		State:             minimalState(),
		StateDir:          stateFile, // file, not directory
		SecretsDir:        t.TempDir(),
		HostStateDir:      "/var/lib/gitups",
		BundleDir:         t.TempDir(),
		Playbook:          "playbooks/checks/preflight.yml",
		ArtifactsBaseName: "preflight-test",
	}, runner, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run should fail when StateDir is unwritable")
	}
	if runner.runCalled {
		t.Fatal("runner should not be invoked when render fails")
	}
}

func writeStub(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty arg", []string{"", "x"}, "'' x"},
		{"plain", []string{"ansible-playbook", "-i", "inv"}, "ansible-playbook -i inv"},
		{"space-containing", []string{"echo", "hello world"}, "echo 'hello world'"},
		{"with-quotes", []string{"foo", "a'b"}, "foo 'a'\\''b'"},
		{"with-dollar", []string{"sh", "-c", "$X"}, "sh -c '$X'"},
		{"with-tab", []string{"x\ty"}, "'x\ty'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShellQuote(tc.in); got != tc.want {
				t.Fatalf("ShellQuote(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
