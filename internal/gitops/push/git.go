package push

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout string, err error)
}

type DefaultGitRunner struct {
	Stderr io.Writer
}

func (r DefaultGitRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	c := exec.CommandContext(ctx, "git", full...)
	var out bytes.Buffer
	c.Stdout = &out
	if r.Stderr != nil {
		c.Stderr = r.Stderr
	} else {
		c.Stderr = os.Stderr
	}
	if err := c.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w", strings.Join(full, " "), err)
	}
	return out.String(), nil
}

func EnsureRepo(ctx context.Context, git GitRunner, dir, branch string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	if _, err := git.Run(ctx, dir, "init", "--initial-branch="+branch); err != nil {
		return fmt.Errorf("git init %s: %w", dir, err)
	}
	return nil
}

func SetRemote(ctx context.Context, git GitRunner, dir, url string) error {
	if _, err := git.Run(ctx, dir, "remote", "set-url", "origin", url); err != nil {
		if _, addErr := git.Run(ctx, dir, "remote", "add", "origin", url); addErr != nil {
			return fmt.Errorf("set remote: %w / %w", err, addErr)
		}
	}
	return nil
}

func StageAndCommit(ctx context.Context, git GitRunner, dir, message string) (bool, error) {
	if _, err := git.Run(ctx, dir, "add", "-A"); err != nil {
		return false, err
	}
	// `status --porcelain` (not `diff --cached`) avoids HEAD-existence pitfalls on a fresh init.
	out, err := git.Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(out) == "" {
		return false, nil
	}
	if _, err := git.Run(ctx, dir, "commit", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}

func GitPush(ctx context.Context, git GitRunner, dir, branch string, force, dryRun bool) error {
	args := []string{"push", "-u", "origin", branch}
	if force {
		args = append(args, "--force")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if _, err := git.Run(ctx, dir, args...); err != nil {
		return err
	}
	return nil
}
