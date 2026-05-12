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
		filepath.Join("playbooks", "apply-all.yml"),
		filepath.Join("playbooks", "apply-infra.yml"),
		filepath.Join("playbooks", "apply-clusters.yml"),
		filepath.Join("playbooks", "destroy-infra.yml"),
		filepath.Join("playbooks", "clusters-destroy.yml"),
		filepath.Join("playbooks", "provider-prepare.yml"),
		filepath.Join("playbooks", "cluster-prepare.yml"),
	} {
		if _, statErr := os.Stat(filepath.Join(dest, rel)); statErr != nil {
			t.Fatalf("expected %s in extracted bundle: %v", rel, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dest, "PLACEHOLDER")); statErr == nil {
		t.Fatalf("PLACEHOLDER must not appear in extracted bundle")
	}
}
