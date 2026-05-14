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

	"github.com/crmarques/bootwright/internal/gitops/catalog"
	"github.com/crmarques/bootwright/internal/gitops/load"
	"github.com/crmarques/bootwright/internal/gitops/push"
	"github.com/crmarques/bootwright/internal/gitops/render"
	"github.com/crmarques/bootwright/internal/gitops/workflow"
)

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
				return fmt.Errorf("%s not found (run `bootwright expand gitops %s` first)",
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
				return fmt.Errorf("%s not found (run `bootwright expand gitops %s` first)", ws.ExpandedPackageSet, ws.Name)
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
				commitMessage = fmt.Sprintf("bootwright: sync %s", ws.Name)
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
	cmd.Flags().StringVar(&token, "token", "", "override credentials for REST and push; else uses BOOTWRIGHT_PUSH_TOKEN or provider env")
	cmd.Flags().StringVar(&user, "user", "", "basic-auth username injected into push URL when --token is set (default: provider-appropriate literal)")
	cmd.Flags().StringVar(&branch, "branch", "main", "branch to commit and push")
	cmd.Flags().StringVar(&commitMessage, "commit-message", "", "commit message when there are changes (default: \"bootwright: sync <name>\")")
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
	if v := os.Getenv("BOOTWRIGHT_PUSH_TOKEN"); v != "" {
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
				return fmt.Errorf("%s not found (run `bootwright expand gitops %s` and `bootwright render gitops %s` first)",
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
				return fmt.Errorf("apply requires spec.controllers.kubernetesResources in %s — the KRC declares the cluster binary bootwright uses", provPath)
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
