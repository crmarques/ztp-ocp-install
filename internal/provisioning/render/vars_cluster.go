package render

import "github.com/crmarques/bootwright/api/v1alpha1"

func clusterVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) ClusterVars {
	return ClusterVars{
		Name: item.Metadata.Name,
		OCP: OCPClusterVars{
			Name:      ocp.Metadata.Name,
			Topology:  ocp.Spec.Topology,
			Release:   ocpReleaseVars(ocp),
			Install:   ocpInstallVars(ocp, env, secretsDir),
			Installer: ocpInstallerVars(ocp.Metadata.Name),
			Nodes:     ocpClusterNodes(item, ocp, env, secretsDir),
		},
		Provider: providerVars(item, provider, ocp, env, secretsDir),
		Network:  networkVars(item, provider),
	}
}

func networkVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider) ClusterNetworkVars {
	endpoints := endpointVars(item.Spec.Endpoints)
	return ClusterNetworkVars{
		MachineNetworks: machineNetworksList(item),
		Endpoints:       endpoints,
		LoadBalancer:    clusterLoadBalancerVars(item),
		NameResolution:  nameResolutionVars(item, provider),
		APIVIP:          endpoints.API.Address,
		IngressVIP:      endpoints.Ingress.Address,
		Records: DNSRecords{
			API:     endpoints.API.Hostname,
			APIInt:  endpoints.APIInt.Hostname,
			Ingress: endpoints.Ingress.Hostname,
		},
	}
}

func endpointVars(spec v1alpha1.ClusterEndpointsSpec) NetworkEndpointVars {
	out := NetworkEndpointVars{}
	if spec.API != nil {
		out.API = EndpointVar{Hostname: spec.API.Hostname, Address: spec.API.Address}
	}
	if spec.APIInt != nil {
		out.APIInt = EndpointVar{Hostname: spec.APIInt.Hostname, Address: spec.APIInt.Address}
	}
	if spec.Ingress != nil {
		out.Ingress = EndpointVar{Hostname: spec.Ingress.Hostname, Address: spec.Ingress.Address}
	}
	return out
}

func machineNetworksList(item v1alpha1.ClusterInfrastructure) []MachineNetworkVars {
	names := sortedKeys(item.Spec.Networks)
	out := make([]MachineNetworkVars, 0, len(names))
	for _, name := range names {
		n := item.Spec.Networks[name]
		entry := MachineNetworkVars{
			Name:       name,
			CIDR:       n.CIDR,
			Gateway:    n.Gateway,
			DNSServers: append([]string(nil), n.DNSServers...),
		}
		if n.Libvirt != nil {
			entry.Libvirt = &MachineNetworkLibvirtVars{
				LibvirtNetwork: libvirtNetworkName(item.Metadata.Name, name),
				Bridge:         n.Libvirt.Bridge,
			}
		}
		if n.VSphere != nil {
			entry.VSphere = &MachineNetworkVSphereVars{Portgroup: n.VSphere.Portgroup}
		}
		out = append(out, entry)
	}
	return out
}

func libvirtNetworkName(clusterName, networkKey string) string {
	return clusterName + "-" + networkKey
}

func clusterLoadBalancerVars(item v1alpha1.ClusterInfrastructure) ClusterLoadBalancerVars {
	if len(item.Spec.LoadBalancers) == 0 {
		return ClusterLoadBalancerVars{Mode: "external", External: true}
	}
	refs := map[string]string{}
	for lbName, lb := range item.Spec.LoadBalancers {
		for _, ep := range lb.Endpoints {
			refs[ep] = lbName
		}
	}
	external := false
	for _, ep := range []string{v1alpha1.EndpointAPI, v1alpha1.EndpointAPIInt, v1alpha1.EndpointIngress} {
		if _, ok := refs[ep]; !ok {
			external = true
			break
		}
	}
	return ClusterLoadBalancerVars{
		Mode:     "managed",
		External: external,
		Refs:     refs,
	}
}

func nameResolutionVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider) NameResolutionVars {
	if provider.Spec.NameResolution == nil || provider.Spec.NameResolution.HostsFile == nil {
		return NameResolutionVars{Mode: "external"}
	}
	hf := provider.Spec.NameResolution.HostsFile
	refs := make([]string, 0, len(hf.HostRefs))
	for _, r := range hf.HostRefs {
		refs = append(refs, r.Name)
	}
	result := NameResolutionVars{
		Mode: "managed",
		Managed: &ManagedNameResolutionVars{
			ProviderHostRefs: refs,
		},
	}
	result.Managed.HostsFile = &HostsFileVars{
		AdditionalIngressHosts: append([]string(nil), hf.AdditionalIngressHosts...),
		Entries:                hostsFileEntries(item, hf.AdditionalIngressHosts),
	}
	return result
}

func hostsFileEntries(item v1alpha1.ClusterInfrastructure, additionalIngress []string) []HostsFileEntry {
	entries := []HostsFileEntry{}
	if item.Spec.Endpoints.API != nil {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.API.Address,
			Names:   []string{item.Spec.Endpoints.API.Hostname},
		})
	}
	if item.Spec.Endpoints.APIInt != nil {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.APIInt.Address,
			Names:   []string{item.Spec.Endpoints.APIInt.Hostname},
		})
	}
	if item.Spec.Endpoints.Ingress != nil && len(additionalIngress) > 0 {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.Ingress.Address,
			Names:   append([]string(nil), additionalIngress...),
		})
	}
	return entries
}
