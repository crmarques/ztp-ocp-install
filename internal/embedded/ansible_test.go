package embedded

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bundle may or may not be synced into internal/embedded/bundle when this
// test runs (it is gitignored and only populated by `make build`). We assert
// the contract either way: extraction succeeds and yields ansible.cfg, or it
// fails with the documented "rebuild gitups" hint.
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
		filepath.Join("playbooks", "apply-infra.yml"),
		filepath.Join("playbooks", "apply-ocp.yml"),
		filepath.Join("playbooks", "destroy-all.yml"),
		filepath.Join("playbooks", "destroy-infra.yml"),
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
