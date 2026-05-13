package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:bundle
var bundleFS embed.FS

const bundleRoot = "bundle"

const AnsibleCfgRelPath = "ansible.cfg"

var RoleRelPaths = []string{
	filepath.Join("roles", "bastion"),
	filepath.Join("roles", "shared"),
	filepath.Join("roles", "providers"),
	filepath.Join("roles", "cluster_infra"),
	filepath.Join("roles", "openshift"),
}

func RolesPath(bundleDir string) string {
	paths := make([]string, 0, len(RoleRelPaths))
	for _, rel := range RoleRelPaths {
		paths = append(paths, filepath.Join(bundleDir, rel))
	}
	return strings.Join(paths, string(os.PathListSeparator))
}

const CollectionsRelPath = "collections"

const FilterPluginsRelPath = "filter_plugins"

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
