package cli

type scopeSpec struct {
	name              string
	short             string
	phaseNames        []string
	applyPlaybook     string
	destroyPlaybook   string
	artifactsBaseName string
}

var providerScope = scopeSpec{
	name:              "provider",
	short:             "Install and configure InfrastructureProvider and ClusterInfrastructure",
	phaseNames:        []string{"provider", "cluster"},
	applyPlaybook:     "playbooks/apply-infra.yml",
	destroyPlaybook:   "playbooks/destroy-infra.yml",
	artifactsBaseName: "infra",
}

var clustersScope = scopeSpec{
	name:              "clusters",
	short:             "Install and configure all OpenShift clusters via openshift-install agent",
	phaseNames:        []string{"clusters"},
	applyPlaybook:     "playbooks/apply-clusters.yml",
	destroyPlaybook:   "playbooks/clusters-destroy.yml",
	artifactsBaseName: "clusters",
}

func (s scopeSpec) phases() []Phase {
	out := make([]Phase, 0, len(s.phaseNames))
	for _, name := range s.phaseNames {
		out = append(out, phases[name])
	}
	return out
}
