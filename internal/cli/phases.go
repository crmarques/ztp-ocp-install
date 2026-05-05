package cli

import (
	"fmt"
	"sort"
	"strings"
)

type Phase struct {
	Name            string
	ApplyPlaybook   string
	DestroyPlaybook string
	NeedsRoot       bool
	Description     string
}

// Playbook paths are relative to the extracted ansible bundle root (see
// internal/embedded). The CLI joins them with the per-run bundle directory
// before handing the spec to the runner.
//
// Apply order is provider → cluster → ocp: provider-scoped services
// (mirror, BMC emulator, HAProxy) come up first so per-cluster substrate
// convergence has somewhere to plumb VIPs, and the ocp installer has a
// reachable mirror in disconnected mode. Destroy reverses the order.
var phases = map[string]Phase{
	"provider": {
		Name:            "provider",
		ApplyPlaybook:   "playbooks/provider-prepare.yml",
		DestroyPlaybook: "playbooks/provider-destroy.yml",
		NeedsRoot:       true,
		Description:     "provision provider-scoped services (BMC emulator, boot-artifacts HTTP, mirror registry, managed HAProxy) and host runtime state",
	},
	"cluster": {
		Name:            "cluster",
		ApplyPlaybook:   "playbooks/cluster-prepare.yml",
		DestroyPlaybook: "playbooks/cluster-destroy.yml",
		NeedsRoot:       true,
		Description:     "provision per-cluster substrate (libvirt domains and networks, managed name resolution, /etc/hosts records, VIP plumbing)",
	},
	"ocp": {
		Name:            "ocp",
		ApplyPlaybook:   "playbooks/ocp-install.yml",
		DestroyPlaybook: "playbooks/ocp-destroy.yml",
		NeedsRoot:       true,
		Description:     "run openshift-install agent against the cluster nodes (boots via Redfish, manages the libvirt domain, writes per-cluster install state)",
	},
}

func workflowPhases(scope string) []Phase {
	names := []string{}
	switch strings.TrimSpace(scope) {
	case "infra":
		names = []string{"provider", "cluster"}
	case "ocp":
		names = []string{"ocp"}
	case "all":
		names = []string{"provider", "cluster", "ocp"}
	}
	out := make([]Phase, 0, len(names))
	for _, name := range names {
		out = append(out, phases[name])
	}
	return out
}

func phasesForApplyScope(scope string) ([]Phase, error) {
	selected := workflowPhases(scope)
	if len(selected) == 0 {
		return nil, fmt.Errorf("unknown apply scope %q (known: infra, ocp)", scope)
	}
	return selected, nil
}

func phasesForDestroyScope(scope string) ([]Phase, error) {
	selected := workflowPhases(scope)
	if len(selected) == 0 {
		return nil, fmt.Errorf("unknown destroy scope %q (known: infra, ocp, all)", scope)
	}
	return reversed(selected), nil
}

func selectPhases(name string) ([]Phase, error) {
	if strings.TrimSpace(name) == "" {
		return workflowPhases("all"), nil
	}
	if p, ok := phases[name]; ok {
		return []Phase{p}, nil
	}
	return nil, fmt.Errorf("unknown phase %q (known: %s)", name, phaseNames())
}

func reversed(in []Phase) []Phase {
	out := make([]Phase, len(in))
	for i, p := range in {
		out[len(in)-1-i] = p
	}
	return out
}

func phaseNames() string {
	names := make([]string, 0, len(phases))
	for name := range phases {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}
