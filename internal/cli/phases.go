package cli

import (
	"fmt"
	"sort"
	"strings"
)

type Phase struct {
	Name          string
	ApplyPlaybook string
	NeedsRoot     bool
	Description   string
}

var phases = map[string]Phase{
	"provider": {
		Name:          "provider",
		ApplyPlaybook: "playbooks/provider-prepare.yml",
		NeedsRoot:     true,
		Description:   "provision provider-scoped services (BMC emulator, boot-artifacts HTTP, mirror registry, managed HAProxy) and host runtime state",
	},
	"cluster": {
		Name:          "cluster",
		ApplyPlaybook: "playbooks/cluster-prepare.yml",
		NeedsRoot:     true,
		Description:   "provision per-cluster substrate (libvirt domains and networks, managed name resolution, /etc/hosts records, VIP plumbing)",
	},
	"clusters": {
		Name:          "clusters",
		ApplyPlaybook: "playbooks/clusters-install.yml",
		NeedsRoot:     true,
		Description:   "run openshift-install agent against the cluster nodes (boots via Redfish, manages the libvirt domain, writes per-cluster install state)",
	},
}

func selectPhases(name string) ([]Phase, error) {
	if strings.TrimSpace(name) == "" {
		return phasesByNames([]string{"provider", "cluster", "clusters"}), nil
	}
	if p, ok := phases[name]; ok {
		return []Phase{p}, nil
	}
	return nil, fmt.Errorf("unknown phase %q (known: %s)", name, phaseNames())
}

func phasesByNames(names []string) []Phase {
	out := make([]Phase, 0, len(names))
	for _, name := range names {
		out = append(out, phases[name])
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
