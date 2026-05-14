package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

func newBastionCheckCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	hostStateDir := defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check controller dependencies",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		var state v1alpha1.State
		if len(cf.files) > 0 {
			loaded, err := loadOptionalDesiredState(cf)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		printTitle(stdout, "bastion check")
		return runBastionChecks(stdout, stderr, state, hostStateDir)
	}
	return cmd
}

func runBastionChecks(stdout io.Writer, stderr io.Writer, state v1alpha1.State, hostStateDir string) error {
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

func newBastionApplyCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun     bool
		yes        bool
		secretsDir string
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Install controller prerequisites",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned commands without executing them")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing local install secret material (env: BOOTWRIGHT_SECRETS_DIR)")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		var state v1alpha1.State
		if len(cf.files) > 0 {
			loaded, err := loadOptionalDesiredState(cf)
			if err != nil {
				return failErr(1, err)
			}
			state = loaded
		}
		proxyEnv, err := resolveProxyEnv(state, secretsDir)
		if err != nil {
			return failErr(1, err)
		}
		plan, err := controllerBootstrapPlan(len(proxyEnv) > 0)
		if err != nil {
			return failErr(1, err)
		}
		cliSpec := planControllerCLIInstall(state, cf.stateDir, defaultControllerCLIInstallDir())

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
		if !yes && !confirm(stdin, stdout, "Continue with bootstrap? [y/N] (default: no): ") {
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
