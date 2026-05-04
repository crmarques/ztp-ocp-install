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
	log := string(logData)
	for _, expected := range []string{"stdout-line\n", "stderr-line\n"} {
		if !strings.Contains(log, expected) {
			t.Fatalf("output log missing %q\n%s", expected, log)
		}
	}
}
