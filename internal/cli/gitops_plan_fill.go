package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/load"
	"github.com/crmarques/bootwright/internal/gitops/placeholders"
	"github.com/crmarques/bootwright/internal/gitops/workflow"
)

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
				fmt.Fprintf(out, "bootwright: mode=full; %d repo(s), %d unit(s)\n", len(repos), len(fp.Spec.Resolved.Packages))
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
			fmt.Fprintf(out, "bootwright: mode=bootstrap; %d direct, %d deferred to KRC (total %d)\n",
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
				fmt.Fprintf(out, "bootwright: deferred to KRC — %d unit(s) land after handoff\n", deferred)
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
				fmt.Fprintf(out, "bootwright: set %s.%s\n", inst, path)
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
			fmt.Fprintf(out, "bootwright: wrote %s (%d placeholder(s) remaining)\n", ws.ExpandedPackageSet, len(remaining))
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
