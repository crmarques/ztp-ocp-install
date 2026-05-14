package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"text/template"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
)

type CLIRunner interface {
	Run(ctx context.Context, binary string, args []string, stdout, stderr io.Writer) error
}

// CLIContext is the fixed template context exposed to ControllerCLI args;
// any other template variable is a render-time error.
type CLIContext struct {
	KubeContext  string
	ManifestPath string
	Namespace    string
	Kind         string
	Name         string
	Selector     string
	Condition    string
	Timeout      string
	Pod          string
}

func RenderCLIArgs(spec *v1.ControllerCLI, ctx CLIContext) ([]string, error) {
	if spec == nil {
		return nil, fmt.Errorf("controller cli spec is nil")
	}
	if spec.Binary == "" {
		return nil, fmt.Errorf("controller cli binary is empty")
	}
	return renderArgTemplates("cli.args", spec.Args, ctx)
}

func renderArgTemplates(label string, args []string, ctx CLIContext) ([]string, error) {
	out := make([]string, 0, len(args))
	for i, a := range args {
		// missingkey=error so typo'd template vars fail loudly instead of rendering "<no value>".
		tmpl, err := template.New(fmt.Sprintf("%s[%d]", label, i)).Option("missingkey=error").Parse(a)
		if err != nil {
			return nil, fmt.Errorf("parse %s[%d] %q: %w", label, i, a, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return nil, fmt.Errorf("render %s[%d] %q: %w", label, i, a, err)
		}
		out = append(out, buf.String())
	}
	return out, nil
}

func RenderIntentArgs(intent string, args []string, ctx CLIContext) ([]string, error) {
	return renderArgTemplates("cli.intents."+intent+".args", args, ctx)
}

type DefaultCLIRunner struct{}

func (DefaultCLIRunner) Run(ctx context.Context, binary string, args []string, stdout, stderr io.Writer) error {
	c := exec.CommandContext(ctx, binary, args...)
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}
