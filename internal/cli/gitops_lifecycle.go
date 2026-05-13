package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/gitops/catalog"
	"github.com/crmarques/gitups/internal/gitops/cluster"
	"github.com/crmarques/gitups/internal/gitops/load"
	"github.com/crmarques/gitups/internal/gitops/render"
	"github.com/crmarques/gitups/internal/gitops/workflow"
)

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
