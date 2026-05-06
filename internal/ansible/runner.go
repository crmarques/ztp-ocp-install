package ansible

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
)

const OutputLogName = "ansible-output.log"

type RunSpec struct {
	Executable        string
	AnsibleCfg        string
	RolesPath         string
	CollectionsPath   string
	FilterPluginsPath string
	Inventory         string
	Playbook          string
	ExtraVars         string
	ExtraVarPairs     []string
	ArtifactsDir      string
	Check             bool
	AskBecomePass     bool
}

type Runner interface {
	Run(context.Context, RunSpec) error
	Command(RunSpec) []string
}

type CommandRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r CommandRunner) Command(spec RunSpec) []string {
	executable := spec.Executable
	if executable == "" {
		executable = "ansible-playbook"
	}
	args := []string{
		executable,
		"-i", spec.Inventory,
		spec.Playbook,
		"-e", "@" + spec.ExtraVars,
	}
	for _, pair := range spec.ExtraVarPairs {
		args = append(args, "-e", pair)
	}
	if spec.Check {
		args = append(args, "--check")
	}
	if spec.AskBecomePass {
		args = append(args, "--ask-become-pass")
	}
	return args
}

func (r CommandRunner) Run(ctx context.Context, spec RunSpec) error {
	if err := os.MkdirAll(spec.ArtifactsDir, 0o700); err != nil {
		return fmt.Errorf("create Ansible artifacts directory: %w", err)
	}
	if err := os.Chmod(spec.ArtifactsDir, 0o700); err != nil {
		return fmt.Errorf("chmod Ansible artifacts directory: %w", err)
	}
	outputLogPath := filepath.Join(spec.ArtifactsDir, OutputLogName)
	outputLog, err := os.OpenFile(outputLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create Ansible output log: %w", err)
	}
	defer outputLog.Close()
	if err := os.Chmod(outputLogPath, 0o600); err != nil {
		return fmt.Errorf("chmod Ansible output log: %w", err)
	}
	lockedOutputLog := &lockedWriter{w: outputLog}

	command := r.Command(spec)
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout = teeWriter(r.Stdout, lockedOutputLog)
	cmd.Stderr = teeWriter(r.Stderr, lockedOutputLog)
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if spec.AnsibleCfg != "" {
		cmd.Env = append(cmd.Env, "ANSIBLE_CONFIG="+spec.AnsibleCfg)
	}
	if spec.RolesPath != "" {
		cmd.Env = append(cmd.Env, "ANSIBLE_ROLES_PATH="+filepath.Clean(spec.RolesPath))
	}
	if spec.CollectionsPath != "" {
		cmd.Env = append(cmd.Env, "ANSIBLE_COLLECTIONS_PATH="+filepath.Clean(spec.CollectionsPath))
	}
	if spec.FilterPluginsPath != "" {
		cmd.Env = append(cmd.Env, "ANSIBLE_FILTER_PLUGINS="+filepath.Clean(spec.FilterPluginsPath))
	}
	if spec.ArtifactsDir != "" {
		cmd.Env = append(cmd.Env, "GITUPS_ANSIBLE_ARTIFACTS="+filepath.Clean(spec.ArtifactsDir))
	}
	if extra := sudoUserSitePackages(); extra != "" {
		cmd.Env = appendPythonPath(cmd.Env, extra)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s (output log: %s): %w", command[0], outputLogPath, err)
	}
	return nil
}

func sudoUserSitePackages() string {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" || sudoUser == "root" {
		return ""
	}
	u, err := user.Lookup(sudoUser)
	if err != nil || u.HomeDir == "" {
		return ""
	}
	probe := exec.Command("python3", "-m", "site", "--user-site")
	probe.Env = append(os.Environ(), "HOME="+u.HomeDir)
	out, err := probe.Output()
	if err != nil {
		return ""
	}
	site := strings.TrimSpace(string(out))
	if site == "" {
		return ""
	}
	if info, err := os.Stat(site); err != nil || !info.IsDir() {
		return ""
	}
	return site
}

func appendPythonPath(env []string, extra string) []string {
	for index, entry := range env {
		if strings.HasPrefix(entry, "PYTHONPATH=") {
			existing := strings.TrimPrefix(entry, "PYTHONPATH=")
			env[index] = "PYTHONPATH=" + extra + string(os.PathListSeparator) + existing
			return env
		}
	}
	return append(env, "PYTHONPATH="+extra)
}

func teeWriter(primary io.Writer, log io.Writer) io.Writer {
	if primary == nil {
		return log
	}
	return io.MultiWriter(primary, log)
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
