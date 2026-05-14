package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
)

type KustomizeBuildRequest struct {
	Base string
}

type KustomizeRunner interface {
	Build(ctx context.Context, req KustomizeBuildRequest) (string, error)
}

type ExecKustomizeRunner struct {
	Bin string
}

func NewExecKustomizeRunner(bin string) *ExecKustomizeRunner {
	if bin == "" {
		bin = "kustomize"
	}
	return &ExecKustomizeRunner{Bin: bin}
}

func (r *ExecKustomizeRunner) Build(ctx context.Context, req KustomizeBuildRequest) (string, error) {
	cmd := exec.CommandContext(ctx, r.Bin, "build", req.Base)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kustomize build: %w: %s", err, string(out))
	}
	return string(out), nil
}

func renderKustomize(ctx context.Context, rp *v1.ResolvedPackage, unit renderUnit, pkgDir string, tctx templateCtx, runner KustomizeRunner) error {
	ks := unit.Kustomize
	if ks == nil {
		return fmt.Errorf("renderer=kustomize but spec.kustomize is nil")
	}
	base, err := sourcePath(unit.SourceDir, "spec.kustomize.base", ks.Base)
	if err != nil {
		return err
	}
	if ks.ValuesTemplate != "" {
		tmplPath, err := sourcePath(unit.SourceDir, "spec.kustomize.valuesTemplate", ks.ValuesTemplate)
		if err != nil {
			return err
		}
		rendered, err := renderTemplateFile(tmplPath, tctx)
		if err != nil {
			return fmt.Errorf("kustomize values template: %w", err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "values.yaml"), []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write values.yaml: %w", err)
		}
	}
	out, err := runner.Build(ctx, KustomizeBuildRequest{Base: base})
	if err != nil {
		return err
	}
	_ = rp
	return os.WriteFile(filepath.Join(pkgDir, "install.yaml"), []byte(out), 0o644)
}
