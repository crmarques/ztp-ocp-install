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

// managedProxyClientURL returns the VM-facing client URL (libvirt-bridge
// gateway) embedded in install-config.yaml. Hosts must use the host URL
// instead because the gateway IP only exists after libvirt is up.
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

// managedProxyClientHostURL returns the host-facing client URL (proxy host's
// SSH address) — routable before libvirt is up, so host_proxy can write a
// working HTTP(S)_PROXY into /etc/dnf/dnf.conf during the first run.
func managedProxyClientHostURL(infra v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, env *v1alpha1.Environment) string {
	if !managedProxyIsolationEnabled(infra, provider, env) {
		return ""
	}
	squid := v1alpha1.ProviderProxySquid(provider)
	host, ok := provider.Spec.Hosts[squid.HostRef.Name]
	if !ok || host.SSH == nil || host.SSH.Address == "" {
		return ""
	}
	port := squid.Port
	if port == 0 {
		port = v1alpha1.DefaultSquidPort
	}
	return fmt.Sprintf("http://%s:%d", host.SSH.Address, port)
}

func managedProxyClientURLForState(state v1alpha1.State, env *v1alpha1.Environment) string {
	return managedProxyClientURLForStateUsing(state, env, managedProxyClientURL)
}

func managedProxyClientHostURLForState(state v1alpha1.State, env *v1alpha1.Environment) string {
	return managedProxyClientURLForStateUsing(state, env, managedProxyClientHostURL)
}

func managedProxyClientURLForStateUsing(
	state v1alpha1.State,
	env *v1alpha1.Environment,
	builder func(v1alpha1.ClusterInfrastructure, v1alpha1.InfrastructureProvider, *v1alpha1.Environment) string,
) string {
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
		if url := builder(infra, provider, env); url != "" {
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
