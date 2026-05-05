// Package embedded ships the repository's Ansible workflow bundle inside the
// gitups binary so the CLI is installable independent of the source tree. The
// bundle is materialised on disk under the user's state directory at runtime.
package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed bundle
var bundleFS embed.FS

const bundleRoot = "bundle"

// AnsibleCfgRelPath is the path of ansible.cfg inside the extracted bundle.
const AnsibleCfgRelPath = "ansible.cfg"

// RolesRelPath is the path of the roles directory inside the extracted bundle.
const RolesRelPath = "roles"

// CollectionsRelPath is the path of the collections directory inside the
// extracted bundle.
const CollectionsRelPath = "collections"

// FilterPluginsRelPath is the path of the Jinja filter-plugins directory
// inside the extracted bundle. The runner exports this as
// ANSIBLE_FILTER_PLUGINS so plugins ship with the binary.
const FilterPluginsRelPath = "filter_plugins"

// ExtractAnsibleBundle writes the embedded Ansible tree to dest, replacing any
// previous contents at that location. The destination is left in a clean state
// containing only the embedded files so a stale extraction cannot leak old
// playbooks into a new run.
func ExtractAnsibleBundle(dest string) error {
	sub, err := fs.Sub(bundleFS, bundleRoot)
	if err != nil {
		return fmt.Errorf("locate embedded ansible bundle: %w", err)
	}
	if _, err := fs.Stat(sub, AnsibleCfgRelPath); err != nil {
		return fmt.Errorf("embedded ansible bundle is empty (rebuild gitups via 'make build'): %w", err)
	}
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("clear bundle destination %s: %w", dest, err)
	}
	return fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "PLACEHOLDER" {
			return nil
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
