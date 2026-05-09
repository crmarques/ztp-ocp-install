package render_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/crmarques/gitups/internal/gitops/catalog"
	"github.com/crmarques/gitups/internal/gitops/load"
	"github.com/crmarques/gitups/internal/gitops/render"
	"github.com/crmarques/gitups/internal/gitops/resolve"
)

// stubHelmRunner returns a canned install.yaml per chart so tests don't shell
// out to the real `helm` binary. Output is stable across runs (hermetic) and
// small enough to diff.
type stubHelmRunner struct{}

var stubHelmOutputs = map[string]string{
	"argo-cd":       "# stub-helm: argo-cd\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: argocd-server\nspec:\n  replicas: 1\n",
	"metallb":       "# stub-helm: metallb\napiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: metallb-speaker\n",
	"ingress-nginx": "# stub-helm: ingress-nginx\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: ingress-nginx-controller\n",
	"gitea":         "# stub-helm: gitea\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: gitea\n",
	"vault":         "# stub-helm: vault\napiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: vault\n",
	"base":          "# stub-helm: istio-base\napiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: virtualservices.networking.istio.io\n",
	"istiod":        "# stub-helm: istiod\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: istiod\n",
}

func (stubHelmRunner) Template(ctx context.Context, req render.HelmTemplateRequest) (string, error) {
	body, ok := stubHelmOutputs[req.Chart]
	if !ok {
		return "", fmt.Errorf("stub helm: no canned output for chart %q", req.Chart)
	}
	header := fmt.Sprintf("# stub: instance=%s chart=%s version=%s namespace=%s\n", req.Instance, req.Chart, req.Version, req.Namespace)
	return header + body, nil
}

// TestRenderRuns drives expand and render end-to-end against the pinned
// internal/testdata/dsv/gitops-package-set.yaml fixture and the sibling
// gitups-packages catalog.
func TestRenderRuns(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	packageSetPath := filepath.Join(repoRoot, "testdata/dsv/gitops-package-set.yaml")
	p, err := load.PackageSet(packageSetPath)
	if err != nil {
		t.Fatalf("load package set: %v", err)
	}
	// baseDir stays at tests/e2e so the fixture's relative source path
	// (../../../gitups-packages/packages) resolves to the sibling catalog; the
	// fixture itself lives one level deeper but is byte-identical to the
	// workspace copy that e2e runs against.
	baseDir := filepath.Join(repoRoot, "tests/e2e")
	cat, err := catalog.Build(p.Spec.Sources, baseDir)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	fp, err := resolve.Expand(p, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "dsv")
	err = render.Render(context.Background(), fp, cat, render.Options{
		OutputPath:        outDir,
		KubectlContext:    "dsv",
		Helm:              stubHelmRunner{},
		SourcePackageSet:  packageSetPath,
		AllowPlaceholders: true,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
}
