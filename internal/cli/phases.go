package cli

import (
	"fmt"
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
var phases = []Phase{
	{
		Name:            "infra",
		ApplyPlaybook:   "playbooks/infra-prepare.yml",
		DestroyPlaybook: "playbooks/infra-destroy.yml",
		NeedsRoot:       true,
		Description:     "provision libvirt domains, sushy-tools BMC emulator, HAProxy containers, /etc/hosts records, and host runtime state",
	},
	{
		Name:            "hub",
		ApplyPlaybook:   "playbooks/hub-install.yml",
		DestroyPlaybook: "playbooks/hub-destroy.yml",
		NeedsRoot:       true,
		Description:     "run openshift-install agent against the hub node (boots via Redfish, manages the libvirt domain, writes hub state)",
	},
	{
		Name:            "gitops-publish",
		ApplyPlaybook:   "playbooks/gitops-publish.yml",
		DestroyPlaybook: "playbooks/gitops-unpublish.yml",
		NeedsRoot:       false,
		Description:     "publish rendered manifests to the hub-watched GitOps repository",
	},
}

func selectPhases(name string) ([]Phase, error) {
	if strings.TrimSpace(name) == "" {
		return phases, nil
	}
	for _, p := range phases {
		if p.Name == name {
			return []Phase{p}, nil
		}
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
	for _, p := range phases {
		names = append(names, p.Name)
	}
	return strings.Join(names, "|")
}
