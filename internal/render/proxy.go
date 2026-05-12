package render

import (
	"fmt"
	"sort"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func managedProxyIsolationEnabled(infra v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, env *v1alpha1.Environment) bool {
	if env == nil || env.Spec.Proxy == nil {
		return false
	}
	if v1alpha1.ProviderProxySquid(provider) == nil {
		return false
	}
	if v1alpha1.ProviderMachineLibvirt(provider) == nil {
		return false
	}
	return primaryLibvirtMachineNetwork(infra).Libvirt != nil
}

func managedProxyClientURL(infra v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, env *v1alpha1.Environment) string {
	if !managedProxyIsolationEnabled(infra, provider, env) {
		return ""
	}
	network := primaryLibvirtMachineNetwork(infra)
	if network.Gateway == "" {
		return ""
	}
	squid := v1alpha1.ProviderProxySquid(provider)
	port := squid.Port
	if port == 0 {
		port = v1alpha1.DefaultSquidPort
	}
	return fmt.Sprintf("http://%s:%d", network.Gateway, port)
}

func managedProxyClientURLForState(state v1alpha1.State, env *v1alpha1.Environment) string {
	if env == nil {
		return ""
	}
	providers := providerIndex(state.InfrastructureProviders)
	names := make([]string, 0, len(state.ClusterInfrastructures))
	byName := map[string]v1alpha1.ClusterInfrastructure{}
	for _, infra := range state.ClusterInfrastructures {
		names = append(names, infra.Metadata.Name)
		byName[infra.Metadata.Name] = infra
	}
	sort.Strings(names)
	for _, name := range names {
		infra := byName[name]
		provider := closureProvider(infra, providers)
		if url := managedProxyClientURL(infra, provider, env); url != "" {
			return url
		}
	}
	return ""
}

func managedProxyClientURLForOCP(state v1alpha1.State, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment) (string, error) {
	if env == nil {
		return "", nil
	}
	infra, err := clusterInfrastructureForOCP(state, ocp)
	if err != nil {
		return "", err
	}
	provider, err := providerForInfrastructure(state, infra)
	if err != nil {
		return "", err
	}
	return managedProxyClientURL(infra, provider, env), nil
}

func primaryLibvirtMachineNetwork(infra v1alpha1.ClusterInfrastructure) MachineNetworkVars {
	for _, network := range machineNetworksList(infra) {
		if network.Libvirt != nil {
			return network
		}
	}
	return MachineNetworkVars{}
}
