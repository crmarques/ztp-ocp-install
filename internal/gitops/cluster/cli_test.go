package cluster

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
)

// fakeCLIRunner records every Run() call and returns scripted responses.
// Each entry in responses is consumed in order; if responses is shorter than
// the call count, subsequent calls return the zero response.
type fakeCLIRunner struct {
	calls     []fakeCLICall
	responses []fakeCLIResponse
}

type fakeCLICall struct {
	binary string
	args   []string
}

type fakeCLIResponse struct {
	stdout string
	stderr string
	err    error
}

func (f *fakeCLIRunner) Run(_ context.Context, binary string, args []string, stdout, stderr io.Writer) error {
	idx := len(f.calls)
	f.calls = append(f.calls, fakeCLICall{binary: binary, args: append([]string(nil), args...)})
	if idx >= len(f.responses) {
		return nil
	}
	resp := f.responses[idx]
	if resp.stdout != "" {
		_, _ = io.WriteString(stdout, resp.stdout)
	}
	if resp.stderr != "" {
		_, _ = io.WriteString(stderr, resp.stderr)
	}
	return resp.err
}

func newTestCLI() *v1.ControllerCLI {
	return &v1.ControllerCLI{
		Binary: "kubectl",
		Args:   []string{"--context", "{{.KubeContext}}"},
		Intents: map[string]v1.ControllerCLIIntent{
			v1.IntentApply:               {Args: []string{"apply", "-k", "{{.ManifestPath}}"}},
			v1.IntentApplyDryRun:         {Args: []string{"apply", "-k", "{{.ManifestPath}}", "--dry-run=server"}},
			v1.IntentGetJSON:             {Args: []string{"-n", "{{.Namespace}}", "get", "{{.Kind}}", "{{.Name}}", "-o", "json"}},
			v1.IntentWaitCondition:       {Args: []string{"-n", "{{.Namespace}}", "wait", "--for=condition={{.Condition}}", "{{.Kind}}/{{.Name}}", "--timeout={{.Timeout}}"}},
			v1.IntentWaitCRDsEstablished: {Args: []string{"wait", "--for=condition=Established", "crds", "--all", "--timeout={{.Timeout}}"}},
			v1.IntentListCRDs:            {Args: []string{"get", "crds", "-o", "name"}},
			v1.IntentServerVersion:       {Args: []string{"version", "-o", "json"}},
			v1.IntentListPodsJSONPath:    {Args: []string{"-n", "{{.Namespace}}", "get", "pods", "-l", "{{.Selector}}", "-o", "jsonpath={range .items[*]}{.metadata.name}|{.status.phase}{\"\\n\"}{end}"}},
			v1.IntentPodLogs:             {Args: []string{"-n", "{{.Namespace}}", "logs", "{{.Pod}}", "--tail=40"}},
		},
	}
}

func TestRenderCLIArgs(t *testing.T) {
	cli := newTestCLI()
	args, err := RenderCLIArgs(cli, CLIContext{KubeContext: "kind-test"})
	if err != nil {
		t.Fatalf("RenderCLIArgs: %v", err)
	}
	if got, want := strings.Join(args, " "), "--context kind-test"; got != want {
		t.Fatalf("RenderCLIArgs args = %q, want %q", got, want)
	}
}

func TestRenderCLIArgsNilSpec(t *testing.T) {
	if _, err := RenderCLIArgs(nil, CLIContext{}); err == nil {
		t.Fatal("RenderCLIArgs(nil): expected error")
	}
}

func TestRenderCLIArgsEmptyBinary(t *testing.T) {
	cli := &v1.ControllerCLI{Args: []string{"-x"}}
	if _, err := RenderCLIArgs(cli, CLIContext{}); err == nil {
		t.Fatal("RenderCLIArgs(empty binary): expected error")
	}
}

func TestRenderIntentArgsMissingKeyIsError(t *testing.T) {
	// {{.Bogus}} is not on CLIContext — missingkey=error must reject it.
	_, err := RenderIntentArgs("apply", []string{"--context", "{{.Bogus}}"}, CLIContext{})
	if err == nil {
		t.Fatal("RenderIntentArgs: expected error on unknown template var")
	}
}

func TestNewKubeClientValidation(t *testing.T) {
	if _, err := NewKubeClient(nil, "krc", "kind-test", nil); err == nil {
		t.Fatal("NewKubeClient(nil cli): expected error")
	}
	if _, err := NewKubeClient(&v1.ControllerCLI{}, "krc", "kind-test", nil); err == nil {
		t.Fatal("NewKubeClient(empty binary): expected error")
	}
	kc, err := NewKubeClient(newTestCLI(), "my-krc", "kind-test", &fakeCLIRunner{})
	if err != nil {
		t.Fatalf("NewKubeClient: %v", err)
	}
	if kc.Binary() != "kubectl" || kc.KubeContext() != "kind-test" || kc.KRCName() != "my-krc" {
		t.Fatalf("accessors: got Binary=%q ctx=%q krc=%q", kc.Binary(), kc.KubeContext(), kc.KRCName())
	}
}

func TestKubeClientRenderIntentUnknown(t *testing.T) {
	kc, _ := NewKubeClient(&v1.ControllerCLI{Binary: "kubectl"}, "krc", "kind-test", &fakeCLIRunner{})
	if _, _, err := kc.Render("apply", CLIContext{ManifestPath: "/tmp/k"}); err == nil {
		t.Fatal("Render with no intents declared: expected error")
	}
}

func TestKubeClientApplyKustomize(t *testing.T) {
	runner := &fakeCLIRunner{}
	kc, _ := NewKubeClient(newTestCLI(), "krc", "kind-test", runner)

	var out bytes.Buffer
	if err := kc.ApplyKustomize(context.Background(), "/tmp/k", false, &out); err != nil {
		t.Fatalf("ApplyKustomize: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	got := strings.Join(runner.calls[0].args, " ")
	if got != "apply -k /tmp/k" {
		t.Fatalf("ApplyKustomize args = %q, want %q", got, "apply -k /tmp/k")
	}
	if !strings.Contains(out.String(), "bootwright: kubectl apply -k /tmp/k") {
		t.Fatalf("expected stdout to echo the command, got %q", out.String())
	}

	// dry-run hits a different intent
	if err := kc.ApplyKustomize(context.Background(), "/tmp/k", true, &out); err != nil {
		t.Fatalf("ApplyKustomize dry-run: %v", err)
	}
	got2 := strings.Join(runner.calls[1].args, " ")
	if got2 != "apply -k /tmp/k --dry-run=server" {
		t.Fatalf("ApplyKustomize dry-run args = %q, want %q", got2, "apply -k /tmp/k --dry-run=server")
	}
}

func TestKubeClientGetJSONStderrOnError(t *testing.T) {
	runner := &fakeCLIRunner{
		responses: []fakeCLIResponse{{stderr: "not found", err: errors.New("exit 1")}},
	}
	kc, _ := NewKubeClient(newTestCLI(), "krc", "kind-test", runner)
	_, err := kc.GetJSON(context.Background(), "ns", "csv", "nope")
	if err == nil {
		t.Fatal("GetJSON: expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetJSON error must include stderr; got %q", err.Error())
	}
}

func TestKubeClientWaitConditionFormatting(t *testing.T) {
	runner := &fakeCLIRunner{}
	kc, _ := NewKubeClient(newTestCLI(), "krc", "kind-test", runner)
	if err := kc.WaitCondition(context.Background(), "ns", "csv", "ex", "Succeeded", 30*time.Second, io.Discard); err != nil {
		t.Fatalf("WaitCondition: %v", err)
	}
	got := strings.Join(runner.calls[0].args, " ")
	want := "-n ns wait --for=condition=Succeeded csv/ex --timeout=30s"
	if got != want {
		t.Fatalf("WaitCondition args = %q, want %q", got, want)
	}
}

func TestParseServerMinor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"happy", `{"serverVersion":{"major":"1","minor":"35"}}`, "1.35"},
		{"trailing +", `{"serverVersion":{"major":"1","minor":"30+"}}`, "1.30"},
		{"empty", `{}`, ""},
		{"malformed", `not json`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseServerMinor([]byte(tc.in)); got != tc.want {
				t.Fatalf("ParseServerMinor(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatTimeout(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "10m"},
		{-1, "10m"},
		{30 * time.Second, "30s"},
		{2 * time.Minute, "120s"},
		{500 * time.Millisecond, "500ms"},
	}
	for _, tc := range tests {
		if got := formatTimeout(tc.in); got != tc.want {
			t.Fatalf("formatTimeout(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSubscriptionsFromPackages(t *testing.T) {
	pkgs := []v1.ResolvedPackage{
		{Renderer: "kustomize", Instance: "skip", ResolvedValues: map[string]any{"namespace": "x"}},
		{Renderer: "olm", Instance: "no-ns", ResolvedValues: map[string]any{}},
		{Renderer: "olm", Instance: "ok", ResolvedValues: map[string]any{"namespace": "ns-a"}},
		{Renderer: "olm", Instance: "wrong-type", ResolvedValues: map[string]any{"namespace": 42}},
	}
	subs := SubscriptionsFromPackages(pkgs)
	if len(subs) != 1 || subs[0] != (SubscriptionRef{Namespace: "ns-a", Name: "ok"}) {
		t.Fatalf("SubscriptionsFromPackages = %+v, want exactly {ns-a/ok}", subs)
	}
}

func TestSubscriptionsForRepoFilters(t *testing.T) {
	pkgs := []v1.ResolvedPackage{
		{Renderer: "olm", Instance: "in-repo", RenderedPaths: v1.RenderedPaths{Repo: "platform"}, ResolvedValues: map[string]any{"namespace": "ns-a"}},
		{Renderer: "olm", Instance: "other-repo", RenderedPaths: v1.RenderedPaths{Repo: "other"}, ResolvedValues: map[string]any{"namespace": "ns-b"}},
	}
	subs := SubscriptionsForRepo(pkgs, "platform")
	if len(subs) != 1 || subs[0].Name != "in-repo" {
		t.Fatalf("SubscriptionsForRepo platform = %+v, want exactly {ns-a/in-repo}", subs)
	}
}

func TestWaitForSubscriptionsNoOpEmpty(t *testing.T) {
	if err := WaitForSubscriptions(context.Background(), nil, nil, WaitOptions{}); err != nil {
		t.Fatalf("WaitForSubscriptions(empty): %v", err)
	}
}

func TestWaitForSubscriptionsNilClient(t *testing.T) {
	err := WaitForSubscriptions(context.Background(), nil, []SubscriptionRef{{Namespace: "ns", Name: "x"}}, WaitOptions{Timeout: time.Second})
	if err == nil {
		t.Fatal("WaitForSubscriptions(nil kc): expected error")
	}
}

func TestWaitForSubscriptionsTimeoutCanceled(t *testing.T) {
	// The runner returns an empty body every call → installedCSV never resolves.
	runner := &fakeCLIRunner{responses: []fakeCLIResponse{{stdout: "{}"}, {stdout: "{}"}}}
	kc, _ := NewKubeClient(newTestCLI(), "krc", "kind-test", runner)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := WaitForSubscriptions(ctx, kc, []SubscriptionRef{{Namespace: "ns", Name: "x"}},
		WaitOptions{Timeout: time.Second, Interval: 5 * time.Millisecond, Out: io.Discard})
	if err == nil {
		t.Fatal("WaitForSubscriptions: expected ctx-canceled error")
	}
}

func TestWaitForSubscriptionsHappyPath(t *testing.T) {
	// 1st GetJSON (subscription): installedCSV resolves to "csv-1".
	// 2nd GetJSON (csv): phase Succeeded.
	runner := &fakeCLIRunner{
		responses: []fakeCLIResponse{
			{stdout: `{"status":{"installedCSV":"csv-1"}}`},
			{stdout: `{"status":{"phase":"Succeeded"}}`},
		},
	}
	kc, _ := NewKubeClient(newTestCLI(), "krc", "kind-test", runner)
	var out bytes.Buffer
	err := WaitForSubscriptions(context.Background(), kc,
		[]SubscriptionRef{{Namespace: "ns", Name: "sub-1"}},
		WaitOptions{Timeout: 5 * time.Second, Interval: time.Millisecond, Out: &out})
	if err != nil {
		t.Fatalf("WaitForSubscriptions: %v", err)
	}
	if !strings.Contains(out.String(), "csv ns/csv-1 Succeeded") {
		t.Fatalf("expected success log line; out=%q", out.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (sub + csv), got %d", len(runner.calls))
	}
}

func TestSubInstalledCSV(t *testing.T) {
	if got := subInstalledCSV([]byte(`{"status":{"installedCSV":"foo-v1"}}`)); got != "foo-v1" {
		t.Fatalf("subInstalledCSV happy = %q", got)
	}
	if got := subInstalledCSV([]byte(`{}`)); got != "" {
		t.Fatalf("subInstalledCSV missing = %q", got)
	}
	if got := subInstalledCSV([]byte(`not json`)); got != "" {
		t.Fatalf("subInstalledCSV malformed = %q", got)
	}
}

func TestCSVPhase(t *testing.T) {
	phase, reason := csvPhase([]byte(`{"status":{"phase":"Failed","reason":"InstallPlanFailed"}}`))
	if phase != "Failed" || reason != "InstallPlanFailed" {
		t.Fatalf("csvPhase = (%q, %q)", phase, reason)
	}
}
