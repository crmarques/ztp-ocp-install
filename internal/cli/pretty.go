package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

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

func printTitle(w io.Writer, label string) {
	titleStyle.Fprintf(w, "\n▌ %s\n", label)
}

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
