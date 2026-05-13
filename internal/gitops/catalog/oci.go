package catalog

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

// OCIResolver materializes an OCI package source by shelling out to `oras`,
// pulling one artifact per package name into <CacheDir>/oci/<encoded-ref>/.
type OCIResolver struct {
	CacheDir     string
	Stdout       io.Writer
	Stderr       io.Writer
	PackageNames []string
}

func (r *OCIResolver) Resolve(s v1.PackageSource, _ string) (string, error) {
	if s.OCI == nil {
		return "", fmt.Errorf("source %q: oci sub-block is nil", s.Name)
	}
	if r.CacheDir == "" {
		return "", fmt.Errorf("source %q: oci resolver CacheDir is required", s.Name)
	}
	if len(r.PackageNames) == 0 {
		return "", fmt.Errorf("source %q: oci resolver requires PackageNames (set from GitOpsPackageSet)", s.Name)
	}
	if s.OCI.Digest == "" {
		return "", fmt.Errorf("source %q: oci.digest is required", s.Name)
	}
	root := filepath.Join(r.CacheDir, "oci", encodeRef(s.OCI))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("source %q: mkdir cache: %w", s.Name, err)
	}
	for _, pkg := range r.PackageNames {
		dest := filepath.Join(root, pkg)
		if _, err := os.Stat(filepath.Join(dest, "package.yaml")); err == nil {
			continue
		}
		ref := buildOCIRef(s.OCI, pkg)
		if err := r.pull(ref, dest); err != nil {
			return "", fmt.Errorf("source %q: pull %s: %w", s.Name, ref, err)
		}
	}
	return root, nil
}

func (r *OCIResolver) pull(ref, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("oras", "pull", ref, "--output", dest)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("oras pull failed: %w", err)
	}
	return nil
}

func buildOCIRef(o *v1.PackageSourceOCI, pkg string) string {
	registry := strings.TrimRight(o.Registry, "/")
	if o.Digest != "" {
		return fmt.Sprintf("%s/%s@%s", registry, pkg, o.Digest)
	}
	return fmt.Sprintf("%s/%s:%s", registry, pkg, o.Tag)
}

func encodeRef(o *v1.PackageSourceOCI) string {
	parts := []string{strings.ReplaceAll(o.Registry, "/", "_")}
	if o.Digest != "" {
		parts = append(parts, "digest", strings.ReplaceAll(o.Digest, ":", "-"))
	} else {
		parts = append(parts, "tag", url.PathEscape(o.Tag))
	}
	return strings.Join(parts, "_")
}
