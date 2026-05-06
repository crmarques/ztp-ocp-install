package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/crmarques/ztp-ocp-install-lab/internal/ansible"
	"github.com/crmarques/ztp-ocp-install-lab/internal/embedded"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
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


func newPreflightCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		secretsDir   string
		hostStateDir string
		executable   string
		dryRun       bool
	)
	secretsDir = defaultSecretsDir()
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Run controller and provider readiness checks for desired state",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material")
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.Flags().StringVar(&executable, "ansible-playbook", resolveAnsiblePlaybook(), "ansible-playbook executable to run (defaults to the gitups-managed venv when present)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render artifacts and print the Ansible preflight command without executing it")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Preflight")
		if err := runHostCheck(stdout, stderr, state, secretsDir, hostStateDir); err != nil {
			return err
		}
		result, err := render.All(cf.stateDir, state)
		if err != nil {
			return failErr(1, err)
		}
		bundleDir, err := extractBundle(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		stateDirAbs, err := filepath.Abs(cf.stateDir)
		if err != nil {
			return failErr(1, err)
		}
		secretsDirAbs, err := filepath.Abs(secretsDir)
		if err != nil {
			return failErr(1, err)
		}
		hostStateDirAbs, err := filepath.Abs(hostStateDir)
		if err != nil {
			return failErr(1, err)
		}
		spec := ansible.RunSpec{
			Executable:        executable,
			AnsibleCfg:        filepath.Join(bundleDir, embedded.AnsibleCfgRelPath),
			RolesPath:         filepath.Join(bundleDir, embedded.RolesRelPath),
			CollectionsPath:   filepath.Join(bundleDir, embedded.CollectionsRelPath),
			FilterPluginsPath: filepath.Join(bundleDir, embedded.FilterPluginsRelPath),
			Inventory:         result.InventoryPath,
			Playbook:          filepath.Join(bundleDir, "playbooks/preflight.yml"),
			ExtraVars:         result.VarsPath,
			ExtraVarPairs: []string{
				"gitups_state_dir=" + stateDirAbs,
				"gitups_secrets_dir=" + secretsDirAbs,
				"gitups_host_state_dir=" + hostStateDirAbs,
			},
			ArtifactsDir: filepath.Join(result.ArtifactsDir, "preflight"),
		}
		runner := ansible.CommandRunner{Stdout: stdout, Stderr: stderr}
		command := runner.Command(spec)
		if dryRun {
			fmt.Fprintf(stdout, "dry-run ansible command [preflight]: %s\n", shellQuote(command))
			return nil
		}
		return runner.Run(c.Context(), spec)
	}
	return cmd
}

type planReport struct {
	Objects       map[string]int        `json:"objects"`
	StateDir      string                `json:"stateDir"`
	Generated     []string              `json:"generated"`
	Phases        []phaseReport         `json:"phases"`
	ComponentPins []render.ComponentPin `json:"componentPins"`
	Checks        []checkReport         `json:"checks,omitempty"`
}

type phaseReport struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	NeedsRoot   bool   `json:"needsRoot"`
}

type checkReport struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func newPlanCmd(stdout io.Writer) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview generated artifacts, phases, prerequisites, and external commands",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&out, "out", "text", "output format: text or json")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		if out != "text" && out != "json" {
			return failf(2, "--out must be text or json")
		}
		report := buildPlanReport(state, cf.stateDir)
		if out == "json" {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
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
		printSubtitle(stdout, "phases:")
		for _, phase := range workflowPhases("all") {
			marker := ""
			if phase.NeedsRoot {
				marker = " [root]"
			}
			fmt.Fprintf(stdout, "- %s%s: %s\n", phase.Name, marker, phase.Description)
		}
		printSubtitle(stdout, "preflight checks:")
		for _, check := range report.Checks {
			if check.OK {
				printOK(stdout, check.Name, check.Detail)
			} else {
				printFail(stdout, check.Name, check.Detail)
			}
		}
		return nil
	}
	return cmd
}

func buildPlanReport(state v1alpha1.State, stateDir string) planReport {
	expected := expectedRenderedPaths(stateDir, state)
	report := planReport{
		Objects: map[string]int{
			"environments":            len(state.Environments),
			"infrastructureProviders": len(state.InfrastructureProviders),
			"clusterInfrastructures":  len(state.ClusterInfrastructures),
			"ocpClusters":             len(state.OCPClusters),
		},
		StateDir:      stateDir,
		Generated:     expected,
		ComponentPins: render.ComponentPins(state),
	}
	for _, phase := range workflowPhases("all") {
		report.Phases = append(report.Phases, phaseReport{Name: phase.Name, Description: phase.Description, NeedsRoot: phase.NeedsRoot})
	}
	for _, check := range collectPreflightChecks(state, workflowPhases("all"), true, defaultSecretsDir(), defaultHostStateDir, defaultPreflightDeps) {
		report.Checks = append(report.Checks, checkReport{Name: check.name, OK: check.ok, Detail: check.detail})
	}
	return report
}

func newStatusCmd(stdout io.Writer) *cobra.Command {
	var (
		diff  bool
		watch bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Read-only view of desired counts, rendered artifacts, phases, and drift",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&diff, "diff", false, "compare rendered desired output with --state-dir")
	cmd.Flags().BoolVar(&watch, "watch", false, "reserved for a future watch loop; currently performs one status read")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Status")
		if watch {
			fmt.Fprintln(stdout, "watch: one-shot status; continuous watch is not implemented yet")
		}
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
		for _, phase := range workflowPhases("all") {
			fmt.Fprintf(stdout, "- %s: apply=%s destroy=%s\n", phase.Name, phase.ApplyPlaybook, phase.DestroyPlaybook)
		}
		if !diff {
			return nil
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
		printSubtitle(stdout, "diff:")
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

func extractBundle(stateDir string) (string, error) {
	bundleDir := filepath.Join(stateDir, ansibleBundleDirName)
	if err := embedded.ExtractAnsibleBundle(bundleDir); err != nil {
		return "", err
	}
	return filepath.Abs(bundleDir)
}

var applySupportedMachineFlavors = map[string]bool{
	v1alpha1.MachineFlavorLibvirt: true,
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
	kind string
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
