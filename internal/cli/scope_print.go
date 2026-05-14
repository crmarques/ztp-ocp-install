package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func printApplySummary(w io.Writer, selected []Phase, askBecomePass bool, dryRun bool) {
	printWorkflowSummary(w, "apply plan:", selected, askBecomePass, dryRun)
}

func printDestroySummary(w io.Writer, selected []Phase, askBecomePass bool, dryRun bool) {
	printWorkflowSummary(w, "destroy plan:", selected, askBecomePass, dryRun)
}

func printWorkflowSummary(w io.Writer, title string, selected []Phase, askBecomePass bool, dryRun bool) {
	fmt.Fprintln(w, title)
	rootPhases := 0
	for _, p := range selected {
		marker := ""
		if p.NeedsRoot {
			marker = " [root]"
			rootPhases++
		}
		fmt.Fprintf(w, "  - %s%s — %s\n", p.Name, marker, p.Description)
	}
	if rootPhases > 0 {
		switch {
		case dryRun:
			fmt.Fprintln(w, "[root] phases require sudo escalation; this is a dry run, no commands execute.")
		case askBecomePass && rootPhases > 1:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; ansible will prompt once for the BECOME (sudo) password and reuse it for this workflow.")
		case askBecomePass:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; ansible will prompt for the BECOME (sudo) password.")
		case os.Geteuid() == 0:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; bootwright is running as root, no BECOME password prompt needed.")
		default:
			fmt.Fprintln(w, "[root] phases run as root on provider hosts; --ask-become-pass=false requires passwordless sudo or an already-root connection user.")
		}
	}
}

func printWorkflowStart(w io.Writer, workflowName string, selected []Phase, askBecomePass bool) {
	if len(selected) == 1 {
		printPhaseStart(w, selected[0], askBecomePass)
		return
	}
	if rootPhaseCount(selected) > 0 {
		fmt.Fprintf(w, "\n>>> running workflow %q [root] — phases: %s\n", workflowName, phaseList(selected))
		if askBecomePass {
			fmt.Fprintln(w, ">>> ansible may prompt once for the sudo (BECOME) password for this workflow.")
		}
		return
	}
	fmt.Fprintf(w, "\n>>> running workflow %q — phases: %s\n", workflowName, phaseList(selected))
}

func printPhaseStart(w io.Writer, phase Phase, askBecomePass bool) {
	if phase.NeedsRoot && askBecomePass {
		fmt.Fprintf(w, "\n>>> running phase %q [root] — %s\n>>> ansible may prompt for the sudo (BECOME) password for this phase.\n", phase.Name, phase.Description)
		return
	}
	fmt.Fprintf(w, "\n>>> running phase %q — %s\n", phase.Name, phase.Description)
}

func rootPhaseCount(selected []Phase) int {
	count := 0
	for _, p := range selected {
		if p.NeedsRoot {
			count++
		}
	}
	return count
}

func phaseList(selected []Phase) string {
	names := make([]string, 0, len(selected))
	for _, p := range selected {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

var askBecomePassDefault = func() bool { return os.Geteuid() != 0 }

func confirm(in io.Reader, prompt io.Writer, message string) bool {
	if in == nil {
		return false
	}
	fmt.Fprint(prompt, message)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
