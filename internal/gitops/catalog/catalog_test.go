package catalog_test

import (
	"path/filepath"
	"testing"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/gitops/catalog"
)

func TestCatalogBuildFilesystem(t *testing.T) {
	repoRoot, _ := filepath.Abs("..")
	sources := []v1.PackageSource{
		{Name: "local", Filesystem: &v1.PackageSourceFilesystem{Path: "../../../gitops-workspace/gitups-packages/packages"}},
	}
	cat, err := catalog.Build(sources, repoRoot)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, want := range []string{"local/argocd", "local/metallb", "local/nginx-ingress"} {
		if _, ok := cat.Lookup(want); !ok {
			t.Errorf("missing entry %q (known: %v)", want, cat.Qualified())
		}
	}
}

func TestCatalogRejectsSourceWithNoSubBlock(t *testing.T) {
	_, err := catalog.Build([]v1.PackageSource{{Name: "x"}}, ".")
	if err == nil {
		t.Fatal("expected error for source with no sub-block set")
	}
}

func TestCatalogRejectsUnregisteredOCISource(t *testing.T) {
	_, err := catalog.Build([]v1.PackageSource{{
		Name: "x",
		OCI:  &v1.PackageSourceOCI{Registry: "ghcr.io/x", Tag: "0.0.1"},
	}}, ".")
	if err == nil {
		t.Fatal("expected error when oci driver is not registered")
	}
}
