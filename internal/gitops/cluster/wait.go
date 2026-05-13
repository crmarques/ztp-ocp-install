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

type SubscriptionRef struct {
	Namespace string
	Name      string
}

func SubscriptionsFromPackages(pkgs []v1.ResolvedPackage) []SubscriptionRef {
	var out []SubscriptionRef
	for i := range pkgs {
		rp := &pkgs[i]
		if rp.Renderer != "olm" {
			continue
		}
		ns, _ := rp.ResolvedValues["namespace"].(string)
		if ns == "" {
			continue
		}
		out = append(out, SubscriptionRef{Namespace: ns, Name: rp.Instance})
	}
	return out
}

func SubscriptionsForRepo(pkgs []v1.ResolvedPackage, repo string) []SubscriptionRef {
	var out []SubscriptionRef
	for i := range pkgs {
		rp := &pkgs[i]
		if rp.RenderedPaths.Repo != repo {
			continue
		}
		if rp.Renderer != "olm" {
			continue
		}
		ns, _ := rp.ResolvedValues["namespace"].(string)
		if ns == "" {
			continue
		}
		out = append(out, SubscriptionRef{Namespace: ns, Name: rp.Instance})
	}
	return out
}

type WaitOptions struct {
	Timeout  time.Duration
	Interval time.Duration
	Out      io.Writer
}

func WaitForSubscriptions(ctx context.Context, kc *KubeClient, subs []SubscriptionRef, opts WaitOptions) error {
	if len(subs) == 0 {
		return nil
	}
	if kc == nil {
		return fmt.Errorf("WaitForSubscriptions: KubeClient is nil")
	}
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	deadline := time.Now().Add(opts.Timeout)
	for _, s := range subs {
		if err := waitOne(ctx, kc, s, deadline, opts); err != nil {
			return err
		}
	}
	return nil
}

func waitOne(ctx context.Context, kc *KubeClient, s SubscriptionRef, deadline time.Time, opts WaitOptions) error {
	fmt.Fprintf(opts.Out, "gitups: wait subscription %s/%s → installedCSV\n", s.Namespace, s.Name)
	var csv string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout resolving installedCSV for subscription %s/%s", s.Namespace, s.Name)
		}
		body, err := kc.GetJSON(ctx, s.Namespace, "subscription", s.Name)
		if err == nil {
			csv = subInstalledCSV(body)
			if csv != "" {
				break
			}
		}
		sleep(ctx, opts.Interval)
	}
	fmt.Fprintf(opts.Out, "gitups: wait csv %s/%s → Succeeded\n", s.Namespace, csv)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			surfaceCSVDiagnostics(ctx, kc, s.Namespace, csv, opts.Out)
			return fmt.Errorf("timeout waiting for csv %s/%s to Succeed", s.Namespace, csv)
		}
		body, err := kc.GetJSON(ctx, s.Namespace, "csv", csv)
		if err == nil {
			phase, reason := csvPhase(body)
			switch phase {
			case "Succeeded":
				fmt.Fprintf(opts.Out, "gitups: csv %s/%s Succeeded\n", s.Namespace, csv)
				return nil
			case "Failed":
				surfaceCSVDiagnostics(ctx, kc, s.Namespace, csv, opts.Out)
				return fmt.Errorf("csv %s/%s Failed: %s", s.Namespace, csv, reason)
			}
		}
		sleep(ctx, opts.Interval)
	}
}

func surfaceCSVDiagnostics(ctx context.Context, kc *KubeClient, ns, csv string, out io.Writer) {
	fmt.Fprintf(out, "gitups: diagnostics for csv %s/%s:\n", ns, csv)
	if body, err := kc.GetJSON(ctx, ns, "csv", csv); err == nil {
		phase, reason := csvPhase(body)
		fmt.Fprintf(out, "gitups:   csv.status.phase=%q reason=%q\n", phase, reason)
	}
	selector := "olm.owner=" + csv
	podList, err := kc.ListPodsJSONPath(ctx, ns, selector)
	if err != nil || len(podList) == 0 {
		fmt.Fprintf(out, "gitups:   (no pods labelled %s in %s — skipping log tail)\n", selector, ns)
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(podList)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		podName := parts[0]
		phase := ""
		if len(parts) > 1 {
			phase = parts[1]
		}
		if podName == "" {
			continue
		}
		fmt.Fprintf(out, "gitups:   pod %s phase=%s — last 40 lines:\n", podName, phase)
		body, _ := kc.PodLogs(ctx, ns, podName)
		for _, l := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
			fmt.Fprintf(out, "gitups:     %s\n", l)
		}
	}
}

func subInstalledCSV(body []byte) string {
	var d struct {
		Status struct {
			InstalledCSV string `json:"installedCSV"`
		} `json:"status"`
	}
	if json.Unmarshal(body, &d) != nil {
		return ""
	}
	return d.Status.InstalledCSV
}

func csvPhase(body []byte) (phase, reason string) {
	var d struct {
		Status struct {
			Phase  string `json:"phase"`
			Reason string `json:"reason"`
		} `json:"status"`
	}
	if json.Unmarshal(body, &d) != nil {
		return "", ""
	}
	return d.Status.Phase, d.Status.Reason
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
