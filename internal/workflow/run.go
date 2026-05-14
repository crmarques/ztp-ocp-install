// Package workflow exposes the provisioning pipeline as a pure Go API
// independent of the CLI. CLI handlers in internal/cli should be thin
// adapters that translate flags to Options and call into this package;
// they should not embed business logic. This is the seam where future
// hub/multi-cluster workflows will plug in without depending on Cobra.
//
// Design constraints:
//   - No package in internal/workflow imports internal/cli.
//   - All printing goes through io.Writer parameters; no fmt.Print or log.
//   - All exec goes through ansible.CommandRunner so tests can fake it.
//   - Options structs are flat: callers compute defaults and resolve paths
//     before calling in; workflow does not consult the environment.
package workflow

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/ansible"
	"github.com/crmarques/bootwright/internal/orchestrate/provisioning"
	"github.com/crmarques/bootwright/internal/provisioning/render"
)

// RunOptions describes one ansible-playbook invocation against rendered
// desired state. Callers pre-resolve every field (no defaults, no env
// lookup) so the function is deterministic from its inputs alone.
type RunOptions struct {
	State         v1alpha1.State
	StateDir      string
	SecretsDir    string
	HostStateDir  string
	Executable    string
	BundleDir     string
	Playbook      string
	Limit         string
	ExtraVarPairs []string
	// ArtifactsBaseName names the per-run subdirectory under the render
	// artifacts root, e.g. "preflight-infra" or "infra-destroy".
	ArtifactsBaseName string
	Check             bool
	AskBecomePass     bool
	DryRun            bool
	// Label is included in the dry-run echo line, e.g. "infra apply".
	Label string
}

// RunResult is what callers need to keep printing after the run completes
// (artifact paths, the rendered installer assets). The CLI prints these;
// workflow does not.
type RunResult struct {
	Render render.Result
	// Command is the argv the runner used, including ansible-playbook.
	Command []string
}

// Run renders artifacts and either prints the dry-run command or executes
// ansible-playbook through the provided runner. Errors from rendering or
// runspec construction are returned as-is. The runner's stdout/stderr are
// already wired by the caller. Accepts the ansible.Runner interface so
// tests can substitute a fake that records calls without exec'ing.
func Run(ctx context.Context, opts RunOptions, runner ansible.Runner, out io.Writer) (RunResult, error) {
	result, err := render.All(opts.StateDir, opts.SecretsDir, opts.State)
	if err != nil {
		return RunResult{}, err
	}
	spec, err := provisioning.NewRunSpec(provisioning.RunSpecConfig{
		Executable:    opts.Executable,
		BundleDir:     opts.BundleDir,
		StateDir:      opts.StateDir,
		SecretsDir:    opts.SecretsDir,
		HostStateDir:  opts.HostStateDir,
		InventoryPath: result.InventoryPath,
		VarsPath:      result.VarsPath,
		Playbook:      opts.Playbook,
		Limit:         opts.Limit,
		ExtraVarPairs: opts.ExtraVarPairs,
		ArtifactsDir:  filepath.Join(result.ArtifactsDir, opts.ArtifactsBaseName),
		Check:         opts.Check,
		AskBecomePass: opts.AskBecomePass,
	})
	if err != nil {
		return RunResult{Render: result}, err
	}
	command := runner.Command(spec)
	if opts.DryRun {
		label := opts.Label
		if label == "" {
			label = opts.Playbook
		}
		fmt.Fprintf(out, "dry-run ansible command [%s]: %s\n", label, ShellQuote(command))
		return RunResult{Render: result, Command: command}, nil
	}
	if err := runner.Run(ctx, spec); err != nil {
		return RunResult{Render: result, Command: command}, err
	}
	return RunResult{Render: result, Command: command}, nil
}

// RenderOnly executes the render half of a Run without producing a
// RunSpec or invoking ansible. Used by `bootwright render installer` and
// other read-only previews.
func RenderOnly(stateDir, secretsDir string, state v1alpha1.State) (render.Result, error) {
	return render.All(stateDir, secretsDir, state)
}

// ResolveInstaller renders effective install-config/agent-config copies
// with secret material inlined. Used by `bootwright render installer
// --resolve-secrets`.
func ResolveInstaller(stateDir, secretsDir string, state v1alpha1.State) (render.Result, error) {
	return render.ResolveInstaller(stateDir, secretsDir, state)
}

// ShellQuote returns a shell-safe representation of argv suitable for
// echoing in dry-run output. Identical to the helper that previously
// lived in internal/cli; moved here so workflow callers don't have to
// reach back into cli for it.
func ShellQuote(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if strings.ContainsAny(arg, " \t\n'\"$`\\") {
			quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}
