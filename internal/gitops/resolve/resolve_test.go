package resolve_test

import (
	"path/filepath"
	"testing"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/catalog"
	"github.com/crmarques/bootwright/internal/gitops/load"
	"github.com/crmarques/bootwright/internal/gitops/resolve"
)

func loadFixtures(t *testing.T) (*v1.GitOpsPackageSet, *catalog.Catalog) {
	t.Helper()
	repoRoot, _ := filepath.Abs("..")
	prov, err := load.PackageSet(filepath.Join(repoRoot, "testdata/dsv/gitops-package-set.yaml"))
	if err != nil {
		t.Fatalf("load package set: %v", err)
	}
	baseDir := filepath.Join(repoRoot, "tests/e2e")
	cat, err := catalog.Build(prov.Spec.Sources, baseDir)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	return prov, cat
}

func TestExpandTopoOrder(t *testing.T) {
	prov, cat := loadFixtures(t)
	fp, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	pos := map[string]int{}
	for i, rp := range fp.Spec.Resolved.Packages {
		pos[rp.Instance] = i
	}
	for _, name := range []string{"olm", "metallb", "nginx-ingress", "metallb-config-default"} {
		if _, ok := pos[name]; !ok {
			t.Fatalf("missing %s in expanded packages: %+v", name, fp.Spec.Resolved.Packages)
		}
	}
	if pos["metallb"] >= pos["nginx-ingress"] {
		t.Fatalf("metallb install must precede nginx-ingress install")
	}
	if pos["metallb"] >= pos["metallb-config-default"] {
		t.Fatalf("metallb install must precede metallb config resource")
	}
}

func TestExpandDerivesEnvResourcesFromRepoRef(t *testing.T) {
	prov, cat := loadFixtures(t)
	fp, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	got := map[string]string{}
	for _, rp := range fp.Spec.Resolved.Packages {
		got[rp.Instance] = rp.RenderedPaths.Repo + "/" + rp.RenderedPaths.Dir
	}
	if got["metallb"] != "basic-infra/packages/metallb/install/helm" {
		t.Fatalf("metallb install path: %q", got["metallb"])
	}
	if got["metallb-config-default"] != "basic-infra-dsv/packages/metallb/resources/config/default" {
		t.Fatalf("derived metallb config path: %q", got["metallb-config-default"])
	}
	wantManaged := "gitops-controllers-dsv/packages/argocd/kubernetes-resource-controller/managed-repo/basic-infra-dsv"
	if got["argocd-managed-repo-basic-infra-dsv"] != wantManaged {
		t.Fatalf("KRC managed-repo(basic-infra-dsv) path: %q", got["argocd-managed-repo-basic-infra-dsv"])
	}
}

func TestExpandPlaceholders(t *testing.T) {
	prov, cat := loadFixtures(t)
	fp, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	want := map[string]bool{
		"spec.resolved.packages[argocd-managed-repo-basic-infra].resolvedValues.repoURL":            false,
		"spec.resolved.packages[argocd-managed-repo-basic-infra-dsv].resolvedValues.repoURL":        false,
		"spec.resolved.packages[argocd-managed-repo-support-services].resolvedValues.repoURL":       false,
		"spec.resolved.packages[argocd-managed-repo-support-services-dsv].resolvedValues.repoURL":   false,
		"spec.resolved.packages[argocd-managed-repo-gitops-controllers-dsv].resolvedValues.repoURL": false,
		"spec.resolved.packages[metallb-config-default].resolvedValues.addressPools[0].cidrs[0]":    false,
		"spec.resolved.packages[vault].resolvedValues.server.dev.devRootToken":                      false,
	}
	for _, ph := range fp.Spec.Resolved.Placeholders {
		if _, ok := want[ph.Path]; ok {
			want[ph.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("missing expected placeholder: %s", path)
		}
	}
}

func TestExpandIdempotentPreservesUserEdits(t *testing.T) {
	prov, cat := loadFixtures(t)
	first, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("first expand: %v", err)
	}

	for i := range first.Spec.Resolved.Packages {
		rp := &first.Spec.Resolved.Packages[i]
		switch {
		case rp.Domain == v1.DomainKRC && rp.ResourceTemplate == "managed-repo":
			rp.ResolvedValues["repoURL"] = "https://git.example.com/" + rp.ResourceName + ".git"
		case rp.Instance == "metallb-config-default":
			pools := rp.ResolvedValues["addressPools"].([]any)
			pool := pools[0].(map[string]any)
			pool["cidrs"] = []any{"10.0.0.1-10.0.0.9"}
		case rp.Instance == "vault":
			server := rp.ResolvedValues["server"].(map[string]any)
			dev := server["dev"].(map[string]any)
			dev["devRootToken"] = "vault-test"
		}
	}

	second, err := resolve.Expand(prov, cat, resolve.Options{Prior: first})
	if err != nil {
		t.Fatalf("re-expand: %v", err)
	}

	for _, rp := range second.Spec.Resolved.Packages {
		switch {
		case rp.Instance == "argocd-managed-repo-basic-infra":
			want := "https://git.example.com/basic-infra.git"
			if got := rp.ResolvedValues["repoURL"]; got != want {
				t.Errorf("managed-repo(basic-infra).repoURL not preserved: got %v", got)
			}
		case rp.Instance == "metallb-config-default":
			pool := rp.ResolvedValues["addressPools"].([]any)[0].(map[string]any)
			cidrs := pool["cidrs"].([]any)
			if cidrs[0] != "10.0.0.1-10.0.0.9" {
				t.Errorf("metallb CIDR not preserved: got %v", cidrs)
			}
		}
	}
	if len(second.Spec.Resolved.Placeholders) != 0 {
		t.Errorf("expected empty placeholders after user edits, got %+v", second.Spec.Resolved.Placeholders)
	}
}

func TestExpandForcePreservesPlaceholderFills(t *testing.T) {
	prov, cat := loadFixtures(t)
	first, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for i := range first.Spec.Resolved.Packages {
		if first.Spec.Resolved.Packages[i].Instance == "argocd-managed-repo-basic-infra" {
			first.Spec.Resolved.Packages[i].ResolvedValues["repoURL"] = "https://user-edit.example.com/"
		}
	}
	second, err := resolve.Expand(prov, cat, resolve.Options{Prior: first, Force: true})
	if err != nil {
		t.Fatalf("force expand: %v", err)
	}
	for _, rp := range second.Spec.Resolved.Packages {
		if rp.Instance != "argocd-managed-repo-basic-infra" {
			continue
		}
		if got := rp.ResolvedValues["repoURL"]; got != "https://user-edit.example.com/" {
			t.Errorf("force expand should preserve user-authored fill, got %v", got)
		}
	}
}

func TestExpandRepeatedExplicitResources(t *testing.T) {
	prov, cat := loadFixtures(t)
	prov.Spec.Controllers = nil
	prov.Spec.Repositories = []v1.RepositoryDecl{
		{
			Name: "base",
			Type: "kubernetes-resources",
			Packages: []v1.PackageRef{
				{Template: "local/metallb", InstallMethod: "helm"},
			},
		},
		{
			Name: "base-dsv",
			Type: "kubernetes-resources",
			RepoRef: &v1.RepositoryRef{
				Name:   "base",
				Commit: "v0.0.1",
			},
			Packages: []v1.PackageRef{
				{
					Template: "local/metallb",
					Resources: []v1.ResourceRef{
						{
							Template: "config",
							Name:     "pool-a",
							Values: map[string]any{
								"addressPools": []any{map[string]any{"name": "a", "cidrs": []any{"10.0.0.1-10.0.0.2"}}},
							},
						},
						{
							Template: "config",
							Name:     "pool-b",
							Values: map[string]any{
								"addressPools": []any{map[string]any{"name": "b", "cidrs": []any{"10.0.1.1-10.0.1.2"}}},
							},
						},
					},
				},
			},
		},
	}
	fp, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	got := map[string]string{}
	for _, rp := range fp.Spec.Resolved.Packages {
		got[rp.Instance] = rp.RenderedPaths.Dir
	}
	if got["metallb-config-pool-a"] != "packages/metallb/resources/config/pool-a" {
		t.Fatalf("pool-a path mismatch: %+v", got)
	}
	if got["metallb-config-pool-b"] != "packages/metallb/resources/config/pool-b" {
		t.Fatalf("pool-b path mismatch: %+v", got)
	}
}

func TestExpandRejectsUnknownTemplate(t *testing.T) {
	prov, cat := loadFixtures(t)
	prov.Spec.Controllers = nil
	prov.Spec.Repositories = []v1.RepositoryDecl{
		{
			Name: "base",
			Type: "kubernetes-resources",
			Packages: []v1.PackageRef{
				{Template: "local/no-such-pkg"},
			},
		},
	}
	_, err := resolve.Expand(prov, cat, resolve.Options{})
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestExpandEnvKeyOverridesMetadataName(t *testing.T) {
	prov, cat := loadFixtures(t)
	prov.Metadata.Name = "basic-infra-dev"
	prov.Spec.EnvKey = "dev"
	fp, err := resolve.Expand(prov, cat, resolve.Options{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	var seen string
	for _, rp := range fp.Spec.Resolved.Packages {
		if rp.Instance == "metallb-config-default" {
			seen = rp.Repository
			break
		}
	}
	if seen != "basic-infra-dev" {
		t.Errorf("metallb config repository: got %q want %q", seen, "basic-infra-dev")
	}
}

func TestExpandRecordsExtendedFrom(t *testing.T) {
	prov, cat := loadFixtures(t)
	ef := &v1.ExtendedFrom{Source: "../basic-infra/gitops-package-set.yaml", Ref: "v1.4.0"}
	fp, err := resolve.Expand(prov, cat, resolve.Options{ExtendedFrom: ef})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if fp.Spec.Resolved.ExtendedFrom == nil || fp.Spec.Resolved.ExtendedFrom.Source != ef.Source || fp.Spec.Resolved.ExtendedFrom.Ref != ef.Ref {
		t.Errorf("ExtendedFrom not persisted: %+v", fp.Spec.Resolved.ExtendedFrom)
	}
}
