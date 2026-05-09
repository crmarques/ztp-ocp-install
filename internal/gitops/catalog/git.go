package catalog

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

// GitResolver materializes a git package source by shallow-cloning the
// repository at the requested ref into a cache directory and returning the
// inner package path.
//
// Refs must be tags or commit SHAs; branch names are rejected so the source
// is reproducible. The cache key is (URL, Ref); subsequent resolves with the
// same cache key skip the clone if the working tree is already present.
type GitResolver struct {
	// CacheDir is the absolute path where clones live. Required.
	CacheDir string

	// Stdout/Stderr capture git output. nil disables forwarding.
	Stdout io.Writer
	Stderr io.Writer
}

var commitSHARe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// Resolve implements catalog.SourceResolver.
func (r *GitResolver) Resolve(s v1.PackageSource, _ string) (string, error) {
	if s.Git == nil {
		return "", fmt.Errorf("source %q: git sub-block is nil", s.Name)
	}
	if r.CacheDir == "" {
		return "", fmt.Errorf("source %q: git resolver CacheDir is required", s.Name)
	}
	if err := rejectBranchRef(s.Git.Ref); err != nil {
		return "", fmt.Errorf("source %q: %w", s.Name, err)
	}
	cloneDir := filepath.Join(r.CacheDir, "git", encodeGitURL(s.Git.URL), url.PathEscape(s.Git.Ref))
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		if err := r.clone(s.Git, cloneDir); err != nil {
			return "", fmt.Errorf("source %q: clone %s: %w", s.Name, s.Git.URL, err)
		}
	}
	pkgPath := s.Git.Path
	if pkgPath == "" {
		pkgPath = "packages"
	}
	root := filepath.Join(cloneDir, pkgPath)
	if _, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("source %q: %s missing in clone: %w", s.Name, pkgPath, err)
	}
	return root, nil
}

func (r *GitResolver) clone(g *v1.PackageSourceGit, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	args := []string{"clone", "--quiet", "--depth", "1", "--branch", g.Ref, g.URL, dest}
	cmd := exec.Command("git", args...)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Fallback: full fetch + checkout when --branch refuses (e.g. raw SHA).
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := exec.Command("git", "clone", "--quiet", g.URL, dest).Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	co := exec.Command("git", "-C", dest, "checkout", "--quiet", g.Ref)
	co.Stdout = r.Stdout
	co.Stderr = r.Stderr
	if err := co.Run(); err != nil {
		return fmt.Errorf("git checkout %s: %w", g.Ref, err)
	}
	return nil
}

// rejectBranchRef rules out anything that looks like a moving target.
// Acceptable: full or short commit SHAs, anything starting with a leading
// `v` or containing a `/` (assumed to be a tag like `pkg/<name>/v0.1.0`).
func rejectBranchRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref is required")
	}
	if commitSHARe.MatchString(ref) {
		return nil
	}
	if strings.HasPrefix(ref, "v") || strings.Contains(ref, "/") {
		return nil
	}
	return fmt.Errorf("ref %q must be a tag or commit SHA; branches are rejected", ref)
}

func encodeGitURL(u string) string {
	enc := strings.ReplaceAll(u, "://", "_")
	enc = strings.ReplaceAll(enc, "/", "_")
	enc = strings.ReplaceAll(enc, ":", "_")
	return enc
}
