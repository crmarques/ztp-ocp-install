package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/catalog"
	"github.com/crmarques/bootwright/internal/gitops/load"
	"github.com/crmarques/bootwright/internal/gitops/resolve"
	"github.com/crmarques/bootwright/internal/gitops/workflow"
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
				"bootwright: scaffolded %s\n  next: edit spec.sources and spec.repositories, then `bootwright expand gitops %s`\n",
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
				return fmt.Errorf("%s not found (run `bootwright init gitops %s` first)", ws.PackageSet, ws.Name)
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
				return fmt.Errorf("%s is still the init scaffold; fill in spec.sources and spec.repositories before `bootwright expand gitops %s`",
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
			fmt.Fprintf(cmd.ErrOrStderr(), "bootwright: wrote %s\n", ws.ExpandedPackageSet)
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
				return fmt.Errorf("%s not found (run `bootwright init gitops %s`)", ws.PackageSet, ws.Name)
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
				fmt.Fprintf(out, "bootwright: %s is the init scaffold (empty sources/repositories) — fill it in, then re-run check\n", ws.PackageSet)
				return nil
			}
			if extFrom != nil {
				fmt.Fprintf(out, "bootwright: %s extends %s\n", ws.PackageSet, extFrom.Source)
			}
			fmt.Fprintf(out, "bootwright: %s ok (%d source(s), %d repositories)\n",
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
			fmt.Fprintf(out, "bootwright: dry expand ok\n")

			if _, err := os.Stat(ws.ExpandedPackageSet); err != nil {
				fmt.Fprintf(out, "bootwright: %s not present — run `bootwright expand gitops %s`\n",
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
			fmt.Fprintf(out, "bootwright: %s ok (%d package(s))\n",
				ws.ExpandedPackageSet, len(fp.Spec.Resolved.Packages))
			printPlaceholderSummary(out, fp)
			return nil
		},
	}
	addOutputDirFlag(cmd, &outputDir)
	return cmd
}
