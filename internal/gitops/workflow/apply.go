package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/catalog"
	"github.com/crmarques/bootwright/internal/gitops/cluster"
)

type ApplyOptions struct {
	DryRun      bool
	WaitCRDs    bool
	WaitTimeout time.Duration
	Out         io.Writer
}

func ApplyFullTree(ctx context.Context, fp *v1.GitOpsPackageSet, ws Workspace, kc *cluster.KubeClient, opts ApplyOptions) error {
	out := writerOrDiscard(opts.Out)
	skip := serviceResourcesRepoSet(fp)
	seen := map[string]bool{}
	var repoOrder []string
	for i := range fp.Spec.Resolved.Packages {
		repo := fp.Spec.Resolved.Packages[i].RenderedPaths.Repo
		if skip[repo] {
			continue
		}
		if !seen[repo] {
			seen[repo] = true
			repoOrder = append(repoOrder, repo)
		}
	}
	fmt.Fprintf(out, "bootwright: applying %d repo(s) via %s (dry-run=%v, mode=full)\n",
		len(repoOrder), kc.Binary(), opts.DryRun)
	for _, repo := range repoOrder {
		repoDir := filepath.Join(ws.RenderRoot, repo)
		if _, err := os.Stat(repoDir); err != nil {
			return fmt.Errorf("%s not rendered (render with `bootwright render gitops %s` first): %w", repo, ws.Name, err)
		}
		if err := ApplyUnitDir(ctx, kc, repoDir, opts.DryRun, out); err != nil {
			return err
		}
		if opts.WaitCRDs && !opts.DryRun {
			subs := cluster.SubscriptionsForRepo(fp.Spec.Resolved.Packages, repo)
			if len(subs) > 0 {
				fmt.Fprintf(out, "bootwright: waiting on %d subscription(s) from %s before next repo\n", len(subs), repo)
				if err := cluster.WaitForSubscriptions(ctx, kc, subs, cluster.WaitOptions{Timeout: opts.WaitTimeout, Out: out}); err != nil {
					return fmt.Errorf("wait after %s: %w", repo, err)
				}
			}
		}
	}
	fmt.Fprintf(out, "bootwright: apply complete\n")
	return nil
}

func ApplyBootstrapOnly(ctx context.Context, fp *v1.GitOpsPackageSet, prov *v1.GitOpsPackageSet, cat *catalog.Catalog, ws Workspace, kc *cluster.KubeClient, opts ApplyOptions) error {
	out := writerOrDiscard(opts.Out)
	planned := BootstrapSubset(fp)
	if len(planned) == 0 {
		return fmt.Errorf("bootstrap subset is empty; nothing to apply")
	}
	SortPlan(planned)
	srcCLI, srcBinary, err := srcCLIForPlan(cat, prov, planned)
	if err != nil {
		return err
	}
	if srcBinary != "" {
		if _, err := exec.LookPath(srcBinary); err != nil {
			return fmt.Errorf("required SRC binary %q not found in PATH (declared in %s spec.cli)", srcBinary, srcCLI.ownerName)
		}
	}

	fmt.Fprintf(out, "bootwright: bootstrap-only mode; %d unit(s) to apply via %s (dry-run=%v)\n",
		len(planned), kc.Binary(), opts.DryRun)
	WriteBootstrapPlan(out, fp, planned)
	if warnings := CompatibilityWarnings(ctx, cat, planned, kc); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(out, "bootwright: compatibility warning — %s\n", w)
		}
	}

	runner := cluster.DefaultCLIRunner{}
	currentWave := -1
	var waveReady []readinessTarget
	for _, rp := range planned {
		if rp.ApplyWave != currentWave {
			if !opts.DryRun && len(waveReady) > 0 {
				if err := waitForReadinessBestEffort(ctx, kc, waveReady, opts.WaitTimeout, out); err != nil {
					return err
				}
			}
			waveReady = nil
			currentWave = rp.ApplyWave
		}
		unitDir := filepath.Join(ws.RenderRoot, rp.RenderedPaths.Repo, rp.RenderedPaths.Dir)
		if _, err := os.Stat(unitDir); err != nil {
			return fmt.Errorf("unit %s not rendered at %s (render with `bootwright render gitops %s` first): %w", rp.Instance, unitDir, ws.Name, err)
		}
		if rp.Controller != nil && rp.Controller.Kind == v1.RoleSRC {
			intent := rp.Controller.Intent
			if intentSpec, ok := srcCLI.spec.Intents[intent]; ok {
				if err := ApplyUnitDir(ctx, kc, unitDir, opts.DryRun, out); err != nil {
					return err
				}
				if err := invokeSRCCliWithArgs(ctx, runner, srcCLI.spec.Binary, intentSpec.Args, unitDir, kc.KubeContext(), rp, out, opts.DryRun); err != nil {
					return err
				}
			} else {
				if err := invokeSRCCli(ctx, runner, srcCLI.spec, unitDir, kc.KubeContext(), rp, out, opts.DryRun); err != nil {
					return err
				}
			}
		} else {
			if err := ApplyUnitDir(ctx, kc, unitDir, opts.DryRun, out); err != nil {
				return err
			}
		}
		waveReady = append(waveReady, readinessTargetsFor(rp, cat)...)
		if opts.WaitCRDs && !opts.DryRun && rp.Renderer == "olm" {
			ns, _ := rp.ResolvedValues["namespace"].(string)
			if ns != "" {
				sub := []cluster.SubscriptionRef{{Namespace: ns, Name: rp.Instance}}
				fmt.Fprintf(out, "bootwright: waiting on subscription %s/%s\n", ns, rp.Instance)
				if err := cluster.WaitForSubscriptions(ctx, kc, sub, cluster.WaitOptions{Timeout: opts.WaitTimeout, Out: out}); err != nil {
					return fmt.Errorf("wait after %s: %w", rp.Instance, err)
				}
			}
		}
	}
	if !opts.DryRun && len(waveReady) > 0 {
		if err := waitForReadinessBestEffort(ctx, kc, waveReady, opts.WaitTimeout, out); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "bootwright: bootstrap complete; handoff to in-cluster KRC/SRC.\n")
	return nil
}

func ApplyUnitDir(ctx context.Context, kc *cluster.KubeClient, dir string, dryRun bool, out io.Writer) error {
	out = writerOrDiscard(out)
	run := func(label string) error {
		fmt.Fprintf(out, "bootwright: apply [%s]\n", label)
		return kc.ApplyKustomize(ctx, dir, dryRun, out)
	}
	if err := run("pass 1"); err != nil {
		if dryRun {
			return fmt.Errorf("apply -k %s: %w", dir, err)
		}
		fmt.Fprintf(out, "bootwright: pass 1 reported errors; waiting for CRD establishment before retry\n")
		if waitErr := WaitForCRDsEstablished(ctx, kc, out); waitErr != nil {
			fmt.Fprintf(out, "bootwright: CRD establishment wait did not complete cleanly: %v\n", waitErr)
		}
		if err2 := run("pass 2"); err2 != nil {
			return fmt.Errorf("apply -k %s (both passes failed): %w", dir, err2)
		}
	}
	return nil
}

func NewKubeClientFromPackageSet(prov *v1.GitOpsPackageSet, cat *catalog.Catalog, toContext string) (*cluster.KubeClient, error) {
	if prov.Spec.Controllers == nil || prov.Spec.Controllers.KubernetesResources == nil {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources is required — the KRC declares the cluster binary bootwright uses")
	}
	a := prov.Spec.Controllers.KubernetesResources
	for _, r := range prov.Spec.Repositories {
		if r.Type != v1.RepoTypeKubernetesResources || r.RepoRef != nil || r.Name != a.Repo {
			continue
		}
		for _, pr := range r.Packages {
			entry, ok := cat.Lookup(pr.Template)
			if !ok {
				continue
			}
			instance := pr.Instance
			if instance == "" {
				parts := strings.Split(pr.Template, "/")
				instance = parts[len(parts)-1]
			}
			if instance != a.Instance {
				continue
			}
			if entry.Def.Spec.CLI == nil || entry.Def.Spec.CLI.Binary == "" {
				return nil, fmt.Errorf("KRC package %q has no spec.cli declared — bootwright needs it to know what binary to run for apply/wait",
					entry.Def.Metadata.Name)
			}
			return cluster.NewKubeClient(entry.Def.Spec.CLI, entry.Def.Metadata.Name, toContext, cluster.DefaultCLIRunner{})
		}
	}
	return nil, fmt.Errorf("KRC instance %q not found in repo %q", a.Instance, a.Repo)
}

func WaitForCRDsEstablished(ctx context.Context, kc *cluster.KubeClient, out io.Writer) error {
	if !crdsExist(ctx, kc) {
		fmt.Fprintf(writerOrDiscard(out), "bootwright: no CRDs yet on %s; skipping establishment wait\n", kc.KubeContext())
		return nil
	}
	return kc.WaitCRDsEstablished(ctx, 60*time.Second, writerOrDiscard(out))
}

func serviceResourcesRepoSet(fp *v1.GitOpsPackageSet) map[string]bool {
	out := map[string]bool{}
	for _, r := range fp.Spec.Resolved.Repositories {
		if r.Type == v1.RepoTypeServiceResources {
			out[r.Name] = true
		}
	}
	return out
}

type readinessTarget struct {
	Kind, Namespace, Name, Condition string
}

func readinessTargetsFor(rp *v1.ResolvedPackage, cat *catalog.Catalog) []readinessTarget {
	entry, ok := cat.Lookup(rp.Template)
	if !ok {
		return nil
	}
	var checks []v1.ReadinessCheck
	if rp.UnitType == v1.UnitTypeInstall {
		checks = append(checks, entry.Def.Spec.Readiness...)
	}
	if u, ok := entry.LookupDomainUnit(rp.Domain, rp.ResourceTemplate); ok {
		checks = append(checks, u.Descriptor.Readiness...)
	} else if u, ok := entry.LookupDomainUnit(v1.DomainInstall, rp.InstallMethod); ok {
		checks = append(checks, u.Descriptor.Readiness...)
	}
	out := make([]readinessTarget, 0, len(checks))
	for _, c := range checks {
		if c.Kind == "" || c.Name == "" || c.Condition == "" {
			continue
		}
		out = append(out, readinessTarget{Kind: c.Kind, Namespace: c.Namespace, Name: c.Name, Condition: c.Condition})
	}
	return out
}

func waitForReadinessBestEffort(ctx context.Context, kc *cluster.KubeClient, targets []readinessTarget, timeout time.Duration, out io.Writer) error {
	out = writerOrDiscard(out)
	seen := map[readinessTarget]bool{}
	perTarget := timeout
	if perTarget > 2*time.Minute || perTarget <= 0 {
		perTarget = 2 * time.Minute
	}
	for _, t := range targets {
		if seen[t] {
			continue
		}
		seen[t] = true
		fmt.Fprintf(out, "bootwright: wave gate — %s/%s/%s condition=%s (best-effort, %s)\n", t.Kind, t.Namespace, t.Name, t.Condition, perTarget)
		if err := kc.WaitCondition(ctx, t.Namespace, t.Kind, t.Name, t.Condition, perTarget, out); err != nil {
			fmt.Fprintf(out, "bootwright: wave gate skipped %s/%s/%s — %v (continuing; dependsOn ordering is still authoritative)\n", t.Kind, t.Namespace, t.Name, err)
		}
	}
	return nil
}

func invokeSRCCli(ctx context.Context, runner cluster.CLIRunner, spec *v1.ControllerCLI, unitDir, toContext string, rp *v1.ResolvedPackage, out io.Writer, dryRun bool) error {
	return invokeSRCCliWithArgs(ctx, runner, spec.Binary, spec.Args, unitDir, toContext, rp, out, dryRun)
}

func invokeSRCCliWithArgs(ctx context.Context, runner cluster.CLIRunner, binary string, argsTmpl []string, unitDir, toContext string, rp *v1.ResolvedPackage, out io.Writer, dryRun bool) error {
	out = writerOrDiscard(out)
	if dryRun {
		fmt.Fprintf(out, "bootwright: [dry-run] %s (skipped: SRC CLI has no uniform --dry-run contract) [%s]\n", binary, unitDir)
		return nil
	}
	ns, _ := rp.ResolvedValues["namespace"].(string)
	ctxFields := cluster.CLIContext{
		KubeContext:  toContext,
		ManifestPath: unitDir,
		Namespace:    ns,
	}
	args, err := cluster.RenderCLIArgs(&v1.ControllerCLI{Binary: binary, Args: argsTmpl}, ctxFields)
	if err != nil {
		return fmt.Errorf("unit %s: %w", rp.Instance, err)
	}
	fmt.Fprintf(out, "bootwright: %s %s\n", binary, strings.Join(args, " "))
	if err := runner.Run(ctx, binary, args, out, out); err != nil {
		return fmt.Errorf("unit %s: %s %s: %w", rp.Instance, binary, strings.Join(args, " "), err)
	}
	return nil
}

func CompatibilityWarnings(ctx context.Context, cat *catalog.Catalog, planned []*v1.ResolvedPackage, kc *cluster.KubeClient) []string {
	serverVer := kubeServerMinor(ctx, kc)
	if serverVer == "" {
		return nil
	}
	seenPkg := map[string]bool{}
	var out []string
	for _, rp := range planned {
		entry, ok := cat.Lookup(rp.Template)
		if !ok {
			continue
		}
		name := entry.Def.Metadata.Name
		if seenPkg[name] {
			continue
		}
		seenPkg[name] = true
		c := entry.Def.Spec.Compatibility
		if c == nil || len(c.Kubernetes) == 0 {
			continue
		}
		if !K8sVersionSatisfies(serverVer, c.Kubernetes) {
			out = append(out, fmt.Sprintf("package %q declares compatibility %v; cluster reports %s",
				name, c.Kubernetes, serverVer))
		}
	}
	return out
}

func kubeServerMinor(ctx context.Context, kc *cluster.KubeClient) string {
	body, err := kc.ServerVersion(ctx)
	if err != nil {
		return ""
	}
	return cluster.ParseServerMinor(body)
}

func crdsExist(ctx context.Context, kc *cluster.KubeClient) bool {
	body, err := kc.ListCRDs(ctx)
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(body))) > 0
}

func writerOrDiscard(out io.Writer) io.Writer {
	if out == nil {
		return io.Discard
	}
	return out
}

type srcCLIBundle struct {
	spec      *v1.ControllerCLI
	ownerName string
}

func srcCLIForPlan(cat *catalog.Catalog, prov *v1.GitOpsPackageSet, plan []*v1.ResolvedPackage) (srcCLIBundle, string, error) {
	hasSRCOwned := false
	for _, rp := range plan {
		if rp.Controller != nil && rp.Controller.Kind == v1.RoleSRC {
			hasSRCOwned = true
			break
		}
	}
	if !hasSRCOwned {
		return srcCLIBundle{}, "", nil
	}
	if prov.Spec.Controllers == nil || prov.Spec.Controllers.ServiceResources == nil {
		return srcCLIBundle{}, "", fmt.Errorf("SRC-owned units present but spec.controllers.serviceResources is missing")
	}
	a := prov.Spec.Controllers.ServiceResources
	for _, r := range prov.Spec.Repositories {
		if r.Type != v1.RepoTypeKubernetesResources || r.RepoRef != nil || r.Name != a.Repo {
			continue
		}
		for _, pr := range r.Packages {
			entry, ok := cat.Lookup(pr.Template)
			if !ok {
				continue
			}
			instance := pr.Instance
			if instance == "" {
				parts := strings.Split(pr.Template, "/")
				instance = parts[len(parts)-1]
			}
			if instance != a.Instance {
				continue
			}
			if entry.Def.Spec.CLI == nil || entry.Def.Spec.CLI.Binary == "" {
				return srcCLIBundle{}, "", fmt.Errorf("SRC package %q has no spec.cli declared", entry.Def.Metadata.Name)
			}
			return srcCLIBundle{spec: entry.Def.Spec.CLI, ownerName: entry.Def.Metadata.Name}, entry.Def.Spec.CLI.Binary, nil
		}
	}
	return srcCLIBundle{}, "", fmt.Errorf("SRC instance %q not found in repo %q", a.Instance, a.Repo)
}

func SortPlan(planned []*v1.ResolvedPackage) {
	sort.SliceStable(planned, func(i, j int) bool {
		if planned[i].ApplyWave != planned[j].ApplyWave {
			return planned[i].ApplyWave < planned[j].ApplyWave
		}
		return planned[i].Instance < planned[j].Instance
	})
}
