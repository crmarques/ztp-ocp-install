package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

func scopeState(state v1alpha1.State, target, scope string) (v1alpha1.State, error) {
	switch target {
	case "clusters", "infra", "all":
		if strings.TrimSpace(scope) == "" {
			return state, nil
		}
		names, err := clusterNamesForTarget(state, target, scope)
		if err != nil {
			return state, err
		}
		return filterStateToClusters(state, names), nil
	default:
		if strings.TrimSpace(scope) != "" {
			return state, fmt.Errorf("--scope is not supported for %s", target)
		}
		return state, nil
	}
}

func clusterNamesForTarget(state v1alpha1.State, target, scope string) ([]string, error) {
	if strings.TrimSpace(scope) != "" {
		names, err := parseClusterScope(scope)
		if err != nil {
			return nil, err
		}
		if err := validateClusterNames(state, names); err != nil {
			return nil, err
		}
		return names, nil
	}

	var names []string
	for _, ocp := range state.OCPClusters {
		switch target {
		case "hub":
			if ocp.Spec.Role == v1alpha1.OCPClusterRoleHub {
				names = append(names, ocp.Metadata.Name)
			}
		default:
			names = append(names, ocp.Metadata.Name)
		}
	}
	if len(names) == 0 {
		switch target {
		case "hub":
			return nil, fmt.Errorf("no hub cluster found")
		default:
			return nil, fmt.Errorf("no clusters found")
		}
	}
	return names, nil
}

func parseClusterScope(scope string) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	for _, part := range strings.Split(scope, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("--scope must name at least one cluster")
	}
	return names, nil
}

func validateClusterNames(state v1alpha1.State, names []string) error {
	known := map[string]bool{}
	for _, ocp := range state.OCPClusters {
		known[ocp.Metadata.Name] = true
	}
	var missing []string
	for _, name := range names {
		if !known[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("unknown cluster(s): %s", strings.Join(missing, ", "))
}

func filterStateToClusters(state v1alpha1.State, names []string) v1alpha1.State {
	selectedOCP := map[string]bool{}
	for _, name := range names {
		selectedOCP[name] = true
	}
	selectedInfra := map[string]bool{}
	filteredOCP := make([]v1alpha1.OCPCluster, 0, len(names))
	for _, ocp := range state.OCPClusters {
		if !selectedOCP[ocp.Metadata.Name] {
			continue
		}
		filteredOCP = append(filteredOCP, ocp)
		selectedInfra[ocp.Spec.InfrastructureRef.Name] = true
	}

	selectedProviders := map[string]bool{}
	filteredInfra := make([]v1alpha1.ClusterInfrastructure, 0, len(state.ClusterInfrastructures))
	for _, infra := range state.ClusterInfrastructures {
		if !selectedInfra[infra.Metadata.Name] {
			continue
		}
		filteredInfra = append(filteredInfra, infra)
		for _, ref := range infra.Spec.ProviderRefs {
			selectedProviders[ref.Name] = true
		}
	}

	filteredProviders := make([]v1alpha1.InfrastructureProvider, 0, len(state.InfrastructureProviders))
	for _, provider := range state.InfrastructureProviders {
		if selectedProviders[provider.Metadata.Name] {
			filteredProviders = append(filteredProviders, provider)
		}
	}

	state.InfrastructureProviders = filteredProviders
	state.ClusterInfrastructures = filteredInfra
	state.OCPClusters = filteredOCP
	return state
}
