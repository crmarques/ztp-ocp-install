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
		ApplyPlaybook: "playbooks/layers/providers/apply.yml",
		NeedsRoot:     true,
		Description:   "converge provider services: proxy, registry, BMC, boot artifacts, and load balancers",
	},
	"cluster": {
		Name:          "cluster",
		ApplyPlaybook: "playbooks/layers/cluster_infra/apply.yml",
		NeedsRoot:     true,
		Description:   "converge per-cluster substrate, networks, name resolution, and VIPs",
	},
	"clusters": {
		Name:          "clusters",
		ApplyPlaybook: "playbooks/layers/openshift/install-agent.yml",
		NeedsRoot:     true,
		Description:   "run openshift-install agent and boot nodes through the provider BMC path",
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
