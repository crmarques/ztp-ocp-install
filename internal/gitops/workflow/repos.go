package workflow

import v1 "github.com/crmarques/bootwright/api/v1alpha1"

func RenderedRepoNames(fp *v1.GitOpsPackageSet) []string {
	seen := map[string]bool{}
	var out []string
	for i := range fp.Spec.Resolved.Packages {
		r := fp.Spec.Resolved.Packages[i].RenderedPaths.Repo
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}
