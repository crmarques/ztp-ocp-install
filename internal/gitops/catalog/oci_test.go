package catalog_test

import (
	"strings"
	"testing"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/gitops/catalog"
)

func TestOCIResolverRejectsMissingDigest(t *testing.T) {
	resolver := &catalog.OCIResolver{
		CacheDir:     t.TempDir(),
		PackageNames: []string{"metallb"},
	}
	_, err := resolver.Resolve(v1.PackageSource{
		Name: "remote",
		OCI: &v1.PackageSourceOCI{
			Registry: "ghcr.io/acme/gitups-packages",
			Tag:      "v0.1.0",
		},
	}, "")
	if err == nil {
		t.Fatal("expected missing digest to error")
	}
	if !strings.Contains(err.Error(), "oci.digest") {
		t.Fatalf("unexpected error: %v", err)
	}
}
