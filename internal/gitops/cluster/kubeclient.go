package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

type KubeClient struct {
	cli         *v1.ControllerCLI
	kubeContext string
	runner      CLIRunner
	krcName     string
}

func NewKubeClient(cli *v1.ControllerCLI, krcName, kubeContext string, runner CLIRunner) (*KubeClient, error) {
	if cli == nil {
		return nil, fmt.Errorf("KRC %q: spec.cli is not declared", krcName)
	}
	if cli.Binary == "" {
		return nil, fmt.Errorf("KRC %q: spec.cli.binary is empty", krcName)
	}
	if runner == nil {
		runner = DefaultCLIRunner{}
	}
	return &KubeClient{cli: cli, kubeContext: kubeContext, runner: runner, krcName: krcName}, nil
}

func (k *KubeClient) Binary() string {
	if k == nil || k.cli == nil {
		return ""
	}
	return k.cli.Binary
}

func (k *KubeClient) KubeContext() string {
	if k == nil {
		return ""
	}
	return k.kubeContext
}

func (k *KubeClient) KRCName() string {
	if k == nil {
		return ""
	}
	return k.krcName
}

func (k *KubeClient) intentArgs(intent string, ctx CLIContext) ([]string, error) {
	if k == nil || k.cli == nil {
		return nil, fmt.Errorf("KubeClient is not initialised")
	}
	entry, ok := k.cli.Intents[intent]
	if !ok {
		return nil, fmt.Errorf("KRC %q: spec.cli.intents.%s is not declared; gitups needs it for the %q operation",
			k.krcName, intent, intent)
	}
	ctx.KubeContext = k.kubeContext
	return RenderIntentArgs(intent, entry.Args, ctx)
}

func (k *KubeClient) run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return k.runner.Run(ctx, k.cli.Binary, args, stdout, stderr)
}

func (k *KubeClient) output(ctx context.Context, args []string) ([]byte, []byte, error) {
	var stdout, stderr strbuf
	err := k.run(ctx, args, &stdout, &stderr)
	return stdout.Bytes(), stderr.Bytes(), err
}

type strbuf struct{ b []byte }

func (s *strbuf) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
func (s *strbuf) Bytes() []byte               { return s.b }

func (k *KubeClient) Render(intent string, ctx CLIContext) (string, []string, error) {
	args, err := k.intentArgs(intent, ctx)
	if err != nil {
		return "", nil, err
	}
	return k.cli.Binary, args, nil
}

func (k *KubeClient) ApplyKustomize(ctx context.Context, manifestPath string, dryRun bool, out io.Writer) error {
	intent := v1.IntentApply
	if dryRun {
		intent = v1.IntentApplyDryRun
	}
	args, err := k.intentArgs(intent, CLIContext{ManifestPath: manifestPath})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "gitups: %s %s\n", k.cli.Binary, strings.Join(args, " "))
	return k.run(ctx, args, out, out)
}

func (k *KubeClient) GetJSON(ctx context.Context, namespace, kind, name string) ([]byte, error) {
	args, err := k.intentArgs(v1.IntentGetJSON, CLIContext{Namespace: namespace, Kind: kind, Name: name})
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := k.output(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w (stderr: %s)", k.cli.Binary, strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
	}
	return stdout, nil
}

func (k *KubeClient) WaitCondition(ctx context.Context, namespace, kind, name, condition string, timeout time.Duration, out io.Writer) error {
	args, err := k.intentArgs(v1.IntentWaitCondition, CLIContext{
		Namespace: namespace, Kind: kind, Name: name, Condition: condition, Timeout: formatTimeout(timeout),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "gitups: %s %s\n", k.cli.Binary, strings.Join(args, " "))
	return k.run(ctx, args, out, out)
}

func (k *KubeClient) WaitCRDsEstablished(ctx context.Context, timeout time.Duration, out io.Writer) error {
	args, err := k.intentArgs(v1.IntentWaitCRDsEstablished, CLIContext{Timeout: formatTimeout(timeout)})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "gitups: %s %s\n", k.cli.Binary, strings.Join(args, " "))
	return k.run(ctx, args, out, out)
}

func (k *KubeClient) ListCRDs(ctx context.Context) ([]byte, error) {
	args, err := k.intentArgs(v1.IntentListCRDs, CLIContext{})
	if err != nil {
		return nil, err
	}
	stdout, _, err := k.output(ctx, args)
	return stdout, err
}

func (k *KubeClient) ServerVersion(ctx context.Context) ([]byte, error) {
	args, err := k.intentArgs(v1.IntentServerVersion, CLIContext{})
	if err != nil {
		return nil, err
	}
	stdout, _, err := k.output(ctx, args)
	return stdout, err
}

func (k *KubeClient) ListPodsJSONPath(ctx context.Context, namespace, selector string) ([]byte, error) {
	args, err := k.intentArgs(v1.IntentListPodsJSONPath, CLIContext{Namespace: namespace, Selector: selector})
	if err != nil {
		return nil, err
	}
	stdout, _, err := k.output(ctx, args)
	return stdout, err
}

func (k *KubeClient) PodLogs(ctx context.Context, namespace, pod string) ([]byte, error) {
	args, err := k.intentArgs(v1.IntentPodLogs, CLIContext{Namespace: namespace, Pod: pod})
	if err != nil {
		return nil, err
	}
	stdout, _, err := k.output(ctx, args)
	return stdout, err
}

func formatTimeout(d time.Duration) string {
	if d <= 0 {
		return "10m"
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	return d.String()
}

// ParseServerMinor extracts "<major>.<minor>" (e.g. "1.35") from a
// `kubectl version -o json` payload; returns "" on any error.
func ParseServerMinor(body []byte) string {
	var d struct {
		ServerVersion struct {
			Major string `json:"major"`
			Minor string `json:"minor"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return ""
	}
	if d.ServerVersion.Major == "" || d.ServerVersion.Minor == "" {
		return ""
	}
	return d.ServerVersion.Major + "." + strings.TrimRight(d.ServerVersion.Minor, "+")
}
