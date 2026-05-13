package render

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type HookRunner interface {
	Run(ctx context.Context, script string, workDir string, phase string, valuesFile string, outDir string) error
}

type execHookRunner struct{}

func NewExecHookRunner() HookRunner { return &execHookRunner{} }

func (execHookRunner) Run(ctx context.Context, script, workDir, phase, valuesFile, outDir string) error {
	cmd := exec.CommandContext(ctx, script, "--phase", phase, "--values", valuesFile, "--out", outDir)
	cmd.Dir = workDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runHook(ctx context.Context, unit renderUnit, phase string, pkgDir string, values map[string]any, runner HookRunner) error {
	if unit.Hooks == nil {
		return nil
	}
	var script string
	switch phase {
	case "pre-render":
		script = unit.Hooks.PreRender
	case "post-render":
		script = unit.Hooks.PostRender
	}
	if script == "" {
		return nil
	}
	scriptPath, err := sourcePath(unit.SourceDir, "hook "+phase, script)
	if err != nil {
		return err
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		return fmt.Errorf("hook %s: %w", phase, err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("hook %s: %s is not executable", phase, scriptPath)
	}

	tmp, err := os.CreateTemp("", "gitups-hook-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(values); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("write hook values: %w; close temp file: %v", err, closeErr)
		}
		return fmt.Errorf("write hook values: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close hook values file: %w", err)
	}

	if err := runner.Run(ctx, scriptPath, unit.SourceDir, phase, tmpPath, pkgDir); err != nil {
		return fmt.Errorf("hook %s: %w", phase, err)
	}
	return nil
}
