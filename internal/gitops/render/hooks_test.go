package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

type recordingHookRunner struct {
	called bool
	script string
}

func (r *recordingHookRunner) Run(_ context.Context, script, _, _, _, _ string) error {
	r.called = true
	r.script = script
	return nil
}

func TestRunHookRejectsUnsafePath(t *testing.T) {
	runner := &recordingHookRunner{}
	err := runHook(context.Background(), renderUnit{
		SourceDir: t.TempDir(),
		Hooks:     &v1.HookSpec{PreRender: "../hook.sh"},
	}, "pre-render", t.TempDir(), map[string]any{}, runner)
	if err == nil {
		t.Fatal("expected unsafe hook path to error")
	}
	if !strings.Contains(err.Error(), "hook pre-render") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.called {
		t.Fatal("unsafe hook path should not invoke runner")
	}
}

func TestRunHookUsesValidatedRelativePath(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(sourceDir, "hooks", "pre.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &recordingHookRunner{}
	err := runHook(context.Background(), renderUnit{
		SourceDir: sourceDir,
		Hooks:     &v1.HookSpec{PreRender: "hooks/pre.sh"},
	}, "pre-render", t.TempDir(), map[string]any{"name": "demo"}, runner)
	if err != nil {
		t.Fatalf("runHook returned error: %v", err)
	}
	if !runner.called {
		t.Fatal("expected hook runner to be called")
	}
	if runner.script != script {
		t.Fatalf("script path got %q, want %q", runner.script, script)
	}
}

func TestRenderOverlaysRejectsUnsafePath(t *testing.T) {
	err := renderOverlays(renderUnit{
		SourceDir: t.TempDir(),
		Overlays:  []string{"../overlays"},
	}, t.TempDir(), templateCtx{})
	if err == nil {
		t.Fatal("expected unsafe overlay path to error")
	}
	if !strings.Contains(err.Error(), "overlay") {
		t.Fatalf("unexpected error: %v", err)
	}
}
