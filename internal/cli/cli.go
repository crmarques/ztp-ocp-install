package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/embedded"
	"github.com/crmarques/ztp-ocp-install-lab/internal/render"
)

const ansibleBundleDirName = "ansible-bundle"

func Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	root := newRootCmd(stdin, stdout, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if !ee.silent && ee.err != nil {
			fmt.Fprintln(stderr, ee.err)
		}
		return ee.code
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type exitError struct {
	code   int
	err    error
	silent bool
}

func (e *exitError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

func failErr(code int, err error) *exitError { return &exitError{code: code, err: err} }
func failf(code int, format string, a ...any) *exitError {
	return &exitError{code: code, err: fmt.Errorf(format, a...)}
}
func silentExit(code int) *exitError { return &exitError{code: code, silent: true} }

func extractBundle(stateDir string) (string, error) {
	bundleDir := filepath.Join(stateDir, ansibleBundleDirName)
	if err := embedded.ExtractAnsibleBundle(bundleDir); err != nil {
		return "", err
	}
	return filepath.Abs(bundleDir)
}

var applySupportedMachineFlavors = map[string]bool{
	v1alpha1.MachineFlavorBareMetal: true,
	v1alpha1.MachineFlavorLibvirt:   true,
}

func ensureApplySupported(state v1alpha1.State) error {
	providers := map[string]v1alpha1.InfrastructureProvider{}
	for _, provider := range state.InfrastructureProviders {
		providers[provider.Metadata.Name] = provider
	}
	for _, item := range state.ClusterInfrastructures {
		closure, errs := v1alpha1.BuildProviderClosure(item, providers)
		if len(errs) > 0 {
			return fmt.Errorf("%s: %s", item.Metadata.Name, strings.Join(errs, "; "))
		}
		kind := closure.MachineFlavor()
		if !applySupportedMachineFlavors[kind] {
			return fmt.Errorf("%s: apply does not yet support provider kind %q (supported: %s)", item.Metadata.Name, kind, supportedMachineFlavorList())
		}
	}
	return nil
}

func supportedMachineFlavorList() string {
	names := make([]string, 0, len(applySupportedMachineFlavors))
	for k, ok := range applySupportedMachineFlavors {
		if ok {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func printRenderResult(stdout io.Writer, result render.Result) {
	printSubtitle(stdout, "rendered:")
	fmt.Fprintf(stdout, "- %s\n", result.EffectiveStatePath)
	fmt.Fprintf(stdout, "- %s\n", result.LockPath)
	fmt.Fprintf(stdout, "- %s\n", result.InventoryPath)
	fmt.Fprintf(stdout, "- %s\n", result.VarsPath)
	for _, asset := range result.InstallerAssets {
		fmt.Fprintf(stdout, "- %s\n", asset.InstallConfigPath)
		fmt.Fprintf(stdout, "- %s\n", asset.AgentConfigPath)
	}
}

func shellQuote(args []string) string {
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
