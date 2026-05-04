package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

// Color styles. fatih/color auto-disables ANSI codes when the writer is not
// a TTY (e.g. bytes.Buffer in tests), so adding decoration here does not
// break substring assertions in `internal/cli` tests.
var (
	titleStyle    = color.New(color.Bold, color.FgCyan)
	subtitleStyle = color.New(color.Bold)
	okStyle       = color.New(color.FgGreen)
	failStyle     = color.New(color.Bold, color.FgRed)
	dimStyle      = color.New(color.Faint)
)

func init() {
	if os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}
}

// printTitle prints a bold cyan section header. Pure decoration — every
// substring tests rely on must still be printed in the body below.
func printTitle(w io.Writer, label string) {
	titleStyle.Fprintf(w, "\n▌ %s\n", label)
}

// printSubtitle prints an inline bold label. Callers must pass the exact
// string tests look for (for example "installer assets:", "phases:").
func printSubtitle(w io.Writer, label string) {
	subtitleStyle.Fprintln(w, label)
}

func printOK(w io.Writer, name, detail string) {
	okStyle.Fprint(w, "  ✓ ")
	if detail != "" {
		fmt.Fprintf(w, "%s — %s\n", name, dimStyle.Sprint(detail))
		return
	}
	fmt.Fprintln(w, name)
}

func printFail(w io.Writer, name, detail string) {
	failStyle.Fprint(w, "  ✗ ")
	if detail != "" {
		fmt.Fprintf(w, "%s — %s\n", name, detail)
		return
	}
	fmt.Fprintln(w, name)
}
