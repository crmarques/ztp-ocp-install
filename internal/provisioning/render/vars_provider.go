package render

import "github.com/crmarques/bootwright/api/v1alpha1"

func providerVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) ProviderVars {
	var result ProviderVars
	result.Kind = v1alpha1.MachineFlavor(provider)
	result.SubstrateRole, result.BMCRole, result.BootArtifactsHttp = providerDispatch(provider)
	if v1alpha1.ProviderMachineLibvirt(provider) == nil {
		return result
	}
	q := provider.Spec.Machine.Libvirt
	result.InfrastructureHosts = providerHostVars(provider.Spec.Hosts, env, secretsDir)
	result.Virtualization = &ProviderVirtualizationVars{
		Type:        v1alpha1.VirtualizationTypeLibvirt,
		Libvirt:     libvirtVars(item, provider, env),
		DefaultNode: defaultNodeVars(q),
	}
	if q.BMCEmulation != nil {
		result.BMC = bmcVars(q.BMCEmulation, nil, env, secretsDir)
	}
	result.Nodes = providerNodes(item, ocp)
	return result
}

func providerDispatch(provider v1alpha1.InfrastructureProvider) (string, string, ProviderBootArtifactsHTTPVars) {
	switch v1alpha1.MachineFlavor(provider) {
	case v1alpha1.MachineFlavorLibvirt:
		http := ProviderBootArtifactsHTTPVars{}
		if v1alpha1.ProviderMachineLibvirt(provider) != nil && provider.Spec.Machine.Libvirt.BMCEmulation != nil && provider.Spec.Machine.Libvirt.BMCEmulation.Port > 0 {
			http = ProviderBootArtifactsHTTPVars{
				Enabled:     true,
				BindAddress: "0.0.0.0",
				Port:        provider.Spec.Machine.Libvirt.BMCEmulation.Port + 2,
			}
		}
		return "libvirt", "emulated", http
	case v1alpha1.MachineFlavorBareMetal:
		return "baremetal", "redfish", ProviderBootArtifactsHTTPVars{}
	case v1alpha1.MachineFlavorVSphere:
		return "vsphere", "none", ProviderBootArtifactsHTTPVars{}
	case v1alpha1.MachineFlavorKubeVirt:
		return "kubevirt", "none", ProviderBootArtifactsHTTPVars{}
	default:
		return "", "none", ProviderBootArtifactsHTTPVars{}
	}
}

func providerHostVars(hosts map[string]v1alpha1.ProviderHostSpec, env *v1alpha1.Environment, secretsDir string) []ProviderHostVars {
	names := sortedKeys(hosts)
	out := make([]ProviderHostVars, 0, len(names))
	for _, name := range names {
		host := hosts[name]
		entry := ProviderHostVars{
			Name:         name,
			LibvirtURI:   host.LibvirtURI,
			Capabilities: append([]string(nil), host.Capabilities...),
		}
		if host.SSH != nil {
			entry.Address = host.SSH.Address
			entry.User = host.SSH.User
			entry.SSHKeyRef = resolvedSecretPath(host.SSH.KeyRef.Name, secretsDir, env)
		}
		out = append(out, entry)
	}
	return out
}

func defaultNodeVars(q *v1alpha1.MachineProviderLibvirtSpec) VirtualNodeResourceVars {
	if len(q.MachineProfiles) == 0 {
		return VirtualNodeResourceVars{
			CPU:       v1alpha1.DefaultNodeCPU,
			MemoryMiB: v1alpha1.DefaultNodeMemoryMiB,
			DiskGiB:   v1alpha1.DefaultNodeDiskGiB,
		}
	}
	keys := sortedKeys(q.MachineProfiles)
	p := q.MachineProfiles[keys[0]]
	return VirtualNodeResourceVars{
		CPU:       p.CPU,
		MemoryMiB: p.MemoryMiB,
		DiskGiB:   p.DiskGiB,
	}
}

func providerComponentVars(state v1alpha1.State, secretsDir string) []ProviderComponentVars {
	env := primaryEnvironment(state)
	result := make([]ProviderComponentVars, 0, len(state.InfrastructureProviders))
	for _, provider := range state.InfrastructureProviders {
		substrateRole, bmcRole, http := providerDispatch(provider)
		item := ProviderComponentVars{
			Name:              provider.Metadata.Name,
			Kind:              v1alpha1.MachineFlavor(provider),
			SubstrateRole:     substrateRole,
			BMCRole:           bmcRole,
			BootArtifactsHttp: http,
		}
		if len(provider.Spec.Hosts) > 0 {
			item.InfrastructureHosts = providerHostVars(provider.Spec.Hosts, env, secretsDir)
		}
		if v1alpha1.ProviderMachineLibvirt(provider) != nil {
			if provider.Spec.Machine.Libvirt.BMCEmulation != nil {
				item.BMC = bmcVars(provider.Spec.Machine.Libvirt.BMCEmulation, providerBMCNodes(provider, state), env, secretsDir)
			}
		}
		result = append(result, item)
	}
	return result
}

func bmcVars(source *v1alpha1.BMCEmulationSpec, nodes []ProviderBMCNodeVars, env *v1alpha1.Environment, secretsDir string) *ProviderBMCVars {
	enabled := false
	if source.Enabled != nil {
		enabled = *source.Enabled
	}
	result := &ProviderBMCVars{
		Enabled:     enabled,
		Protocol:    source.Protocol,
		Emulator:    source.Emulator,
		BindAddress: source.BindAddress,
		Port:        source.Port,
		Nodes:       nodes,
	}
	if source.Auth != nil && source.Auth.CredentialRef.Name != "" {
		result.Auth = &ProviderBMCAuthVars{CredentialRef: resolvedSecretPath(source.Auth.CredentialRef.Name, secretsDir, env)}
	}
	return result
}

func providerBMCNodes(provider v1alpha1.InfrastructureProvider, state v1alpha1.State) []ProviderBMCNodeVars {
	if v1alpha1.ProviderMachineLibvirt(provider) == nil {
		return nil
	}
	var result []ProviderBMCNodeVars
	providers := providerIndex(state.InfrastructureProviders)
	for _, infra := range state.ClusterInfrastructures {
		closure, _ := v1alpha1.BuildProviderClosure(infra, providers)
		if closure.MachineProviderName != provider.Metadata.Name || closure.Machine == nil || closure.Machine.Libvirt == nil {
			continue
		}
		machineNames := sortedKeys(infra.Spec.Machines)
		for _, mname := range machineNames {
			machine := infra.Spec.Machines[mname]
			primary := primaryInterface(machine)
			hostRef := ""
			if machine.Libvirt != nil {
				hostRef = machine.Libvirt.HostRef.Name
			}
			result = append(result, ProviderBMCNodeVars{
				ClusterName: infra.Metadata.Name,
				Name:        mname,
				HostRef:     hostRef,
				IPAddress:   primary.IPAddress,
				MACAddress:  primary.MACAddress,
			})
		}
	}
	return result
}

func libvirtVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, env *v1alpha1.Environment) *LibvirtVars {
	q := v1alpha1.ProviderMachineLibvirt(provider)
	if len(item.Spec.Networks) == 0 || q == nil {
		return nil
	}
	machineNetworks := machineNetworksList(item)
	if len(machineNetworks) == 0 {
		return nil
	}
	machineNetwork := machineNetworks[0]
	if machineNetwork.Libvirt == nil {
		return nil
	}
	restricted := managedProxyIsolationEnabled(item, provider, env)
	proxyPort := 0
	if squid := v1alpha1.ProviderProxySquid(provider); restricted && squid != nil {
		proxyPort = squid.Port
		if proxyPort == 0 {
			proxyPort = v1alpha1.DefaultSquidPort
		}
	}
	return &LibvirtVars{
		Network:                 machineNetwork.Libvirt.LibvirtNetwork,
		Bridge:                  machineNetwork.Libvirt.Bridge,
		EgressRestrictedToProxy: restricted,
		ProxyURL:                managedProxyClientURL(item, provider, env),
		ProxyPort:               proxyPort,
		DNSHosts:                libvirtDNSHosts(machineNetwork, env),
	}
}

func libvirtDNSHosts(machineNetwork MachineNetworkVars, env *v1alpha1.Environment) []LibvirtDNSHost {
	if env == nil || machineNetwork.Gateway == "" {
		return nil
	}
	kind := v1alpha1.OCPInstallKind(*env)
	if kind == "" || kind == v1alpha1.OCPInstallKindConnected {
		return nil
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	host := mirrorRegistryHostname(registries.Mirror.URL)
	if host == "" {
		return nil
	}
	return []LibvirtDNSHost{{
		IP:        machineNetwork.Gateway,
		Hostnames: []string{host},
	}}
}

func providerNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) []ProviderNodeVars {
	roles := nodeRolesByMachineRef(ocp)
	machineNames := sortedKeys(item.Spec.Machines)
	out := make([]ProviderNodeVars, 0, len(machineNames))
	for _, name := range machineNames {
		machine := item.Spec.Machines[name]
		primary := primaryInterface(machine)
		hostRef := ""
		if machine.Libvirt != nil {
			hostRef = machine.Libvirt.HostRef.Name
		}
		var resources VirtualNodeResourceVars
		if machine.Resources != nil {
			resources = VirtualNodeResourceVars{
				CPU:       machine.Resources.CPU,
				MemoryMiB: machine.Resources.MemoryMiB,
				DiskGiB:   machine.Resources.DiskGiB,
			}
		}
		out = append(out, ProviderNodeVars{
			Name:       name,
			Role:       roles[name],
			HostRef:    hostRef,
			IPAddress:  primary.IPAddress,
			MACAddress: primary.MACAddress,
			Resources:  resources,
		})
	}
	return out
}

func nodeRolesByMachineRef(ocp v1alpha1.OCPCluster) map[string]string {
	out := map[string]string{}
	for nodeName, node := range ocp.Spec.Nodes {
		ref := nodeName
		if node.MachineRef != nil && node.MachineRef.Name != "" {
			ref = node.MachineRef.Name
		}
		out[ref] = node.Role
	}
	return out
}
