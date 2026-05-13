package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/crmarques/gitups/internal/gitops/workflow"
)

const defaultGitopsWorkspace = workflow.DefaultWorkspaceRoot

type workspace = workflow.Workspace

func newWorkspace(outputDir, name string) (workspace, error) {
	return workflow.NewWorkspace(outputDir, name)
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
			if err := os.WriteFile(ws.PackageSet, []byte(workflow.ScaffoldPackageSet(ws.Name)), 0o644); err != nil {
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
			baseDir := filepath.Dir(workflow.AbsPath(ws.PackageSet))
			workflow.RegisterSourceResolvers(prov.Spec.Sources, workflow.PackageNamesByTemplate(prov), workflow.SourceCacheDir(defaultStateDir()))
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

			baseDir := filepath.Dir(workflow.AbsPath(ws.PackageSet))
			workflow.RegisterSourceResolvers(prov.Spec.Sources, workflow.PackageNamesByTemplate(prov), workflow.SourceCacheDir(defaultStateDir()))
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
			workflow.RegisterSourceResolvers(fp.Spec.Sources, workflow.PackageNamesFromExpandedPackageSet(fp), workflow.SourceCacheDir(defaultStateDir()))
			cat, err := catalog.Build(fp.Spec.Sources, baseDir)
			if err != nil {
				return err
			}
			if err := workflow.EnsureRenderBinaries(); err != nil {
				return err
			}
			ctx := kubeContext
			if ctx == "" {
				ctx = workflow.CurrentKubectlContext()
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
			return workflow.VerifyDeterminism(cmd.Context(), fp, cat, opts, ws, cmd.ErrOrStderr())
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
			repos := workflow.RenderedRepoNames(fp)
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

			cat, err := workflow.BuildCatalog(prov, ws, workflow.SourceCacheDir(defaultStateDir()))
			if err != nil {
				return err
			}
			kubeClient, err := workflow.NewKubeClientFromPackageSet(prov, cat, toContext)
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
				return workflow.ApplyFullTree(cmd.Context(), fp, ws, kubeClient, workflow.ApplyOptions{DryRun: dryRun, WaitCRDs: waitCRDs, WaitTimeout: waitTimeout, Out: out})
			}
			return workflow.ApplyBootstrapOnly(cmd.Context(), fp, prov, cat, ws, kubeClient, workflow.ApplyOptions{DryRun: dryRun, WaitCRDs: waitCRDs, WaitTimeout: waitTimeout, Out: out})
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
			cat, err := workflow.BuildCatalog(prov, ws, workflow.SourceCacheDir(defaultStateDir()))
			if err != nil {
				return err
			}
			kc, err := workflow.NewKubeClientFromPackageSet(prov, cat, toContext)
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
			workflow.RegisterSourceResolvers(fp.Spec.Sources, workflow.PackageNamesFromExpandedPackageSet(fp), workflow.SourceCacheDir(defaultStateDir()))
			cat, err := catalog.Build(fp.Spec.Sources, baseDir)
			if err != nil {
				return err
			}
			if err := workflow.EnsureRenderBinaries(); err != nil {
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
				KubectlContext:         workflow.CurrentKubectlContext(),
				AllowPlaceholders:      true,
				SuppressPackageSetCopy: true,
			}); err != nil {
				return fmt.Errorf("dry render: %w", err)
			}
			drifts, err := workflow.DiffWorkspace(ws.RenderRoot, scratchOut)
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
					workflow.WriteDriftDiff(out, filepath.Join(scratchOut, d.Path), filepath.Join(ws.RenderRoot, d.Path), diffLines)
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
						fmt.Fprintf(out, "    [wave %d] %s (%s)\n", rp.ApplyWave, rp.Instance, workflow.PlanUnitTag(rp))
					}
				}
				return nil
			}
			planned := workflow.BootstrapSubset(fp)
			workflow.SortPlan(planned)
			fmt.Fprintf(out, "gitups: mode=bootstrap; %d direct, %d deferred to KRC (total %d)\n",
				len(planned), len(fp.Spec.Resolved.Packages)-len(planned), len(fp.Spec.Resolved.Packages))
			for _, rp := range planned {
				fmt.Fprintf(out, "  [wave %d] %-48s (%s) → %s\n", rp.ApplyWave, rp.Instance, workflow.PlanUnitTag(rp), rp.RenderedPaths.Repo)
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
				inst, path, val, err := workflow.ParseFillSet(s)
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
				if err := workflow.SetDottedPath(rp.ResolvedValues, path, val); err != nil {
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
