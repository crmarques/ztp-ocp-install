package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/gitops/catalog"
)

func BuildCatalog(prov *v1.GitOpsPackageSet, ws Workspace, cacheDir string) (*catalog.Catalog, error) {
	RegisterSourceResolvers(prov.Spec.Sources, PackageNamesByTemplate(prov), cacheDir)
	return catalog.Build(prov.Spec.Sources, ws.Root)
}

func RegisterSourceResolvers(sources []v1.PackageSource, names map[string][]string, cacheDir string) {
	for _, s := range sources {
		switch {
		case s.OCI != nil:
			r := &catalog.OCIResolver{
				CacheDir:     cacheDir,
				Stdout:       os.Stdout,
				Stderr:       os.Stderr,
				PackageNames: names[s.Name],
			}
			catalog.RegisterSourceResolver("oci", r.Resolve)
		case s.Git != nil:
			r := &catalog.GitResolver{
				CacheDir: cacheDir,
				Stdout:   os.Stdout,
				Stderr:   os.Stderr,
			}
			catalog.RegisterSourceResolver("git", r.Resolve)
		}
	}
}

func SourceCacheDir(stateDir string) string {
	return filepath.Join(stateDir, "gitops", "sources")
}

func PackageNamesFromExpandedPackageSet(fp *v1.GitOpsPackageSet) map[string][]string {
	out := map[string]map[string]struct{}{}
	for _, pkg := range fp.Spec.Resolved.Packages {
		parts := strings.SplitN(pkg.Template, "/", 2)
		if len(parts) != 2 {
			continue
		}
		set, ok := out[parts[0]]
		if !ok {
			set = map[string]struct{}{}
			out[parts[0]] = set
		}
		set[parts[1]] = struct{}{}
	}
	return flattenSourceNameMap(out)
}

func PackageNamesByTemplate(prov *v1.GitOpsPackageSet) map[string][]string {
	out := map[string]map[string]struct{}{}
	for _, repo := range prov.Spec.Repositories {
		for _, pkg := range repo.Packages {
			parts := strings.SplitN(pkg.Template, "/", 2)
			if len(parts) != 2 {
				continue
			}
			set, ok := out[parts[0]]
			if !ok {
				set = map[string]struct{}{}
				out[parts[0]] = set
			}
			set[parts[1]] = struct{}{}
		}
	}
	return flattenSourceNameMap(out)
}

func flattenSourceNameMap(in map[string]map[string]struct{}) map[string][]string {
	flat := map[string][]string{}
	for src, set := range in {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		flat[src] = names
	}
	return flat
}
