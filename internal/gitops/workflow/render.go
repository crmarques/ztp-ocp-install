package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/catalog"
	gitopsrender "github.com/crmarques/bootwright/internal/gitops/render"
)

func EnsureRenderBinaries() error {
	for _, bin := range []string{"helm", "kustomize"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required binary %q not found in PATH", bin)
		}
	}
	return nil
}

func CurrentKubectlContext() string { return "" }

func VerifyDeterminism(ctx context.Context, fp *v1.GitOpsPackageSet, cat *catalog.Catalog, first gitopsrender.Options, ws Workspace, out io.Writer) error {
	scratchRoot, err := os.MkdirTemp("", "bootwright-det-")
	if err != nil {
		return fmt.Errorf("create determinism scratch: %w", err)
	}
	defer os.RemoveAll(scratchRoot)
	scratchOut := filepath.Join(scratchRoot, ws.Name)
	second := first
	second.OutputPath = scratchOut
	second.PreserveExtras = false
	second.Prune = false
	if err := gitopsrender.Render(ctx, fp, cat, second); err != nil {
		return fmt.Errorf("determinism re-render: %w", err)
	}
	drifts, err := DiffWorkspace(ws.RenderRoot, scratchOut)
	if err != nil {
		return fmt.Errorf("determinism diff: %w", err)
	}
	filtered := drifts[:0]
	for _, d := range drifts {
		if d.Kind == "extra" || d.Kind == "orphan-dir" {
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) == 0 {
		return nil
	}
	fmt.Fprintf(out, "bootwright: determinism check failed — %d file(s) differ between two render passes:\n", len(filtered))
	for _, d := range filtered {
		fmt.Fprintf(out, "  %-12s %s\n", d.Kind, d.Path)
	}
	return fmt.Errorf("non-deterministic render; inspect chart values for timestamps, random IDs, or auto-generated secrets, or pass --skip-determinism-check to bypass")
}
