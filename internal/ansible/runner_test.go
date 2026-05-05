package ansible

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommandRunnerIncludesAskBecomePass(t *testing.T) {
	command := CommandRunner{}.Command(RunSpec{
		Inventory:     "inventory.yaml",
		Playbook:      "playbook.yml",
		ExtraVars:     "vars.yml",
		AskBecomePass: true,
	})
	if got := command[len(command)-1]; got != "--ask-become-pass" {
		t.Fatalf("last arg got %q, want --ask-become-pass; command=%v", got, command)
	}
}

func TestCommandRunnerSavesCombinedOutputLogOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "fake-ansible-playbook")
	if err := os.WriteFile(executable, []byte(`#!/bin/sh
echo stdout-line
echo stderr-line >&2
exit 3
`), 0o755); err != nil {
		t.Fatalf("write fake ansible-playbook: %v", err)
	}

	artifactsDir := filepath.Join(dir, "artifacts")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := CommandRunner{Stdout: &stdout, Stderr: &stderr}.Run(context.Background(), RunSpec{
		Executable:   executable,
		Inventory:    "inventory.yaml",
		Playbook:     "playbook.yml",
		ExtraVars:    "vars.yml",
		ArtifactsDir: artifactsDir,
	})
	if err == nil {
		t.Fatal("expected command failure")
	}

	logPath := filepath.Join(artifactsDir, OutputLogName)
	if !strings.Contains(err.Error(), logPath) {
		t.Fatalf("error missing output log path %q: %v", logPath, err)
	}
	if got := stdout.String(); got != "stdout-line\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := stderr.String(); got != "stderr-line\n" {
		t.Fatalf("unexpected stderr: %q", got)
	}

	logData, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read output log: %v", readErr)
	}
	if info, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("stat output log: %v", statErr)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("output log mode got %03o, want 600", got)
	}
	if info, statErr := os.Stat(artifactsDir); statErr != nil {
		t.Fatalf("stat artifacts dir: %v", statErr)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("artifacts dir mode got %03o, want 700", got)
	}
	log := string(logData)
	for _, expected := range []string{"stdout-line\n", "stderr-line\n"} {
		if !strings.Contains(log, expected) {
			t.Fatalf("output log missing %q\n%s", expected, log)
		}
	}
}
