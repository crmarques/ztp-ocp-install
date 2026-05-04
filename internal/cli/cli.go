package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/embedded"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
	"github.com/crmarques/ztp-ocp-install-lab/internal/render"
)

// ansibleBundleDirName is the directory under --state-dir where the embedded
// Ansible bundle is materialised on render/apply/destroy.
const ansibleBundleDirName = "ansible-bundle"

// Run wires Args + I/O streams onto a fresh cobra tree and maps any returned
// error to a process exit code. Tests drive the CLI through this function
// with bytes.Buffer streams.
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

// exitError carries a process exit code (and optional message) up through
// cobra's Execute path. silent suppresses stderr printing for commands that
// already emitted their own report (for example diff drift).
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

// ---------- Simple subcommands -------------------------------------------------

func newValidateCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		checkHost    bool
		secretsDir   string
		hostStateDir string
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Schema, defaults, and cross-reference validation; optionally probe local host tooling",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&checkHost, "check-host", false, "after schema validation, probe local tooling, /dev/kvm, and secrets material required to apply this state")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing secret material referenced by Environment and cluster install specs (used with --check-host)")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory (used with --check-host)")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Validation")
		fmt.Fprintf(stdout, "validated %d Environment, %d InfrastructureProvider, %d ClusterInfrastructure, %d OCPCluster object(s)\n",
			len(state.Environments), len(state.InfrastructureProviders), len(state.ClusterInfrastructures), len(state.OCPClusters))
		if !checkHost {
			return nil
		}
		return runHostCheck(stdout, stderr, state, secretsDir, hostStateDir)
	}
	return cmd
}

func runHostCheck(stdout io.Writer, stderr io.Writer, state v1alpha1.State, secretsDir string, hostStateDir string) error {
	checks := collectPreflightChecks(state, nil, true, secretsDir, hostStateDir, defaultPreflightDeps)
	printSubtitle(stdout, "host check:")
	failed := 0
	for _, c := range checks {
		if c.ok {
			printOK(stdout, c.name, c.detail)
			continue
		}
		printFail(stdout, c.name, c.detail)
		failed++
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "host check: %d required check(s) failed\n", failed)
		return silentExit(1)
	}
	fmt.Fprintf(stdout, "host check: all %d check(s) passed\n", len(checks))
	return nil
}

func newPlanCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Print object counts, installer assets, and component pins",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		pins := render.ComponentPins(state)
		installerAssets := render.InstallerAssets(cf.stateDir, state)
		printTitle(stdout, "Plan")
		fmt.Fprintf(stdout, "plan: %d Environment, %d InfrastructureProvider, %d ClusterInfrastructure, %d OCPCluster object(s)\n",
			len(state.Environments), len(state.InfrastructureProviders), len(state.ClusterInfrastructures), len(state.OCPClusters))
		fmt.Fprintf(stdout, "stateDir: %s\n", cf.stateDir)
		printSubtitle(stdout, "installer assets:")
		for index, asset := range installerAssets {
			installConfig, err := render.InstallerConfig(state, state.OCPClusters[index])
			if err != nil {
				return failErr(1, err)
			}
			agentConfig, err := render.AgentConfig(state, state.OCPClusters[index])
			if err != nil {
				return failErr(1, err)
			}
			fmt.Fprintf(stdout, "- %s (%s): platform=%s controlPlane=%d compute=%d hosts=%d\n",
				asset.ClusterName, asset.Method, installerPlatformName(installConfig),
				controlPlaneReplicas(installConfig), computeReplicas(installConfig), agentHostCount(agentConfig))
			fmt.Fprintf(stdout, "  files: %s, %s\n", asset.InstallConfigPath, asset.AgentConfigPath)
		}
		printSubtitle(stdout, "component pins:")
		for _, pin := range pins {
			fmt.Fprintf(stdout, "- %s=%s (%s)\n", pin.Name, pin.Version, pin.Source)
		}
		return nil
	}
	return cmd
}

func newRenderCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Write deterministic artifacts under --state-dir",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		result, err := loadAndRender(cf.files, cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Render")
		printRenderResult(stdout, result)
		fmt.Fprintf(stdout, "ansible bundle: %s\n", bundleDir)
		return nil
	}
	return cmd
}

func newStatusCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Read-only view of desired counts, rendered artifacts, and phases",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Status")
		fmt.Fprintf(stdout, "desired: %d Environment, %d InfrastructureProvider, %d ClusterInfrastructure, %d OCPCluster object(s)\n",
			len(state.Environments), len(state.InfrastructureProviders), len(state.ClusterInfrastructures), len(state.OCPClusters))
		fmt.Fprintf(stdout, "stateDir: %s\n", cf.stateDir)
		expected := expectedRenderedPaths(cf.stateDir, state)
		rendered, missing := splitExisting(expected)
		fmt.Fprintf(stdout, "rendered artifacts: %d present, %d missing\n", len(rendered), len(missing))
		for _, path := range missing {
			fmt.Fprintf(stdout, "- missing: %s\n", path)
		}
		printSubtitle(stdout, "phases:")
		for _, phase := range phases {
			fmt.Fprintf(stdout, "- %s: apply=%s destroy=%s\n", phase.Name, phase.ApplyPlaybook, phase.DestroyPlaybook)
		}
		return nil
	}
	return cmd
}

func newDiffCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Render to a temp dir and exit non-zero on drift",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		tempDir, err := os.MkdirTemp("", "gitups-diff-")
		if err != nil {
			return failErr(1, err)
		}
		defer os.RemoveAll(tempDir)
		if _, err := render.All(tempDir, state); err != nil {
			return failErr(1, err)
		}
		diffs, err := compareRenderedTrees(tempDir, cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Diff")
		if len(diffs) == 0 {
			printOK(stdout, "no drift: rendered output matches state-dir", "")
			return nil
		}
		printFail(stdout, fmt.Sprintf("drift detected: %d file(s)", len(diffs)), "")
		for _, d := range diffs {
			fmt.Fprintf(stdout, "- %s: %s\n", d.kind, d.path)
		}
		return silentExit(1)
	}
	return cmd
}

// ---------- Shared helpers -----------------------------------------------------

// extractBundle materialises the embedded Ansible tree into the state dir
// and returns its absolute path. The bundle is rewritten on every call so a
// stale extraction cannot leak old playbooks into a new run.
func extractBundle(stateDir string) (string, error) {
	bundleDir := filepath.Join(stateDir, ansibleBundleDirName)
	if err := embedded.ExtractAnsibleBundle(bundleDir); err != nil {
		return "", err
	}
	return filepath.Abs(bundleDir)
}

func loadAndRender(files []string, stateDir string) (render.Result, error) {
	state, err := infra.LoadNormalizeValidate(files)
	if err != nil {
		return render.Result{}, err
	}
	return render.All(stateDir, state)
}

func ensureApplySupported(state v1alpha1.State) error {
	providers := map[string]v1alpha1.InfrastructureProvider{}
	for _, provider := range state.InfrastructureProviders {
		providers[provider.Metadata.Name] = provider
	}
	for _, item := range state.ClusterInfrastructures {
		provider := providers[v1alpha1.FirstProviderRefName(item)]
		kind := v1alpha1.MachineFlavor(provider)
		if kind != v1alpha1.MachineFlavorLibvirt {
			return fmt.Errorf("%s: apply currently supports only provider kind %q, got %q", item.Metadata.Name, v1alpha1.MachineFlavorLibvirt, kind)
		}
	}
	return nil
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

func installerPlatformName(installConfig map[string]any) string {
	platform, _ := installConfig["platform"].(map[string]any)
	for _, name := range []string{"baremetal", "vsphere", "none"} {
		if _, ok := platform[name]; ok {
			return name
		}
	}
	return "unknown"
}

func controlPlaneReplicas(installConfig map[string]any) int {
	controlPlane, _ := installConfig["controlPlane"].(map[string]any)
	return intValue(controlPlane["replicas"])
}

func computeReplicas(installConfig map[string]any) int {
	compute, _ := installConfig["compute"].([]any)
	if len(compute) == 0 {
		return 0
	}
	worker, _ := compute[0].(map[string]any)
	return intValue(worker["replicas"])
}

func agentHostCount(agentConfig map[string]any) int {
	hosts, _ := agentConfig["hosts"].([]any)
	return len(hosts)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case uint64:
		return int(typed)
	default:
		return 0
	}
}

func expectedRenderedPaths(stateDir string, state v1alpha1.State) []string {
	paths := []string{
		filepath.Join(stateDir, "effective-state.yaml"),
		filepath.Join(stateDir, "gitups.lock.yaml"),
		filepath.Join(stateDir, "ansible", "inventory.yaml"),
		filepath.Join(stateDir, "ansible", "vars.yaml"),
	}
	for _, asset := range render.InstallerAssets(stateDir, state) {
		paths = append(paths, asset.InstallConfigPath, asset.AgentConfigPath)
	}
	return paths
}

func splitExisting(paths []string) (present, missing []string) {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			present = append(present, path)
		} else {
			missing = append(missing, path)
		}
	}
	return present, missing
}

type fileDiff struct {
	kind string // "missing", "extra", "changed"
	path string
}

func compareRenderedTrees(rendered, observed string) ([]fileDiff, error) {
	renderedFiles, err := walkRelative(rendered)
	if err != nil {
		return nil, err
	}
	observedFiles, err := walkRelative(observed)
	if err != nil {
		return nil, err
	}
	var diffs []fileDiff
	for rel := range renderedFiles {
		if _, ok := observedFiles[rel]; !ok {
			diffs = append(diffs, fileDiff{kind: "missing", path: rel})
			continue
		}
		same, err := sameFile(filepath.Join(rendered, rel), filepath.Join(observed, rel))
		if err != nil {
			return nil, err
		}
		if !same {
			diffs = append(diffs, fileDiff{kind: "changed", path: rel})
		}
	}
	for rel := range observedFiles {
		if _, ok := renderedFiles[rel]; !ok {
			diffs = append(diffs, fileDiff{kind: "extra", path: rel})
		}
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].path != diffs[j].path {
			return diffs[i].path < diffs[j].path
		}
		return diffs[i].kind < diffs[j].kind
	})
	return diffs, nil
}

func walkRelative(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// Skip ansible artifacts and the embedded-bundle extraction: both are
		// run-time side-effects, not user intent, so they must not register
		// as drift.
		if strings.HasPrefix(rel, filepath.Join("ansible", "artifacts")+string(os.PathSeparator)) {
			return nil
		}
		if rel == ansibleBundleDirName || strings.HasPrefix(rel, ansibleBundleDirName+string(os.PathSeparator)) {
			return nil
		}
		out[rel] = struct{}{}
		return nil
	})
	return out, err
}

func sameFile(a, b string) (bool, error) {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(dataA, dataB), nil
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
