package load_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/gitups/internal/gitops/load"
)

func TestGitOpsPackageSetLoads(t *testing.T) {
	repoRoot, _ := filepath.Abs("..")
	p, err := load.PackageSet(filepath.Join(repoRoot, "testdata/dsv/gitops-package-set.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Metadata.Name != "dsv" {
		t.Errorf("metadata.name: got %q", p.Metadata.Name)
	}
	if len(p.Spec.Repositories) != 6 {
		t.Errorf("want 6 repositories, got %d", len(p.Spec.Repositories))
	}
}

func TestExpandedGitOpsPackageSetLoads(t *testing.T) {
	path := writeFile(t, t.TempDir(), "gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: demo
spec:
  sources:
    - name: local
      filesystem:
        path: ./packages
  repositories:
    - name: platform
      type: kubernetes-resources
      packages:
        - template: local/metallb
  resolved:
    repository:
      layout: split
      outputPath: .gitups/render/demo
    packages:
      - template: local/metallb
        unitType: install
        domain: install
        installMethod: helm
        repository: platform
        instance: metallb
        role: workload
        renderer: helm
        resolvedValues: {}
        renderedPaths:
          repo: platform
          dir: packages/metallb/install/helm
        applyWave: 0
    placeholders: []
`)
	fp, err := load.ExpandedPackageSet(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if fp.Spec.Resolved.Repository.Layout != "split" {
		t.Errorf("layout: got %q", fp.Spec.Resolved.Repository.Layout)
	}
}

func TestDetectKind(t *testing.T) {
	repoRoot, _ := filepath.Abs("..")
	tm, err := load.DetectKind(filepath.Join(repoRoot, "testdata/dsv/gitops-package-set.yaml"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if tm.Kind != "GitOpsPackageSet" {
		t.Errorf("kind: got %q", tm.Kind)
	}
}

// TestGitOpsPackageSetResolvedNoExtends exercises the back-compat path: a GitOpsPackageSet
// without spec.extends must load identically through GitOpsPackageSet and
// GitOpsPackageSetResolved, with a nil ExtendedFrom trace.
func TestGitOpsPackageSetResolvedNoExtends(t *testing.T) {
	repoRoot, _ := filepath.Abs("..")
	path := filepath.Join(repoRoot, "testdata/dsv/gitops-package-set.yaml")
	p, extFrom, err := load.PackageSetResolved(path)
	if err != nil {
		t.Fatalf("resolved: %v", err)
	}
	if extFrom != nil {
		t.Errorf("want nil ExtendedFrom, got %+v", extFrom)
	}
	if p.Metadata.Name != "dsv" {
		t.Errorf("metadata.name: got %q", p.Metadata.Name)
	}
	if len(p.Spec.Repositories) != 6 {
		t.Errorf("want 6 repositories, got %d", len(p.Spec.Repositories))
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// TestGitOpsPackageSetResolvedExtendsMerge covers the core extends behavior: env
// picks up base sources and repositories, merges values on matching packages,
// appends env-only packages, and records ExtendedFrom.
func TestGitOpsPackageSetResolvedExtendsMerge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "basic-infra/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: basic-infra
spec:
  sources:
    - name: local
      filesystem:
        path: ./pkgs
  repositories:
    - name: base
      type: kubernetes-resources
      packages:
        - template: local/metallb
        - template: local/argocd
`)
	writeFile(t, dir, "basic-infra-dev/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: basic-infra-dev
spec:
  envKey: dev
  extends:
    source: ../basic-infra/gitops-package-set.yaml
    ref: v1.4.0
  repositories:
    - name: base-{{.Env}}
      type: kubernetes-resources
      repoRef:
        name: base
        commit: v0.0.1
`)
	merged, extFrom, err := load.PackageSetResolved(filepath.Join(dir, "basic-infra-dev/gitops-package-set.yaml"))
	if err != nil {
		t.Fatalf("resolved: %v", err)
	}
	if extFrom == nil || extFrom.Source != "../basic-infra/gitops-package-set.yaml" || extFrom.Ref != "v1.4.0" {
		t.Fatalf("ExtendedFrom: got %+v", extFrom)
	}
	if merged.Spec.Extends != nil {
		t.Errorf("effective GitOpsPackageSet must not carry spec.extends, got %+v", merged.Spec.Extends)
	}
	if merged.Metadata.Name != "basic-infra-dev" {
		t.Errorf("metadata.name: got %q", merged.Metadata.Name)
	}
	if merged.Spec.EnvKey != "dev" {
		t.Errorf("envKey: got %q", merged.Spec.EnvKey)
	}
	if len(merged.Spec.Sources) != 1 || merged.Spec.Sources[0].Name != "local" {
		t.Fatalf("sources: got %+v", merged.Spec.Sources)
	}
	if len(merged.Spec.Repositories) != 2 {
		t.Fatalf("repositories: want 2, got %d (%+v)", len(merged.Spec.Repositories), merged.Spec.Repositories)
	}
	if merged.Spec.Repositories[0].Name != "base" || merged.Spec.Repositories[1].Name != "base-{{.Env}}" {
		t.Fatalf("unexpected repository order: %+v", merged.Spec.Repositories)
	}
}

// TestGitOpsPackageSetResolvedRejectsTransitiveExtends: a base GitOpsPackageSet cannot
// itself declare spec.extends — we keep resolution to a single level so
// state is easy to reason about.
func TestGitOpsPackageSetResolvedRejectsTransitiveExtends(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: a}
spec:
  sources: [{name: local, filesystem: {path: ./pkgs}}]
  repositories:
    - name: base
      type: kubernetes-resources
      packages: [{template: local/x}]
`)
	writeFile(t, dir, "b/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: b}
spec:
  extends: {source: ../a/gitops-package-set.yaml}
  repositories:
    - name: env
      type: kubernetes-resources
      repoRef: {name: base}
`)
	writeFile(t, dir, "c/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: c}
spec:
  extends: {source: ../b/gitops-package-set.yaml}
  repositories:
    - name: env2
      type: kubernetes-resources
      repoRef: {name: base}
`)
	_, _, err := load.PackageSetResolved(filepath.Join(dir, "c/gitops-package-set.yaml"))
	if err == nil {
		t.Fatal("expected transitive extends to error")
	}
	if !strings.Contains(err.Error(), "one level") {
		t.Errorf("error should mention single-level rule: %v", err)
	}
}

// TestGitOpsPackageSetResolvedRejectsGitSource defers git+ source support to a
// follow-up; today it must fail with a clear, user-facing error.
func TestGitOpsPackageSetResolvedRejectsGitSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "env/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: env}
spec:
  extends:
    source: git+https://example.com/acme/gitops.git#basic-infra/gitops-package-set.yaml
    ref: v1.0.0
  repositories: []
`)
	_, _, err := load.PackageSetResolved(filepath.Join(dir, "env/gitops-package-set.yaml"))
	if err == nil {
		t.Fatal("expected git+ source to be rejected")
	}
	if !strings.Contains(err.Error(), "git+") {
		t.Errorf("error should mention git+ prefix: %v", err)
	}
}

// TestGitOpsPackageSetResolvedSourceConflict: a source name that appears in both
// base and env is ambiguous and must error rather than silently preferring
// one side's definition.
func TestGitOpsPackageSetResolvedSourceConflict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "base/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: base}
spec:
  sources: [{name: local, filesystem: {path: ./pkgs}}]
  repositories:
    - name: base
      type: kubernetes-resources
      packages: [{template: local/x}]
`)
	writeFile(t, dir, "env/gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: env}
spec:
  extends: {source: ../base/gitops-package-set.yaml}
  sources: [{name: local, filesystem: {path: ./other}}]
  repositories: []
`)
	_, _, err := load.PackageSetResolved(filepath.Join(dir, "env/gitops-package-set.yaml"))
	if err == nil {
		t.Fatal("expected source conflict to error")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("error should name the conflicting source: %v", err)
	}
}

func TestGitOpsPackageSetRejectsOldTopLevelPackages(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: dsv}
spec:
  sources: [{name: local, filesystem: {path: ./pkgs}}]
  packages: [{template: local/old}]
`)
	_, err := load.PackageSet(path)
	if err == nil {
		t.Fatal("expected old spec.packages to be rejected")
	}
	if !strings.Contains(err.Error(), "spec.packages") {
		t.Errorf("error should mention spec.packages: %v", err)
	}
}

func TestPackageSetRejectsUnknownFields(t *testing.T) {
	path := writeFile(t, t.TempDir(), "gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: dsv}
spec:
  sources: []
  repositories: []
  typo: true
`)
	_, err := load.PackageSet(path)
	if err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
	if !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackageSetRejectsUnsafeGitSourcePath(t *testing.T) {
	path := writeFile(t, t.TempDir(), "gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: dsv}
spec:
  sources:
    - name: local
      git:
        url: https://example.com/packages.git
        ref: v0.1.0
        path: ../packages
  repositories: []
`)
	_, err := load.PackageSet(path)
	if err == nil {
		t.Fatal("expected unsafe git.path to be rejected")
	}
	if !strings.Contains(err.Error(), "git.path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpandedPackageSetRejectsUnsafeRenderedPath(t *testing.T) {
	path := writeFile(t, t.TempDir(), "gitops-package-set.yaml", `apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata: {name: demo}
spec:
  resolved:
    repository:
      layout: split
    packages:
      - template: local/metallb
        unitType: install
        domain: install
        installMethod: helm
        repository: platform
        instance: metallb
        role: workload
        renderer: helm
        resolvedValues: {}
        renderedPaths:
          repo: platform
          dir: ../outside
        applyWave: 0
`)
	_, err := load.ExpandedPackageSet(path)
	if err == nil {
		t.Fatal("expected unsafe rendered path to be rejected")
	}
	if !strings.Contains(err.Error(), "renderedPaths.dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}
