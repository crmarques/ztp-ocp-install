package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
)

const DefaultWorkspaceRoot = "./gitops-workspaces"

type Workspace struct {
	Name               string
	Root               string
	PackageSet         string
	ExpandedPackageSet string
	RenderRoot         string
}

func NewWorkspace(outputDir, name string) (Workspace, error) {
	if name == "" {
		return Workspace{}, fmt.Errorf("name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return Workspace{}, fmt.Errorf("name %q must not contain path separators", name)
	}
	if outputDir == "" {
		outputDir = DefaultWorkspaceRoot
	}
	root := filepath.Join(outputDir, name)
	return Workspace{
		Name:               name,
		Root:               root,
		PackageSet:         filepath.Join(root, "gitops-package-set.yaml"),
		ExpandedPackageSet: filepath.Join(root, ".bootwright", "expanded", "gitops-package-set.yaml"),
		RenderRoot:         filepath.Join(root, ".bootwright", "render"),
	}, nil
}

func AbsPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func ScaffoldPackageSet(name string) string {
	return fmt.Sprintf(`apiVersion: bootwright.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: %s
spec:
  # Package sources: where bootwright looks up package definitions.
  sources: []
  # - name: local
  #   filesystem:
  #     path: ./packages

  # Repositories select package installs and environment resources.
  repositories: []
  # - name: platform
  #   type: kubernetes-resources
  #   packages:
  #     - template: local/olm
  #     - template: local/metallb
  #       installMethod: helm
  # - name: platform-{{.Env}}
  #   type: kubernetes-resources
  #   repoRef:
  #     name: platform
  #     commit: v0.0.1
`, name)
}
