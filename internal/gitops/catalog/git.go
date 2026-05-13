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
	"github.com/crmarques/gitups/internal/gitops/safepath"
)

// GitResolver materializes a git package source by shallow-cloning at the
// requested ref. Refs must be tags or commit SHAs; branches are rejected.
type GitResolver struct {
	CacheDir string
	Stdout   io.Writer
	Stderr   io.Writer
}

var commitSHARe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

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
	if err := verifyImmutableRef(cloneDir, s.Git.Ref); err != nil {
		return "", fmt.Errorf("source %q: %w", s.Name, err)
	}
	pkgPath := s.Git.Path
	if pkgPath == "" {
		pkgPath = "packages"
	}
	cleanPath, err := safepath.Relative("git.path", pkgPath)
	if err != nil {
		return "", fmt.Errorf("source %q: %w", s.Name, err)
	}
	root := filepath.Join(cloneDir, cleanPath)
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
	// --branch refuses raw SHAs; retry with full clone + checkout.
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	clone := exec.Command("git", "clone", "--quiet", g.URL, dest)
	clone.Stdout = r.Stdout
	clone.Stderr = r.Stderr
	if err := clone.Run(); err != nil {
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

func rejectBranchRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref is required")
	}
	if commitSHARe.MatchString(ref) {
		return nil
	}
	if ref == "HEAD" || strings.HasPrefix(ref, "refs/heads/") {
		return fmt.Errorf("ref %q must be a tag or commit SHA; branches are rejected", ref)
	}
	switch ref {
	case "main", "master", "develop", "development", "trunk":
		return fmt.Errorf("ref %q must be a tag or commit SHA; branches are rejected", ref)
	}
	if strings.HasPrefix(ref, "v") || strings.Contains(ref, "/") || strings.HasPrefix(ref, "refs/tags/") {
		return nil
	}
	return fmt.Errorf("ref %q must be a tag or commit SHA; branches are rejected", ref)
}

func verifyImmutableRef(repoDir, ref string) error {
	if commitSHARe.MatchString(ref) {
		return nil
	}
	tagRef := "refs/tags/" + strings.TrimPrefix(ref, "refs/tags/")
	if exec.Command("git", "-C", repoDir, "show-ref", "--verify", "--quiet", tagRef).Run() == nil {
		return nil
	}
	branchRef := "refs/remotes/origin/" + strings.TrimPrefix(ref, "refs/heads/")
	if exec.Command("git", "-C", repoDir, "show-ref", "--verify", "--quiet", branchRef).Run() == nil {
		return fmt.Errorf("ref %q resolves to a branch; use a tag or commit SHA", ref)
	}
	return fmt.Errorf("ref %q must resolve to a local tag or commit SHA", ref)
}

func encodeGitURL(u string) string {
	enc := strings.ReplaceAll(u, "://", "_")
	enc = strings.ReplaceAll(enc, "/", "_")
	enc = strings.ReplaceAll(enc, ":", "_")
	return enc
}
