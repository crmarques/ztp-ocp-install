package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/gitops/catalog"
	"github.com/crmarques/gitups/internal/gitops/cluster"
	"github.com/crmarques/gitups/internal/gitops/load"
	"github.com/crmarques/gitups/internal/gitops/placeholders"
	"github.com/crmarques/gitups/internal/gitops/push"
	"github.com/crmarques/gitups/internal/gitops/render"
	"github.com/crmarques/gitups/internal/gitops/resolve"
)

const defaultGitopsWorkspace = "./gitops-workspaces"

type workspace struct {
	Name               string
	Root               string
	PackageSet         string
	ExpandedPackageSet string
	RenderRoot         string
}

func newWorkspace(outputDir, name string) (workspace, error) {
	if name == "" {
		return workspace{}, fmt.Errorf("name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return workspace{}, fmt.Errorf("name %q must not contain path separators", name)
	}
	if outputDir == "" {
		outputDir = defaultGitopsWorkspace
	}
	root := filepath.Join(outputDir, name)
	return workspace{
		Name:               name,
		Root:               root,
		PackageSet:         filepath.Join(root, "gitops-package-set.yaml"),
		ExpandedPackageSet: filepath.Join(root, ".gitups", "expanded", "gitops-package-set.yaml"),
		RenderRoot:         filepath.Join(root, ".gitups", "render"),
	}, nil
}

func addOutputDirFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVarP(target, "output-dir", "d", defaultGitopsWorkspace,
		"workspace root; the environment lives at <output-dir>/<name>/")
}

func newGitopsInitCmd() *cobra.Command {
	var (
		outputDir string
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Scaffold an empty GitOpsPackageSet for <name>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.PackageSet); err == nil && !force {
				return fmt.Errorf("%s already exists (pass --force to overwrite)", ws.PackageSet)
			}
			if err := os.MkdirAll(ws.Root, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", ws.Root, err)
			}
			if err := os.WriteFile(ws.PackageSet, []byte(scaffoldGitOpsPackageSet(ws.Name)), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", ws.PackageSet, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(),
				"gitups: scaffolded %s\n  next: edit spec.sources and spec.repositories, then `gitups expand gitops %s`\n",
				ws.PackageSet, ws.Name)
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing gitops-package-set.yaml")
	return cmd
}

func newGitopsExpandCmd() *cobra.Command {
	var (
		outputDir string
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Expand GitOpsPackageSet <name> into spec.resolved",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.PackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups init gitops %s` first)", ws.PackageSet, ws.Name)
			}
			prov, extFrom, err := load.PackageSetResolved(ws.PackageSet)
			if err != nil {
				return err
			}
			if prov.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.PackageSet, prov.Metadata.Name, ws.Name)
			}
			if load.IsScaffold(prov) {
				return fmt.Errorf("%s is still the init scaffold; fill in spec.sources and spec.repositories before `gitups expand gitops %s`",
					ws.PackageSet, ws.Name)
			}
			if len(prov.Spec.Sources) == 0 {
				return fmt.Errorf("%s: spec.sources is empty", ws.PackageSet)
			}
			if len(prov.Spec.Repositories) == 0 {
				return fmt.Errorf("%s: spec.repositories is empty", ws.PackageSet)
			}
			baseDir := filepath.Dir(absPath(ws.PackageSet))
			registerGitopsSourceResolvers(prov.Spec.Sources, packageNamesByTemplate(prov))
			cat, err := catalog.Build(prov.Spec.Sources, baseDir)
			if err != nil {
				return err
			}
			var prior *v1.GitOpsPackageSet
			if !force {
				if _, statErr := os.Stat(ws.ExpandedPackageSet); statErr == nil {
					prior, err = load.ExpandedPackageSet(ws.ExpandedPackageSet)
					if err != nil {
						return fmt.Errorf("load prior package-set for idempotent expand: %w", err)
					}
				}
			}
			fp, err := resolve.Expand(prov, cat, resolve.Options{
				Prior:        prior,
				Force:        force,
				ExtendedFrom: extFrom,
			})
			if err != nil {
				return err
			}
			body, err := yaml.Marshal(fp)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(ws.ExpandedPackageSet), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(ws.ExpandedPackageSet), err)
			}
			if err := os.WriteFile(ws.ExpandedPackageSet, body, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "gitups: wrote %s\n", ws.ExpandedPackageSet)
			printPlaceholderSummary(cmd.ErrOrStderr(), fp)
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().BoolVar(&force, "force", false, "discard existing expanded GitOpsPackageSet and regenerate from scratch")
	return cmd
}

func newGitopsCheckCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Validate GitOpsPackageSet and expanded GitOpsPackageSet for <name>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			out := cmd.ErrOrStderr()
			if _, err := os.Stat(ws.PackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups init gitops %s`)", ws.PackageSet, ws.Name)
			}
			prov, extFrom, err := load.PackageSetResolved(ws.PackageSet)
			if err != nil {
				return fmt.Errorf("package set invalid: %w", err)
			}
			if prov.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.PackageSet, prov.Metadata.Name, ws.Name)
			}
			if load.IsScaffold(prov) {
				fmt.Fprintf(out, "gitups: %s is the init scaffold (empty sources/repositories) — fill it in, then re-run check\n", ws.PackageSet)
				return nil
			}
			if extFrom != nil {
				fmt.Fprintf(out, "gitups: %s extends %s\n", ws.PackageSet, extFrom.Source)
			}
			fmt.Fprintf(out, "gitups: %s ok (%d source(s), %d repositories)\n",
				ws.PackageSet, len(prov.Spec.Sources), len(prov.Spec.Repositories))

			baseDir := filepath.Dir(absPath(ws.PackageSet))
			registerGitopsSourceResolvers(prov.Spec.Sources, packageNamesByTemplate(prov))
			cat, err := catalog.Build(prov.Spec.Sources, baseDir)
			if err != nil {
				return fmt.Errorf("catalog: %w", err)
			}
			if _, err := resolve.Expand(prov, cat, resolve.Options{}); err != nil {
				return fmt.Errorf("dry expand: %w", err)
			}
			fmt.Fprintf(out, "gitups: dry expand ok\n")

			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				fmt.Fprintf(out, "gitups: %s not present — run `gitups expand gitops %s`\n",
					ws.ExpandedPackageSet, ws.Name)
				return nil
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return fmt.Errorf("package-set invalid: %w", err)
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			fmt.Fprintf(out, "gitups: %s ok (%d package(s))\n",
				ws.ExpandedPackageSet, len(fp.Spec.Resolved.Packages))
			printPlaceholderSummary(out, fp)
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	return cmd
}

func newGitopsRenderCmd() *cobra.Command {
	var (
		outputDir         string
		kubeContext       string
		allowPlaceholders bool
		prune             bool
		skipDetCheck      bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Render repo directories from GitOpsPackageSet <name>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)",
					ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			baseDir := ws.Root
			registerGitopsSourceResolvers(fp.Spec.Sources, packageNamesFromExpandedPackageSet(fp))
			cat, err := catalog.Build(fp.Spec.Sources, baseDir)
			if err != nil {
				return err
			}
			if err := ensureBinaries(); err != nil {
				return err
			}
			ctx := kubeContext
			if ctx == "" {
				ctx = currentKubectlContext()
			}
			opts := render.Options{
				OutputPath:             ws.RenderRoot,
				KubectlContext:         ctx,
				AllowPlaceholders:      allowPlaceholders,
				SuppressPackageSetCopy: true,
				PreserveExtras:         true,
				Prune:                  prune,
			}
			if err := render.Render(cmd.Context(), fp, cat, opts); err != nil {
				return err
			}
			if skipDetCheck {
				return nil
			}
			return verifyDeterminism(cmd.Context(), fp, cat, opts, ws, cmd.ErrOrStderr())
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringVar(&kubeContext, "context", "", "cluster context label stamped into the rendered overlay (advisory)")
	cmd.Flags().BoolVar(&allowPlaceholders, "allow-placeholders", false, "render even when placeholders remain")
	cmd.Flags().BoolVar(&prune, "prune", false, "remove top-level directories not produced by this render pass")
	cmd.Flags().BoolVar(&skipDetCheck, "skip-determinism-check", false, "skip the second render pass that verifies byte-identical output")
	cmd.SetContext(context.Background())
	return cmd
}

// re-renders into a scratch dir and diffs against the workspace to catch chart-side
// non-determinism (auto-generated TLS certs, random IDs, timestamps).
func verifyDeterminism(ctx context.Context, fp *v1.GitOpsPackageSet, cat *catalog.Catalog, first render.Options, ws workspace, out writer) error {
	scratchRoot, err := os.MkdirTemp("", "gitups-det-")
	if err != nil {
		return fmt.Errorf("create determinism scratch: %w", err)
	}
	defer os.RemoveAll(scratchRoot)
	scratchOut := filepath.Join(scratchRoot, ws.Name)
	second := first
	second.OutputPath = scratchOut
	second.PreserveExtras = false
	second.Prune = false
	if err := render.Render(ctx, fp, cat, second); err != nil {
		return fmt.Errorf("determinism re-render: %w", err)
	}
	drifts, err := diffWorkspace(ws.RenderRoot, scratchOut)
	if err != nil {
		return fmt.Errorf("determinism diff: %w", err)
	}
	// PreserveExtras runs only on the first pass, so extras/orphan-dirs are expected drift
	filtered := drifts[:0]
	for _, d := range drifts {
		if d.Kind == "extra" || d.Kind == "orphan-dir" {
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) == 0 {
		return nil
	}
	fmt.Fprintf(out, "gitups: determinism check failed — %d file(s) differ between two render passes:\n", len(filtered))
	for _, d := range filtered {
		fmt.Fprintf(out, "  %-12s %s\n", d.Kind, d.Path)
	}
	return fmt.Errorf("non-deterministic render; inspect chart values for timestamps, random IDs, or auto-generated secrets, or pass --skip-determinism-check to bypass")
}

func newGitopsPushCmd() *cobra.Command {
	var (
		outputDir     string
		provider      string
		baseURL       string
		ownerType     string
		token         string
		user          string
		branch        string
		commitMessage string
		visibility    string
		createMissing bool
		flatten       bool
		force         bool
		dryRun        bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name> --provider <p> --base-url <url>",
		Short: "Publish rendered repos to a git provider (github|gitlab|gitea)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)", ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			if _, err := exec.LookPath("git"); err != nil {
				return fmt.Errorf("required binary %q not found in PATH", "git")
			}

			parsed, err := push.ParseBaseURL(baseURL)
			if err != nil {
				return err
			}
			if commitMessage == "" {
				commitMessage = fmt.Sprintf("gitups: sync %s", ws.Name)
			}
			resolvedToken := resolvePushToken(token, provider)

			prov, err := push.NewProvider(provider, push.ProviderConfig{
				Base:      parsed,
				Token:     resolvedToken,
				OwnerType: ownerType,
			})
			if err != nil {
				return err
			}

			out := cmd.ErrOrStderr()
			repos := renderedRepoNames(fp)
			if len(repos) == 0 {
				return fmt.Errorf("no rendered repos referenced by %s", ws.ExpandedPackageSet)
			}

			_, err = push.Push(cmd.Context(), push.Config{
				WorkspaceRoot: ws.RenderRoot,
				RepoNames:     repos,
				Provider:      prov,
				Git:           push.DefaultGitRunner{Stderr: out},
				Out:           out,
			}, push.Options{
				Provider:      provider,
				BaseURL:       baseURL,
				OwnerType:     ownerType,
				Token:         resolvedToken,
				User:          user,
				Branch:        branch,
				CommitMessage: commitMessage,
				Visibility:    visibility,
				CreateMissing: createMissing,
				Flatten:       flatten,
				Force:         force,
				DryRun:        dryRun,
			}, parsed)
			return err
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringVar(&provider, "provider", "", "git provider: github|gitlab|gitea (required)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "HTTPS base URL including owner/group, e.g. https://github.com/myorg (required)")
	cmd.Flags().StringVar(&ownerType, "owner-type", "org", "GitHub/Gitea create-endpoint selector: org or user")
	cmd.Flags().StringVar(&token, "token", "", "override credentials for REST and push; else uses GITUPS_PUSH_TOKEN or provider env")
	cmd.Flags().StringVar(&user, "user", "", "basic-auth username injected into push URL when --token is set (default: provider-appropriate literal)")
	cmd.Flags().StringVar(&branch, "branch", "main", "branch to commit and push")
	cmd.Flags().StringVar(&commitMessage, "commit-message", "", "commit message when there are changes (default: \"gitups: sync <name>\")")
	cmd.Flags().StringVar(&visibility, "visibility", "private", "repo visibility on creation: private|public|internal")
	cmd.Flags().BoolVar(&createMissing, "create-missing", true, "create remote repo via provider API when absent")
	cmd.Flags().BoolVar(&flatten, "flatten", false, "replace '/' with '-' in rendered repo names for providers without subgroups")
	cmd.Flags().BoolVar(&force, "force", false, "force-push")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "do not create remote repos; pass --dry-run to git push")
	cmd.SetContext(context.Background())
	return cmd
}

func resolvePushToken(flag, provider string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv("GITUPS_PUSH_TOKEN"); v != "" {
		return v
	}
	switch strings.ToLower(provider) {
	case "github":
		return os.Getenv("GITHUB_TOKEN")
	case "gitlab":
		return os.Getenv("GITLAB_TOKEN")
	case "gitea":
		return os.Getenv("GITEA_TOKEN")
	}
	return ""
}

func renderedRepoNames(fp *v1.GitOpsPackageSet) []string {
	seen := map[string]bool{}
	var out []string
	for i := range fp.Spec.Resolved.Packages {
		r := fp.Spec.Resolved.Packages[i].RenderedPaths.Repo
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

func newGitopsApplyCmd() *cobra.Command {
	var (
		outputDir         string
		toContext         string
		dryRun            bool
		yes               bool
		allowPlaceholders bool
		waitCRDs          bool
		full              bool
		waitTimeout       time.Duration
	)
	cmd := &cobra.Command{
		Use:   "gitops <name> --to <cluster-context>",
		Short: "Bootstrap the target cluster via the KRC-declared CLI (+ SRC CLI for SRC-owned units), then hand off to the in-cluster KRC/SRC",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toContext == "" {
				return fmt.Errorf("--to <cluster-context> is required")
			}
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` and `gitups render gitops %s` first)",
					ws.ExpandedPackageSet, ws.Name, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			if !allowPlaceholders && len(fp.Spec.Resolved.Placeholders) > 0 {
				return fmt.Errorf("%d unfilled placeholder(s) in %s (re-run with --allow-placeholders to force)",
					len(fp.Spec.Resolved.Placeholders), ws.ExpandedPackageSet)
			}

			out := cmd.ErrOrStderr()

			provPath := filepath.Join(ws.Root, "gitops-package-set.yaml")
			prov, err := load.PackageSet(provPath)
			if err != nil {
				return fmt.Errorf("load package set %s: %w", provPath, err)
			}
			if prov.Spec.Controllers == nil || prov.Spec.Controllers.KubernetesResources == nil {
				return fmt.Errorf("apply requires spec.controllers.kubernetesResources in %s — the KRC declares the cluster binary gitups uses", provPath)
			}

			cat, err := buildGitOpsPackageSetCatalog(prov, ws)
			if err != nil {
				return err
			}
			kubeClient, err := newKubeClientFromGitOpsPackageSet(prov, cat, toContext)
			if err != nil {
				return err
			}
			if _, err := exec.LookPath(kubeClient.Binary()); err != nil {
				return fmt.Errorf("KRC %q: required binary %q (declared in spec.cli.binary) not found in PATH",
					kubeClient.KRCName(), kubeClient.Binary())
			}

			hasSRC := prov.Spec.Controllers.ServiceResources != nil

			if !dryRun && !yes {
				prompt := fmt.Sprintf("Apply %s to cluster context %q? [y/N] (default: no): ", ws.Name, toContext)
				if !confirm(cmd.InOrStdin(), out, prompt) {
					return errors.New("apply aborted")
				}
			}

			if full || !hasSRC {
				return applyFullTree(cmd, fp, ws, kubeClient, dryRun, waitCRDs, waitTimeout, out)
			}
			return applyBootstrapOnly(cmd, fp, prov, cat, ws, kubeClient, dryRun, waitCRDs, waitTimeout, out)
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringVar(&toContext, "to", "", "cluster context to apply into (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "invoke the KRC's apply-dry-run intent; no cluster state changes")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the apply confirmation prompt")
	cmd.Flags().BoolVar(&allowPlaceholders, "allow-placeholders", false, "apply even when placeholders remain in expanded GitOpsPackageSet")
	cmd.Flags().BoolVar(&waitCRDs, "wait-crds", false, "after each repo, wait for its OLM subscriptions to Succeed before the next repo")
	cmd.Flags().BoolVar(&full, "full", false, "apply the whole rendered tree even when spec.controllers declares a KRC/SRC (for SRC-less setups or disaster recovery)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "per-repo wait budget when --wait-crds is set")
	cmd.SetContext(context.Background())
	return cmd
}

// applies each rendered repo in topo order via the KRC-declared apply intent;
// service-resources repos (declarest payload skeletons) are skipped — not K8s manifest trees.
func applyFullTree(cmd *cobra.Command, fp *v1.GitOpsPackageSet, ws workspace, kc *cluster.KubeClient, dryRun, waitCRDs bool, waitTimeout time.Duration, out writer) error {
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
	fmt.Fprintf(out, "gitups: applying %d repo(s) via %s (dry-run=%v, mode=full)\n",
		len(repoOrder), kc.Binary(), dryRun)
	for _, repo := range repoOrder {
		repoDir := filepath.Join(ws.RenderRoot, repo)
		if _, err := os.Stat(repoDir); err != nil {
			return fmt.Errorf("%s not rendered (render with `gitups render gitops %s` first): %w", repo, ws.Name, err)
		}
		if err := applyUnitDir(cmd.Context(), kc, repoDir, dryRun, out); err != nil {
			return err
		}
		if waitCRDs && !dryRun {
			subs := cluster.SubscriptionsForRepo(fp.Spec.Resolved.Packages, repo)
			if len(subs) > 0 {
				fmt.Fprintf(out, "gitups: waiting on %d subscription(s) from %s before next repo\n", len(subs), repo)
				if err := cluster.WaitForSubscriptions(cmd.Context(), kc, subs, cluster.WaitOptions{Timeout: waitTimeout, Out: stdioWriter{w: out}}); err != nil {
					return fmt.Errorf("wait after %s: %w", repo, err)
				}
			}
		}
	}
	fmt.Fprintf(out, "gitups: apply complete\n")
	return nil
}

// applies the bootstrap subset only (installs, KRC/SRC self resources, controller-owned units);
// routing is per-unit because the subset spans repos and skips siblings; waves gate on readiness checks.
func applyBootstrapOnly(cmd *cobra.Command, fp *v1.GitOpsPackageSet, prov *v1.GitOpsPackageSet, cat *catalog.Catalog, ws workspace, kc *cluster.KubeClient, dryRun, waitCRDs bool, waitTimeout time.Duration, out writer) error {
	planned := bootstrapSubset(fp)
	if len(planned) == 0 {
		return fmt.Errorf("bootstrap subset is empty; nothing to apply")
	}
	sort.SliceStable(planned, func(i, j int) bool {
		if planned[i].ApplyWave != planned[j].ApplyWave {
			return planned[i].ApplyWave < planned[j].ApplyWave
		}
		return planned[i].Instance < planned[j].Instance
	})

	srcCLI, srcBinary, err := srcCLIForPlan(cat, prov, planned)
	if err != nil {
		return err
	}
	if srcBinary != "" {
		if _, err := exec.LookPath(srcBinary); err != nil {
			return fmt.Errorf("required SRC binary %q not found in PATH (declared in %s spec.cli)", srcBinary, srcCLI.ownerName)
		}
	}

	fmt.Fprintf(out, "gitups: bootstrap-only mode; %d unit(s) to apply via %s (dry-run=%v)\n",
		len(planned), kc.Binary(), dryRun)
	writeBootstrapPlan(out, fp, planned)
	if warnings := compatibilityWarnings(cmd.Context(), cat, planned, kc); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(out, "gitups: compatibility warning — %s\n", w)
		}
	}

	appliedRepos := map[string]bool{}
	runner := cluster.DefaultCLIRunner{}
	currentWave := -1
	var waveReady []readinessTarget
	for _, rp := range planned {
		if rp.ApplyWave != currentWave {
			if !dryRun && len(waveReady) > 0 {
				if err := waitForReadiness(cmd.Context(), kc, waveReady, waitTimeout, out); err != nil {
					return err
				}
			}
			waveReady = nil
			currentWave = rp.ApplyWave
		}
		unitDir := filepath.Join(ws.RenderRoot, rp.RenderedPaths.Repo, rp.RenderedPaths.Dir)
		if _, err := os.Stat(unitDir); err != nil {
			return fmt.Errorf("unit %s not rendered at %s (render with `gitups render gitops %s` first): %w", rp.Instance, unitDir, ws.Name, err)
		}
		if rp.Controller != nil && rp.Controller.Kind == v1.RoleSRC {
			intent := rp.Controller.Intent
			if intentSpec, ok := srcCLI.spec.Intents[intent]; ok {
				if err := applyUnitDir(cmd.Context(), kc, unitDir, dryRun, out); err != nil {
					return err
				}
				if err := invokeSRCCliWithArgs(cmd.Context(), runner, srcCLI.spec.Binary, intentSpec.Args, unitDir, kc.KubeContext(), rp, out, dryRun); err != nil {
					return err
				}
			} else {
				if err := invokeSRCCli(cmd.Context(), runner, srcCLI.spec, unitDir, kc.KubeContext(), rp, out, dryRun); err != nil {
					return err
				}
			}
		} else {
			if err := applyUnitDir(cmd.Context(), kc, unitDir, dryRun, out); err != nil {
				return err
			}
		}
		appliedRepos[rp.RenderedPaths.Repo] = true
		waveReady = append(waveReady, readinessTargetsFor(rp, cat)...)
		if waitCRDs && !dryRun && rp.Renderer == "olm" {
			ns, _ := rp.ResolvedValues["namespace"].(string)
			if ns != "" {
				sub := []cluster.SubscriptionRef{{Namespace: ns, Name: rp.Instance}}
				fmt.Fprintf(out, "gitups: waiting on subscription %s/%s\n", ns, rp.Instance)
				if err := cluster.WaitForSubscriptions(cmd.Context(), kc, sub, cluster.WaitOptions{Timeout: waitTimeout, Out: stdioWriter{w: out}}); err != nil {
					return fmt.Errorf("wait after %s: %w", rp.Instance, err)
				}
			}
		}
	}
	if !dryRun && len(waveReady) > 0 {
		if err := waitForReadiness(cmd.Context(), kc, waveReady, waitTimeout, out); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "gitups: bootstrap complete; handoff to in-cluster KRC/SRC.\n")
	return nil
}

type writer interface{ Write([]byte) (int, error) }

// retries once after waiting for CRDs to establish — the first apply often races CRD registration
func applyUnitDir(ctx context.Context, kc *cluster.KubeClient, dir string, dryRun bool, out writer) error {
	run := func(label string) error {
		fmt.Fprintf(out, "gitups: apply [%s]\n", label)
		return kc.ApplyKustomize(ctx, dir, dryRun, stdioWriter{w: out})
	}
	if err := run("pass 1"); err != nil {
		if dryRun {
			return fmt.Errorf("apply -k %s: %w", dir, err)
		}
		fmt.Fprintf(out, "gitups: pass 1 reported errors; waiting for CRD establishment before retry\n")
		if waitErr := waitForCRDsEstablished(ctx, kc, out); waitErr != nil {
			fmt.Fprintf(out, "gitups: CRD establishment wait did not complete cleanly: %v\n", waitErr)
		}
		if err2 := run("pass 2"); err2 != nil {
			return fmt.Errorf("apply -k %s (both passes failed): %w", dir, err2)
		}
	}
	return nil
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

// best-effort gate: package-level readiness often points at CRs that only exist after a later wave,
// so wait failures are logged and skipped — the dependsOn DAG remains the real ordering authority.
func waitForReadiness(ctx context.Context, kc *cluster.KubeClient, targets []readinessTarget, timeout time.Duration, out writer) error {
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
		fmt.Fprintf(out, "gitups: wave gate — %s/%s/%s condition=%s (best-effort, %s)\n", t.Kind, t.Namespace, t.Name, t.Condition, perTarget)
		if err := kc.WaitCondition(ctx, t.Namespace, t.Kind, t.Name, t.Condition, perTarget, stdioWriter{w: out}); err != nil {
			fmt.Fprintf(out, "gitups: wave gate skipped %s/%s/%s — %v (continuing; dependsOn ordering is still authoritative)\n", t.Kind, t.Namespace, t.Name, err)
		}
	}
	return nil
}

func newKubeClientFromGitOpsPackageSet(prov *v1.GitOpsPackageSet, cat *catalog.Catalog, toContext string) (*cluster.KubeClient, error) {
	if prov.Spec.Controllers == nil || prov.Spec.Controllers.KubernetesResources == nil {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources is required — the KRC declares the cluster binary gitups uses")
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
				return nil, fmt.Errorf("KRC package %q has no spec.cli declared — gitups needs it to know what binary to run for apply/wait",
					entry.Def.Metadata.Name)
			}
			return cluster.NewKubeClient(entry.Def.Spec.CLI, entry.Def.Metadata.Name, toContext, cluster.DefaultCLIRunner{})
		}
	}
	return nil, fmt.Errorf("KRC instance %q not found in repo %q", a.Instance, a.Repo)
}

func invokeSRCCli(ctx context.Context, runner cluster.CLIRunner, spec *v1.ControllerCLI, unitDir, toContext string, rp *v1.ResolvedPackage, out writer, dryRun bool) error {
	return invokeSRCCliWithArgs(ctx, runner, spec.Binary, spec.Args, unitDir, toContext, rp, out, dryRun)
}

func invokeSRCCliWithArgs(ctx context.Context, runner cluster.CLIRunner, binary string, argsTmpl []string, unitDir, toContext string, rp *v1.ResolvedPackage, out writer, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(out, "gitups: [dry-run] %s (skipped: SRC CLI has no uniform --dry-run contract) [%s]\n", binary, unitDir)
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
	fmt.Fprintf(out, "gitups: %s %s\n", binary, strings.Join(args, " "))
	if err := runner.Run(ctx, binary, args, out, out); err != nil {
		return fmt.Errorf("unit %s: %s %s: %w", rp.Instance, binary, strings.Join(args, " "), err)
	}
	return nil
}

// advisory only — never blocks apply
func compatibilityWarnings(ctx context.Context, cat *catalog.Catalog, planned []*v1.ResolvedPackage, kc *cluster.KubeClient) []string {
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
		if !k8sVersionSatisfies(serverVer, c.Kubernetes) {
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

// supported constraint syntax: ">=1.N", "<1.N", "<=1.N", ">1.N", "==1.N", or plain "1.N";
// unrecognised grammars are treated as matched so the check stays a hint, not a false-alarm gate.
func k8sVersionSatisfies(server string, constraints []string) bool {
	sMaj, sMin := parseMajorMinor(server)
	if sMaj == 0 {
		return true
	}
	for _, c := range constraints {
		c = strings.TrimSpace(c)
		op, rest := splitOp(c)
		cMaj, cMin := parseMajorMinor(rest)
		if cMaj == 0 {
			continue
		}
		cmp := (sMaj*1000 + sMin) - (cMaj*1000 + cMin)
		ok := true
		switch op {
		case ">=":
			ok = cmp >= 0
		case ">":
			ok = cmp > 0
		case "<=":
			ok = cmp <= 0
		case "<":
			ok = cmp < 0
		case "==", "":
			ok = cmp == 0
		}
		if !ok {
			return false
		}
	}
	return true
}

func splitOp(c string) (op, rest string) {
	for _, o := range []string{">=", "<=", "==", ">", "<"} {
		if strings.HasPrefix(c, o) {
			return o, strings.TrimSpace(c[len(o):])
		}
	}
	return "", c
}

func parseMajorMinor(s string) (maj, min int) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	for i, p := range parts[:2] {
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		if i == 0 {
			maj = n
		} else {
			min = n
		}
	}
	return maj, min
}

func writeBootstrapPlan(out writer, fp *v1.GitOpsPackageSet, planned []*v1.ResolvedPackage) {
	total := len(fp.Spec.Resolved.Packages)
	deferred := total - len(planned)
	fmt.Fprintf(out, "gitups: plan — %d direct, %d deferred to KRC (total %d); handoff after direct set succeeds\n",
		len(planned), deferred, total)
	const maxList = 30
	for i, rp := range planned {
		if i == maxList {
			fmt.Fprintf(out, "gitups:   ... (+%d more; run `gitups plan gitops %s` for the full list)\n",
				len(planned)-maxList, fp.Metadata.Name)
			break
		}
		fmt.Fprintf(out, "gitups:   [wave %d] %s (%s) → %s\n",
			rp.ApplyWave, rp.Instance, planUnitTag(rp), rp.RenderedPaths.Repo)
	}
}

func planUnitTag(rp *v1.ResolvedPackage) string {
	switch {
	case rp.Controller != nil:
		return fmt.Sprintf("%s/%s", rp.Controller.Instance, rp.Controller.Intent)
	case rp.UnitType == v1.UnitTypeInstall:
		return fmt.Sprintf("install/%s", rp.InstallMethod)
	case rp.UnitType == v1.UnitTypeResource:
		if rp.Role == v1.RoleKRC || rp.Role == v1.RoleSRC {
			return fmt.Sprintf("%s/self", rp.Role)
		}
		return fmt.Sprintf("resource/%s", rp.ResourceTemplate)
	}
	return rp.UnitType
}

func bootstrapSubset(fp *v1.GitOpsPackageSet) []*v1.ResolvedPackage {
	var out []*v1.ResolvedPackage
	for i := range fp.Spec.Resolved.Packages {
		rp := &fp.Spec.Resolved.Packages[i]
		switch {
		case rp.UnitType == v1.UnitTypeInstall:
			out = append(out, rp)
		case rp.Controller != nil:
			out = append(out, rp)
		case rp.Role == v1.RoleKRC || rp.Role == v1.RoleSRC:
			out = append(out, rp)
		}
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

func buildGitOpsPackageSetCatalog(prov *v1.GitOpsPackageSet, ws workspace) (*catalog.Catalog, error) {
	registerGitopsSourceResolvers(prov.Spec.Sources, packageNamesByTemplate(prov))
	return catalog.Build(prov.Spec.Sources, ws.Root)
}

func registerGitopsSourceResolvers(sources []v1.PackageSource, names map[string][]string) {
	cacheDir := filepath.Join(defaultStateDir(), "gitops", "sources")
	for _, s := range sources {
		switch {
		case s.OCI != nil:
			r := &catalog.OCIResolver{
				CacheDir:     cacheDir,
				Stdout:       os.Stdout,
				Stderr:       os.Stderr,
				PackageNames: names[s.Name],
			}
			catalog.RegisterSourceResolver("oci", r.Resolve)
		case s.Git != nil:
			r := &catalog.GitResolver{
				CacheDir: cacheDir,
				Stdout:   os.Stdout,
				Stderr:   os.Stderr,
			}
			catalog.RegisterSourceResolver("git", r.Resolve)
		}
	}
}

func packageNamesFromExpandedPackageSet(fp *v1.GitOpsPackageSet) map[string][]string {
	out := map[string]map[string]struct{}{}
	for _, pkg := range fp.Spec.Resolved.Packages {
		parts := strings.SplitN(pkg.Template, "/", 2)
		if len(parts) != 2 {
			continue
		}
		set, ok := out[parts[0]]
		if !ok {
			set = map[string]struct{}{}
			out[parts[0]] = set
		}
		set[parts[1]] = struct{}{}
	}
	flat := map[string][]string{}
	for src, set := range out {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		flat[src] = names
	}
	return flat
}

func packageNamesByTemplate(prov *v1.GitOpsPackageSet) map[string][]string {
	out := map[string]map[string]struct{}{}
	for _, repo := range prov.Spec.Repositories {
		for _, pkg := range repo.Packages {
			parts := strings.SplitN(pkg.Template, "/", 2)
			if len(parts) != 2 {
				continue
			}
			set, ok := out[parts[0]]
			if !ok {
				set = map[string]struct{}{}
				out[parts[0]] = set
			}
			set[parts[1]] = struct{}{}
		}
	}
	flat := map[string][]string{}
	for src, set := range out {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		flat[src] = names
	}
	return flat
}

func waitForCRDsEstablished(ctx context.Context, kc *cluster.KubeClient, out interface{ Write([]byte) (int, error) }) error {
	// kubectl wait --all on a CRD-less cluster reports "no matching resources" — skip cleanly
	if !crdsExist(ctx, kc) {
		fmt.Fprintf(out, "gitups: no CRDs yet on %s; skipping establishment wait\n", kc.KubeContext())
		return nil
	}
	return kc.WaitCRDsEstablished(ctx, 60*time.Second, out)
}

func crdsExist(ctx context.Context, kc *cluster.KubeClient) bool {
	body, err := kc.ListCRDs(ctx)
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(body))) > 0
}

func newGitopsWaitCmd() *cobra.Command {
	var (
		outputDir string
		toContext string
		timeout   time.Duration
	)
	cmd := &cobra.Command{
		Use:   "gitops <name> --to <cluster-context>",
		Short: "Block until OLM subscriptions referenced by <name> have Succeeded CSVs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toContext == "" {
				return fmt.Errorf("--to <cluster-context> is required")
			}
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)", ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			provPath := filepath.Join(ws.Root, "gitops-package-set.yaml")
			prov, err := load.PackageSet(provPath)
			if err != nil {
				return fmt.Errorf("load package set %s: %w", provPath, err)
			}
			cat, err := buildGitOpsPackageSetCatalog(prov, ws)
			if err != nil {
				return err
			}
			kc, err := newKubeClientFromGitOpsPackageSet(prov, cat, toContext)
			if err != nil {
				return err
			}
			if _, err := exec.LookPath(kc.Binary()); err != nil {
				return fmt.Errorf("KRC %q: required binary %q not found in PATH",
					kc.KRCName(), kc.Binary())
			}
			subs := cluster.SubscriptionsFromPackages(fp.Spec.Resolved.Packages)
			out := cmd.ErrOrStderr()
			if len(subs) == 0 {
				fmt.Fprintf(out, "gitups: no OLM subscriptions in %s\n", ws.ExpandedPackageSet)
				return nil
			}
			fmt.Fprintf(out, "gitups: waiting on %d subscription(s) via %s (timeout %s)\n",
				len(subs), kc.Binary(), timeout)
			return cluster.WaitForSubscriptions(cmd.Context(), kc, subs, cluster.WaitOptions{
				Timeout: timeout,
				Out:     out,
			})
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringVar(&toContext, "to", "", "cluster context to talk to (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "overall wait budget across all subscriptions")
	cmd.SetContext(context.Background())
	return cmd
}

func newGitopsStatusCmd() *cobra.Command {
	var (
		outputDir string
		showDiff  bool
		diffLines int
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Report drift between rendered repos and expanded GitOpsPackageSet <name>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)",
					ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			baseDir := ws.Root
			registerGitopsSourceResolvers(fp.Spec.Sources, packageNamesFromExpandedPackageSet(fp))
			cat, err := catalog.Build(fp.Spec.Sources, baseDir)
			if err != nil {
				return err
			}
			if err := ensureBinaries(); err != nil {
				return err
			}
			scratchRoot, err := os.MkdirTemp("", "gitups-status-")
			if err != nil {
				return fmt.Errorf("create scratch: %w", err)
			}
			defer os.RemoveAll(scratchRoot)
			scratchOut := filepath.Join(scratchRoot, ws.Name)
			if err := render.Render(cmd.Context(), fp, cat, render.Options{
				OutputPath:             scratchOut,
				KubectlContext:         currentKubectlContext(),
				AllowPlaceholders:      true,
				SuppressPackageSetCopy: true,
			}); err != nil {
				return fmt.Errorf("dry render: %w", err)
			}
			drifts, err := diffWorkspace(ws.RenderRoot, scratchOut)
			if err != nil {
				return err
			}
			out := cmd.ErrOrStderr()
			if len(drifts) == 0 {
				fmt.Fprintf(out, "gitups: %s is up to date with %s\n", ws.RenderRoot, ws.ExpandedPackageSet)
				return nil
			}
			fmt.Fprintf(out, "gitups: %d drift(s) between %s and %s:\n",
				len(drifts), ws.RenderRoot, ws.ExpandedPackageSet)
			for _, d := range drifts {
				fmt.Fprintf(out, "  %-12s %s\n", d.Kind, d.Path)
				if showDiff && d.Kind == "modified" {
					writeDriftDiff(out, filepath.Join(scratchOut, d.Path), filepath.Join(ws.RenderRoot, d.Path), diffLines)
				}
			}
			return fmt.Errorf("drift detected; re-render with `gitups render gitops %s` to reconcile", ws.Name)
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().BoolVar(&showDiff, "diff", false, "print a unified diff for each modified file")
	cmd.Flags().IntVar(&diffLines, "diff-lines", 20, "max lines of diff to print per modified file (use 0 for unlimited)")
	cmd.SetContext(context.Background())
	return cmd
}

func writeDriftDiff(out writer, want, have string, maxLines int) {
	wantBody, werr := os.ReadFile(want)
	haveBody, herr := os.ReadFile(have)
	if werr != nil || herr != nil {
		return
	}
	wantLines := strings.Split(string(wantBody), "\n")
	haveLines := strings.Split(string(haveBody), "\n")
	var lines []string
	n := len(wantLines)
	if len(haveLines) < n {
		n = len(haveLines)
	}
	start := 0
	for start < n && wantLines[start] == haveLines[start] {
		start++
	}
	for i := start; i < len(wantLines); i++ {
		if i >= start+maxLines {
			lines = append(lines, fmt.Sprintf("      ... (+%d more lines in want)", len(wantLines)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("    - %s", wantLines[i]))
	}
	for i := start; i < len(haveLines); i++ {
		if i >= start+maxLines {
			lines = append(lines, fmt.Sprintf("      ... (+%d more lines in have)", len(haveLines)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("    + %s", haveLines[i]))
	}
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
}

type drift struct {
	Kind string // missing | modified | extra | orphan-dir | missing-dir
	Path string
}

func diffWorkspace(wsRoot, rendered string) ([]drift, error) {
	rEntries, err := os.ReadDir(rendered)
	if err != nil {
		return nil, fmt.Errorf("read rendered: %w", err)
	}
	var drifts []drift
	rendereredRepos := map[string]bool{}
	for _, e := range rEntries {
		if !e.IsDir() {
			continue
		}
		rendereredRepos[e.Name()] = true
		wsPath := filepath.Join(wsRoot, e.Name())
		rPath := filepath.Join(rendered, e.Name())
		if _, err := os.Stat(wsPath); errors.Is(err, fs.ErrNotExist) {
			drifts = append(drifts, drift{Kind: "missing-dir", Path: e.Name() + "/"})
		}
		if err := compareRepoTree(rPath, wsPath, e.Name(), &drifts); err != nil {
			return nil, err
		}
	}
	wsEntries, err := os.ReadDir(wsRoot)
	if err == nil {
		for _, e := range wsEntries {
			if !e.IsDir() {
				continue
			}
			if !rendereredRepos[e.Name()] {
				drifts = append(drifts, drift{Kind: "orphan-dir", Path: e.Name() + "/"})
			}
		}
	}
	sort.SliceStable(drifts, func(i, j int) bool {
		if drifts[i].Path == drifts[j].Path {
			return drifts[i].Kind < drifts[j].Kind
		}
		return drifts[i].Path < drifts[j].Path
	})
	return drifts, nil
}

func compareRepoTree(rendered, workspace, prefix string, drifts *[]drift) error {
	rFiles := map[string]bool{}
	err := filepath.WalkDir(rendered, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(rendered, p)
		rFiles[rel] = true
		displayPath := filepath.Join(prefix, rel)
		wsFile := filepath.Join(workspace, rel)
		wsBody, werr := os.ReadFile(wsFile)
		if errors.Is(werr, fs.ErrNotExist) {
			*drifts = append(*drifts, drift{Kind: "missing", Path: displayPath})
			return nil
		}
		if werr != nil {
			return werr
		}
		rBody, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if !bytes.Equal(rBody, wsBody) {
			*drifts = append(*drifts, drift{Kind: "modified", Path: displayPath})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(workspace); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(workspace, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(workspace, p)
		if !rFiles[rel] {
			*drifts = append(*drifts, drift{Kind: "extra", Path: filepath.Join(prefix, rel)})
		}
		return nil
	})
}

func scaffoldGitOpsPackageSet(name string) string {
	return fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: %s
spec:
  # Package sources: where gitups looks up package definitions.
  sources: []
  # - name: local
  #   filesystem:
  #     path: ./packages

  # Repositories select package installs and environment resources.
  repositories: []
  # - name: platform
  #   type: kubernetes-resources
  #   packages:
  #     - template: local/olm
  #     - template: local/metallb
  #       installMethod: helm
  # - name: platform-{{.Env}}
  #   type: kubernetes-resources
  #   repoRef:
  #     name: platform
  #     commit: v0.0.1
`, name)
}

func printPlaceholderSummary(w interface{ Write([]byte) (int, error) }, fp *v1.GitOpsPackageSet) {
	if len(fp.Spec.Resolved.Placeholders) == 0 {
		fmt.Fprintf(stdioWriter{w}, "gitups: no placeholders; ready to render.\n")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "gitups: %d placeholder(s) require user input:\n", len(fp.Spec.Resolved.Placeholders))
	for _, ph := range fp.Spec.Resolved.Placeholders {
		tag := ""
		if ph.Sensitive {
			tag = " [sensitive]"
		}
		fmt.Fprintf(&b, "  %s%s — %s\n", ph.Path, tag, ph.Reason)
	}
	_, _ = w.Write([]byte(b.String()))
}

type stdioWriter struct {
	w interface{ Write([]byte) (int, error) }
}

func (s stdioWriter) Write(p []byte) (int, error) { return s.w.Write(p) }

func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func ensureBinaries() error {
	for _, bin := range []string{"helm", "kustomize"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required binary %q not found in PATH", bin)
		}
	}
	return nil
}

func currentKubectlContext() string { return "" }

func newGitopsPlanCmd() *cobra.Command {
	var (
		outputDir string
		full      bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Print the ordered apply plan without touching the cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)", ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			var prov *v1.GitOpsPackageSet
			provPath := filepath.Join(ws.Root, "gitops-package-set.yaml")
			if _, err := os.Stat(provPath); err == nil {
				prov, _ = load.PackageSet(provPath)
			}
			hasControllers := prov != nil && prov.Spec.Controllers != nil &&
				(prov.Spec.Controllers.KubernetesResources != nil || prov.Spec.Controllers.ServiceResources != nil)

			out := cmd.OutOrStdout()
			if full || !hasControllers {
				seen := map[string]bool{}
				var repos []string
				for i := range fp.Spec.Resolved.Packages {
					r := fp.Spec.Resolved.Packages[i].RenderedPaths.Repo
					if !seen[r] {
						seen[r] = true
						repos = append(repos, r)
					}
				}
				fmt.Fprintf(out, "gitups: mode=full; %d repo(s), %d unit(s)\n", len(repos), len(fp.Spec.Resolved.Packages))
				for _, r := range repos {
					fmt.Fprintf(out, "  repo %s\n", r)
					for i := range fp.Spec.Resolved.Packages {
						rp := &fp.Spec.Resolved.Packages[i]
						if rp.RenderedPaths.Repo != r {
							continue
						}
						fmt.Fprintf(out, "    [wave %d] %s (%s)\n", rp.ApplyWave, rp.Instance, planUnitTag(rp))
					}
				}
				return nil
			}
			planned := bootstrapSubset(fp)
			sort.SliceStable(planned, func(i, j int) bool {
				if planned[i].ApplyWave != planned[j].ApplyWave {
					return planned[i].ApplyWave < planned[j].ApplyWave
				}
				return planned[i].Instance < planned[j].Instance
			})
			fmt.Fprintf(out, "gitups: mode=bootstrap; %d direct, %d deferred to KRC (total %d)\n",
				len(planned), len(fp.Spec.Resolved.Packages)-len(planned), len(fp.Spec.Resolved.Packages))
			for _, rp := range planned {
				fmt.Fprintf(out, "  [wave %d] %-48s (%s) → %s\n", rp.ApplyWave, rp.Instance, planUnitTag(rp), rp.RenderedPaths.Repo)
			}
			inPlan := map[string]bool{}
			for _, rp := range planned {
				inPlan[rp.Instance] = true
			}
			deferred := 0
			for i := range fp.Spec.Resolved.Packages {
				if !inPlan[fp.Spec.Resolved.Packages[i].Instance] {
					deferred++
				}
			}
			if deferred > 0 {
				fmt.Fprintf(out, "gitups: deferred to KRC — %d unit(s) land after handoff\n", deferred)
			}
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().BoolVar(&full, "full", false, "show the full-tree plan even when spec.controllers declares a KRC/SRC")
	cmd.SetContext(context.Background())
	return cmd
}

func newGitopsFillCmd() *cobra.Command {
	var (
		outputDir string
		sets      []string
	)
	cmd := &cobra.Command{
		Use:   "gitops <name>",
		Short: "Fill placeholders in expanded GitOpsPackageSet <name> via --set args",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				return fmt.Errorf("%s not found (run `gitups expand gitops %s` first)", ws.ExpandedPackageSet, ws.Name)
			}
			fp, err := load.ExpandedPackageSet(ws.ExpandedPackageSet)
			if err != nil {
				return err
			}
			if fp.Metadata.Name != ws.Name {
				return fmt.Errorf("%s has metadata.name %q but workspace is %q",
					ws.ExpandedPackageSet, fp.Metadata.Name, ws.Name)
			}
			byInstance := map[string]*v1.ResolvedPackage{}
			for i := range fp.Spec.Resolved.Packages {
				byInstance[fp.Spec.Resolved.Packages[i].Instance] = &fp.Spec.Resolved.Packages[i]
			}
			out := cmd.ErrOrStderr()
			for _, s := range sets {
				inst, path, val, err := parseFillSet(s)
				if err != nil {
					return fmt.Errorf("--set %q: %w", s, err)
				}
				rp, ok := byInstance[inst]
				if !ok {
					return fmt.Errorf("--set %q: instance %q not found in %s", s, inst, ws.ExpandedPackageSet)
				}
				if rp.ResolvedValues == nil {
					rp.ResolvedValues = map[string]any{}
				}
				if err := setDottedPath(rp.ResolvedValues, path, val); err != nil {
					return fmt.Errorf("--set %q: %w", s, err)
				}
				fmt.Fprintf(out, "gitups: set %s.%s\n", inst, path)
			}
			var remaining []v1.Placeholder
			for i := range fp.Spec.Resolved.Packages {
				rp := &fp.Spec.Resolved.Packages[i]
				if placeholders.Contains(rp.ResolvedValues) {
					for _, ph := range fp.Spec.Resolved.Placeholders {
						if strings.HasPrefix(ph.Path, fmt.Sprintf("spec.resolved.packages[%s].", rp.Instance)) {
							remaining = append(remaining, ph)
						}
					}
				}
			}
			fp.Spec.Resolved.Placeholders = remaining
			body, err := yaml.Marshal(fp)
			if err != nil {
				return fmt.Errorf("marshal package-set: %w", err)
			}
			if err := os.WriteFile(ws.ExpandedPackageSet, body, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", ws.ExpandedPackageSet, err)
			}
			fmt.Fprintf(out, "gitups: wrote %s (%d placeholder(s) remaining)\n", ws.ExpandedPackageSet, len(remaining))
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringArrayVar(&sets, "set", nil, "repeatable: <instance>.<dotted.path>=<value>")
	cmd.SetContext(context.Background())
	return cmd
}

// values are stored as strings; descriptor types govern coercion downstream
func parseFillSet(s string) (instance, path, value string, err error) {
	eq := strings.IndexByte(s, '=')
	if eq < 0 {
		return "", "", "", fmt.Errorf("missing '='")
	}
	left := s[:eq]
	value = s[eq+1:]
	dot := strings.IndexByte(left, '.')
	if dot < 0 {
		return "", "", "", fmt.Errorf("missing '.' between <instance> and <path>")
	}
	instance = left[:dot]
	path = left[dot+1:]
	if instance == "" || path == "" {
		return "", "", "", fmt.Errorf("instance and path are both required")
	}
	return instance, path, value, nil
}

// array indices not supported — edit YAML directly for nested arrays
func setDottedPath(m map[string]any, path string, v any) error {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = v
			return nil
		}
		next, ok := cur[p]
		if !ok {
			child := map[string]any{}
			cur[p] = child
			cur = child
			continue
		}
		nm, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("path %q: segment %q is not a map", path, p)
		}
		cur = nm
	}
	return nil
}

func newGitopsDestroyCmd() *cobra.Command {
	var (
		outputDir    string
		kubeContext  string
		archiveRepos bool
		dryRun       bool
		yes          bool
	)
	cmd := &cobra.Command{
		Use:   "gitops <name> --to <cluster-context>",
		Short: "Tear down a bootstrap (delete rendered manifests via the KRC CLI)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := newWorkspace(outputDir, args[0])
			if err != nil {
				return err
			}
			if kubeContext == "" {
				return fmt.Errorf("--to <cluster-context> is required")
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out,
				"gitops destroy is currently advisory: rendered tree at %s\n"+
					"  kubectl --context %s delete -f <repo-dir> --recursive  (per repo, reverse apply order)\n",
				ws.RenderRoot, kubeContext)
			if archiveRepos {
				fmt.Fprintln(out,
					"--archive-repos: not implemented; deprecate or delete repos via your git provider's UI/API.")
			}
			if dryRun {
				fmt.Fprintln(out, "(dry-run; no actions taken)")
				return nil
			}
			if !yes {
				prompt := fmt.Sprintf("Destroy %s on cluster context %q? [y/N] (default: no): ", ws.Name, kubeContext)
				if !confirm(cmd.InOrStdin(), out, prompt) {
					return errors.New("destroy aborted")
				}
			}
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	cmd.Flags().StringVar(&kubeContext, "to", "", "kubectl context to operate against (required)")
	cmd.Flags().BoolVar(&archiveRepos, "archive-repos", false, "archive (do not delete) the published repos via the git provider")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the destroy plan without executing it")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	return cmd
}
