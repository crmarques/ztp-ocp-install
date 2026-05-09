package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
)

func newBastionCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bastion",
		Short: "Install and configure the bastion (controller) machine",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newBastionCheckCmd(stdout, stderr),
		newBastionApplyCmd(stdin, stdout, stderr),
		newBastionDestroyCmd(stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newBastionCheckCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	hostStateDir := defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify bastion (controller) dependencies are available",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		var state v1alpha1.State
		if len(cf.files) > 0 {
			loaded, err := infra.LoadNormalizeValidate(cf.files)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		printTitle(stdout, "bastion check")
		checks := collectBastionChecks(state, hostStateDir, defaultPreflightDeps)
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
			fmt.Fprintf(stderr, "bastion check: %d required check(s) failed\n", failed)
			return silentExit(1)
		}
		fmt.Fprintf(stdout, "bastion check: all %d check(s) passed\n", len(checks))
		return nil
	}
	return cmd
}

func newBastionApplyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun     bool
		yes        bool
		secretsDir string
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Install bastion (controller) prerequisites",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print bootstrap commands without executing them")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the bootstrap confirmation prompt")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: GITUPS_SECRETS_DIR)")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		var state v1alpha1.State
		if len(cf.files) > 0 {
			loaded, err := infra.LoadNormalizeValidate(cf.files)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		plan, err := controllerBootstrapPlan()
		if err != nil {
			return failErr(1, err)
		}
		cliSpec := planControllerCLIInstall(state, cf.stateDir, defaultControllerCLIInstallDir())
		proxyEnv, err := resolveProxyEnv(state, secretsDir)
		if err != nil {
			return failErr(1, err)
		}

		printTitle(stdout, "bastion apply")
		fmt.Fprintf(stdout, "ansible-core target: managed venv at %s\n", ansibleVenvDir())
		if summary := proxySummary(proxyEnv); summary != "" {
			fmt.Fprintf(stdout, "proxy: %s\n", summary)
		}
		printSubtitle(stdout, "planned actions:")
		for _, step := range plan {
			fmt.Fprintf(stdout, "- %s\n  $ %s\n", step.label, shellQuote(step.cmd))
		}
		switch {
		case cliSpec != nil:
			fmt.Fprintf(stdout, "- install OCP CLIs (oc, kubectl, openshift-install) %s into %s\n  $ %s\n",
				cliSpec.OCPReleaseVersion, cliSpec.InstallDir, shellQuote(cliSpec.PlannedCommand()))
		case len(cf.files) > 0:
			fmt.Fprintln(stdout, "- skipping OCP CLIs: no openshift.release.version declared in state")
		default:
			fmt.Fprintln(stdout, "- skipping OCP CLIs: pass -f <state-dir> so the release version is known")
		}
		if dryRun {
			return nil
		}
		if !yes && !confirm(stdin, stdout, "Continue with bootstrap? [y/N]: ") {
			return failErr(1, errors.New("bootstrap aborted"))
		}
		if err := runBootstrapPlan(c.Context(), stdin, stdout, stderr, plan, proxyEnv); err != nil {
			return err
		}
		if cliSpec != nil {
			if err := runControllerCLIInstall(c.Context(), stdin, stdout, stderr, *cliSpec, proxyEnv); err != nil {
				return failErr(1, err)
			}
		}
		printOK(stdout, "bastion is ready", "")
		return nil
	}
	return cmd
}

func newBastionDestroyCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Remove bastion (controller) Gitups-managed venv and binaries",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the removal plan without removing anything")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the destroy confirmation prompt")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		if !dryRun && !yes {
			return failErr(1, errors.New("destroy refused: pass --yes to confirm teardown, or --dry-run to preview"))
		}
		printTitle(stdout, "bastion destroy")
		targets := []string{ansibleVenvDir(), defaultControllerCLIInstallDir()}
		for _, target := range targets {
			if dryRun {
				fmt.Fprintf(stdout, "dry-run: would remove %s\n", target)
				continue
			}
			if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(stdout, "skipped (absent): %s\n", target)
				continue
			}
			if err := os.RemoveAll(target); err != nil {
				fmt.Fprintf(stderr, "remove %s: %v\n", target, err)
				return silentExit(1)
			}
			fmt.Fprintf(stdout, "removed %s\n", target)
		}
		return nil
	}
	return cmd
}
