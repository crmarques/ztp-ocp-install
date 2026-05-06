package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
)

func newDoctorCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check controller machine prerequisites",
		Long: "Subcommands verify the controller machine dependencies\n" +
			"gitups requires to run playbooks.\n\n" +
			"`doctor check` reports the status of each dependency.\n" +
			"`doctor fix` installs any missing dependencies.",
	}
	cmd.AddCommand(
		newDoctorCheckCmd(stdout, stderr),
		newDoctorFixCmd(stdin, stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newDoctorCheckCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var hostStateDir string
	hostStateDir = defaultHostStateDir
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify controller dependencies are available",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().StringVar(&hostStateDir, "host-state-dir", hostStateDir, "root-managed host runtime state directory")
	_ = cmd.MarkFlagRequired("file")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(cf.files)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Doctor check")
		checks := collectDoctorChecks(state, hostStateDir, defaultPreflightDeps)
		failed := 0
		for _, c := range checks {
			if c.ok {
				printOK(stdout, c.name, c.detail)
			} else {
				printFail(stdout, c.name, c.detail)
				failed++
			}
		}
		if failed > 0 {
			fmt.Fprintf(stderr, "doctor check: %d required check(s) failed\n", failed)
			return silentExit(1)
		}
		fmt.Fprintf(stdout, "doctor check: all %d check(s) passed\n", len(checks))
		return nil
	}
	return cmd
}

func newDoctorFixCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)
	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Install missing controller prerequisites",
		Args:  cobra.NoArgs,
	}
	cf := addCommonFlags(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print bootstrap commands without executing them")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the bootstrap confirmation prompt")
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

		printTitle(stdout, "Doctor fix")
		fmt.Fprintf(stdout, "ansible-core target: managed venv at %s\n", ansibleVenvDir())
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
		if err := runBootstrapPlan(c.Context(), stdin, stdout, stderr, plan); err != nil {
			return err
		}
		if cliSpec != nil {
			if err := runControllerCLIInstall(c.Context(), stdin, stdout, stderr, *cliSpec); err != nil {
				return failErr(1, err)
			}
		}
		printOK(stdout, "controller is ready", "")
		return nil
	}
	return cmd
}
