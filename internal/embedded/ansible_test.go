package embedded

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAnsibleBundleEitherSucceedsOrReportsEmpty(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "bundle")
	err := ExtractAnsibleBundle(dest)
	if err != nil {
		if !strings.Contains(err.Error(), "embedded ansible bundle is empty") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	for _, rel := range []string{
		AnsibleCfgRelPath,
		filepath.Join("playbooks", "checks", "preflight.yml"),
		filepath.Join("playbooks", "targets", "all", "apply.yml"),
		filepath.Join("playbooks", "targets", "infra", "apply.yml"),
		filepath.Join("playbooks", "targets", "infra", "destroy.yml"),
		filepath.Join("playbooks", "targets", "clusters", "apply.yml"),
		filepath.Join("playbooks", "targets", "clusters", "destroy.yml"),
		filepath.Join("playbooks", "layers", "providers", "apply.yml"),
		filepath.Join("playbooks", "layers", "cluster_infra", "apply.yml"),
	} {
		if _, statErr := os.Stat(filepath.Join(dest, rel)); statErr != nil {
			t.Fatalf("expected %s in extracted bundle: %v", rel, statErr)
		}
	}
	for _, rel := range RoleRelPaths {
		if _, statErr := os.Stat(filepath.Join(dest, rel)); statErr != nil {
			t.Fatalf("expected role search path %s in extracted bundle: %v", rel, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dest, "PLACEHOLDER")); statErr == nil {
		t.Fatalf("PLACEHOLDER must not appear in extracted bundle")
	}
}

func TestConfiguredRoleSearchPathsExistInSource(t *testing.T) {
	for _, rel := range RoleRelPaths {
		path := filepath.Join("..", "..", "ansible", rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected source role search path %s: %v", path, err)
		}
	}
}
