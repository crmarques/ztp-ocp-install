package render

import (
	"net/netip"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/proxy"
	"github.com/crmarques/gitups/internal/secretref"
)

func resolvedSecretPath(name, secretsDir string, env *v1alpha1.Environment) string {
	return secretref.ResolvePath(name, env, secretsDir)
}

func Vars(state v1alpha1.State, secretsDir string) VarsFile {
	clusters := make([]ClusterVars, 0, len(state.ClusterInfrastructures))
	providers := providerIndex(state.InfrastructureProviders)
	ocpByInfra := ocpByInfrastructure(state.OCPClusters)
	env := primaryEnvironment(state)
	for _, item := range state.ClusterInfrastructures {
		provider := closureProvider(item, providers)
		ocp := ocpByInfra[item.Metadata.Name]
		clusters = append(clusters, clusterVars(item, provider, ocp, env, secretsDir))
	}
	return VarsFile{
		GitupsOCPInstall:       ocpInstallEnvVars(state, env, secretsDir),
		GitupsProviders:        providerComponentVars(state, secretsDir),
		GitupsLoadBalancers:    sharedLoadBalancerVars(state, env),
		GitupsMirrorRegistries: mirrorRegistryRunVars(state, env, secretsDir),
		GitupsForwardProxies:   forwardProxyRunVars(state, env, secretsDir),
		GitupsClusters:         clusters,
		GitupsComponentPins:    ComponentPins(state),
	}
}

func ocpInstallEnvVars(state v1alpha1.State, env *v1alpha1.Environment, secretsDir string) EnvironmentOCPInstallVars {
	kind := v1alpha1.OCPInstallKindConnected
	if env != nil {
		if k := v1alpha1.OCPInstallKind(*env); k != "" {
			kind = k
		}
	}
	result := EnvironmentOCPInstallVars{
		Mode:         kind,
		Disconnected: kind == v1alpha1.OCPInstallKindDisconnected,
	}
	if env == nil {
		return result
	}
	if eff := proxy.Resolve(state, env); eff != nil {
		hostFallback := managedProxyClientHostURLForState(state, env)
		vmFallback := managedProxyClientURLForState(state, env)
		httpProxy, httpsProxy := effectiveProxyURLs(eff, hostFallback)
		vmHTTP, vmHTTPS := effectiveProxyURLs(eff, vmFallback)
		if httpProxy != "" || httpsProxy != "" || vmHTTP != "" || vmHTTPS != "" || len(eff.NoProxy) > 0 || eff.Auth.Name != "" {
			result.Proxy = &ProxyVars{
				HTTP:         httpProxy,
				HTTPS:        httpsProxy,
				VMHTTP:       vmHTTP,
				VMHTTPS:      vmHTTPS,
				NoProxy:      append([]string(nil), eff.NoProxy...),
				ProxyAuthRef: resolvedSecretPath(eff.Auth.Name, secretsDir, env),
			}
		}
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return result
	}
	result.Registry = &MirrorRegistryVars{
		URL:            registries.Mirror.URL,
		Host:           mirrorRegistryHostname(registries.Mirror.URL),
		CredentialsRef: resolvedSecretPath(registries.Mirror.CredentialsRef.Name, secretsDir, env),
	}
	return result
}

func effectiveProxyURLs(eff *proxy.Effective, fallbackURL string) (string, string) {
	if eff == nil {
		return "", ""
	}
	httpProxy := eff.HTTP
	if httpProxy == "" {
		httpProxy = fallbackURL
	}
	httpsProxy := eff.HTTPS
	if httpsProxy == "" {
		httpsProxy = fallbackURL
	}
	return httpProxy, httpsProxy
}

func forwardProxyRunVars(state v1alpha1.State, env *v1alpha1.Environment, secretsDir string) []ForwardProxyRunVars {
	if env == nil {
		return nil
	}
	eff := proxy.Resolve(state, env)
	if eff == nil {
		return nil
	}
	imageRef := componentImageURLs(env, v1alpha1.ComponentCategoryProxy, v1alpha1.ComponentTypeSquid)
	// Host-facing URL: proxy_squid pins this hostname in /etc/hosts and it
	// must match what host_proxy writes into HTTP(S)_PROXY on every host.
	fallbackURL := managedProxyClientHostURLForState(state, env)
	clientURL := eff.HTTP
	if clientURL == "" {
		clientURL = eff.HTTPS
	}
	if clientURL == "" {
		clientURL = fallbackURL
	}
	providers := append([]v1alpha1.InfrastructureProvider(nil), state.InfrastructureProviders...)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Metadata.Name < providers[j].Metadata.Name
	})
	result := make([]ForwardProxyRunVars, 0, len(providers))
	for _, provider := range providers {
		squid := v1alpha1.ProviderProxySquid(provider)
		if squid == nil {
			continue
		}
		hostAddress := ""
		if host, ok := provider.Spec.Hosts[squid.HostRef.Name]; ok && host.SSH != nil {
			hostAddress = host.SSH.Address
		}
		port := squid.Port
		if port == 0 {
			port = v1alpha1.DefaultSquidPort
		}
		runtime := squid.Runtime
		if runtime == "" {
			runtime = v1alpha1.ContainerRuntimePodman
		}
		result = append(result, ForwardProxyRunVars{
			Name:                  provider.Metadata.Name,
			ProviderRef:           provider.Metadata.Name,
			ProviderHostRef:       squid.HostRef.Name,
			URL:                   clientURL,
			Host:                  hostAddress,
			Port:                  port,
			DataDir:               squid.DataDir,
			Runtime:               runtime,
			CredentialsSecretName: resolvedSecretPath(eff.Auth.Name, secretsDir, env),
			Image:                 imageRef,
		})
	}
	return result
}

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

func ocpReleaseVars(ocp v1alpha1.OCPCluster) OCPReleaseVars {
	if ocp.Spec.Install.Release == nil {
		return OCPReleaseVars{}
	}
	return OCPReleaseVars{
		Channel: ocp.Spec.Install.Release.Channel,
		Version: ocp.Spec.Install.Release.Version,
	}
}

func ocpInstallVars(ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) OCPInstallVars {
	return OCPInstallVars{
		Method:                   ocp.Spec.Install.Method,
		BaseDomain:               ocp.Spec.Install.BaseDomain,
		PullSecretRef:            resolvedSecretPath(ocp.Spec.Install.PullSecretRef.Name, secretsDir, env),
		SSHKeyRef:                resolvedSecretPath(ocp.Spec.Install.SSHKeyRef.Name, secretsDir, env),
		ReleaseImageOverride:     releaseImageOverride(ocp),
		AdditionalTrustBundleRef: resolvedSecretPath(ocp.Spec.Install.AdditionalTrustBundleRef.Name, secretsDir, env),
		GeneratedSecrets:         generatedSecretVarsFromEnv(env, secretsDir),
		LocalRegistry:            localRegistryVars(env, ocp, secretsDir),
	}
}

func releaseImageOverride(ocp v1alpha1.OCPCluster) string {
	if ocp.Spec.Install.Release == nil || ocp.Spec.Install.Release.Version == "" {
		return ""
	}
	for _, source := range ocp.Spec.Install.ImageDigestSources {
		if source.Source != v1alpha1.OCPReleaseSourceQuayOCPRelease || len(source.Mirrors) == 0 {
			continue
		}
		return strings.TrimRight(source.Mirrors[0], "/") + ":" + ocp.Spec.Install.Release.Version + "-x86_64"
	}
	return ""
}

func localRegistryVars(env *v1alpha1.Environment, ocp v1alpha1.OCPCluster, secretsDir string) *LocalRegistryVars {
	if env == nil {
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
	return &LocalRegistryVars{
		Registry: MirrorRegistryVars{
			URL:            registries.Mirror.URL,
			Host:           mirrorRegistryHostname(registries.Mirror.URL),
			CredentialsRef: resolvedSecretPath(registries.Mirror.CredentialsRef.Name, secretsDir, env),
			TrustBundleRef: resolvedSecretPath(ocp.Spec.Install.AdditionalTrustBundleRef.Name, secretsDir, env),
		},
	}
}

func mirrorRegistryHostname(url string) string {
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		return url[:idx]
	}
	return url
}

func generatedSecretVarsFromEnv(env *v1alpha1.Environment, secretsDir string) []GeneratedSecretVars {
	if env == nil {
		return nil
	}
	names := make([]string, 0, len(env.Spec.Secrets))
	for name, secret := range env.Spec.Secrets {
		if secret.Generated == nil || secret.Generated.SelfSignedCertificate == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]GeneratedSecretVars, 0, len(names))
	for _, name := range names {
		cert := env.Spec.Secrets[name].Generated.SelfSignedCertificate
		result = append(result, GeneratedSecretVars{
			Name:                  name,
			Path:                  filepath.Join(secretsDir, name),
			Type:                  v1alpha1.GeneratedSecretSelfSigned,
			SelfSignedCertificate: selfSignedCertificateVars(*cert),
		})
	}
	return result
}

func selfSignedCertificateVars(item v1alpha1.SelfSignedCertificateSpec) *SelfSignedCertificateVars {
	dnsNames := append([]string(nil), item.DNSNames...)
	ipAddresses := append([]string(nil), item.IPAddresses...)
	if len(dnsNames) == 0 && len(ipAddresses) == 0 {
		if _, err := netip.ParseAddr(item.CommonName); err == nil {
			ipAddresses = append(ipAddresses, item.CommonName)
		} else if item.CommonName != "" {
			dnsNames = append(dnsNames, item.CommonName)
		}
	}
	return &SelfSignedCertificateVars{
		CommonName:     item.CommonName,
		DNSNames:       dnsNames,
		IPAddresses:    ipAddresses,
		ValidityDays:   item.ValidityDays,
		SubjectAltName: subjectAltName(dnsNames, ipAddresses),
	}
}

func subjectAltName(dnsNames, ipAddresses []string) string {
	entries := make([]string, 0, len(dnsNames)+len(ipAddresses))
	for _, name := range dnsNames {
		entries = append(entries, "DNS:"+name)
	}
	for _, address := range ipAddresses {
		entries = append(entries, "IP:"+address)
	}
	return strings.Join(entries, ",")
}

func ocpInstallerVars(clusterName string) OCPInstallerVars {
	dir := installerRelativeDir(clusterName)
	return OCPInstallerVars{
		RelativeDir:               dir,
		RelativeInstallConfigPath: dir + "/install-config.yaml",
		RelativeAgentConfigPath:   dir + "/agent-config.yaml",
	}
}

func installerRelativeDir(clusterName string) string {
	return "clusters-bootstrap.git/" + clusterName + "/openshift"
}

func ocpClusterNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) []OCPClusterNodeVars {
	nodeNames := sortedKeys(ocp.Spec.Nodes)
	result := make([]OCPClusterNodeVars, 0, len(nodeNames))
	for _, name := range nodeNames {
		node := ocp.Spec.Nodes[name]
		machineRef := name
		if node.MachineRef != nil && node.MachineRef.Name != "" {
			machineRef = node.MachineRef.Name
		}
		entry := OCPClusterNodeVars{
			Name:       name,
			MachineRef: machineRef,
			Role:       node.Role,
		}
		if machine, ok := item.Spec.Machines[machineRef]; ok {
			if machine.Libvirt != nil {
				entry.HostRef = machine.Libvirt.HostRef.Name
			}
			if machine.BareMetal != nil && machine.BareMetal.BMC != nil {
				entry.BareMetal = &MachineBMCVars{
					Address:                        machine.BareMetal.BMC.Address,
					Port:                           machine.BareMetal.BMC.Port,
					Protocol:                       machine.BareMetal.BMC.Protocol,
					CredentialRef:                  resolvedSecretPath(machine.BareMetal.BMC.CredentialRef.Name, secretsDir, env),
					DisableCertificateVerification: machine.BareMetal.BMC.DisableCertificateVerification,
					BootMACAddress:                 machine.BareMetal.BootMACAddress,
				}
			}
			primary := primaryInterface(machine)
			entry.IPAddress = primary.IPAddress
			entry.MACAddress = primary.MACAddress
		}
		result = append(result, entry)
	}
	return result
}

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

func sharedLoadBalancerVars(state v1alpha1.State, env *v1alpha1.Environment) []SharedLoadBalancerVars {
	imageRef := componentImageURLs(env, v1alpha1.ComponentCategoryLoadBalancer, v1alpha1.ComponentTypeHAProxy)
	ocpByInfra := ocpByInfrastructure(state.OCPClusters)
	providers := providerIndex(state.InfrastructureProviders)
	var result []SharedLoadBalancerVars
	for _, infra := range state.ClusterInfrastructures {
		ocp := ocpByInfra[infra.Metadata.Name]
		provider := closureProvider(infra, providers)
		if provider.Spec.LoadBalancer == nil || provider.Spec.LoadBalancer.HAProxy == nil {
			continue
		}
		hostRef := provider.Spec.LoadBalancer.HAProxy.HostRef.Name
		runtime := provider.Spec.LoadBalancer.HAProxy.Runtime
		if runtime == "" {
			runtime = v1alpha1.ContainerRuntimePodman
		}
		lbNames := sortedKeys(infra.Spec.LoadBalancers)
		for _, lbName := range lbNames {
			lb := infra.Spec.LoadBalancers[lbName]
			item := SharedLoadBalancerVars{
				Name:        lbName,
				ClusterName: infra.Metadata.Name,
				ProviderRef: provider.Metadata.Name,
				Image:       imageRef,
				Runtime:     runtime,
				Placement:   LoadBalancerPlacementVars{ProviderHostRef: hostRef},
				Frontends:   make([]SharedLoadBalancerFrontendVars, 0, len(lb.Endpoints)),
			}
			for _, ep := range lb.Endpoints {
				item.Frontends = append(item.Frontends, frontendForEndpoint(infra, ocp, provider, lbName, ep))
			}
			result = append(result, item)
		}
	}
	return result
}

func frontendForEndpoint(infra v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, provider v1alpha1.InfrastructureProvider, lbName, ep string) SharedLoadBalancerFrontendVars {
	ports := []LoadBalancerPortVars{}
	for _, p := range v1alpha1.StandardLoadBalancerPorts(ep) {
		ports = append(ports, LoadBalancerPortVars{ListenPort: p[0], TargetPort: p[1]})
	}
	frontend := SharedLoadBalancerFrontendVars{
		Name:        ep,
		EndpointRef: ep,
		Ports:       ports,
		Backend:     LoadBalancerBackendVars{NodeRole: v1alpha1.StandardEndpointBackendRole(ep)},
	}
	bindAddress := endpointAddressFromCI(infra, ep)
	frontend.Bindings = append(frontend.Bindings, LoadBalancerBindingVars{
		ClusterName: infra.Metadata.Name,
		OCPName:     ocp.Metadata.Name,
		BindAddress: bindAddress,
		Backends:    backendNodes(infra, ocp, frontend.Backend.NodeRole),
	})
	_ = lbName
	_ = provider
	return frontend
}

func endpointAddressFromCI(infra v1alpha1.ClusterInfrastructure, ep string) string {
	switch ep {
	case v1alpha1.EndpointAPI:
		if infra.Spec.Endpoints.API != nil {
			return infra.Spec.Endpoints.API.Address
		}
	case v1alpha1.EndpointAPIInt:
		if infra.Spec.Endpoints.APIInt != nil {
			return infra.Spec.Endpoints.APIInt.Address
		}
	case v1alpha1.EndpointIngress:
		if infra.Spec.Endpoints.Ingress != nil {
			return infra.Spec.Endpoints.Ingress.Address
		}
	}
	return ""
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

func backendNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, role string) []LoadBalancerBackendNode {
	roleByMachineRef := nodeRolesByMachineRef(ocp)
	var workerRefs, controlPlaneRefs []string
	for ref, r := range roleByMachineRef {
		switch r {
		case v1alpha1.NodeRoleWorker:
			workerRefs = append(workerRefs, ref)
		case v1alpha1.NodeRoleControlPlane:
			controlPlaneRefs = append(controlPlaneRefs, ref)
		}
	}
	sort.Strings(workerRefs)
	sort.Strings(controlPlaneRefs)
	var selected []string
	switch role {
	case v1alpha1.NodeRoleWorker:
		selected = workerRefs
	case v1alpha1.NodeRoleControlPlane:
		selected = controlPlaneRefs
	default:
		if len(workerRefs) > 0 {
			selected = workerRefs
		} else {
			selected = controlPlaneRefs
		}
	}
	selectedSet := map[string]bool{}
	for _, ref := range selected {
		selectedSet[ref] = true
	}
	machineNames := sortedKeys(item.Spec.Machines)
	var out []LoadBalancerBackendNode
	for _, name := range machineNames {
		if !selectedSet[name] {
			continue
		}
		machine := item.Spec.Machines[name]
		out = append(out, LoadBalancerBackendNode{
			Name:      name,
			Address:   primaryInterface(machine).IPAddress,
			Role:      roleByMachineRef[name],
			TargetRef: name,
		})
	}
	return out
}

func primaryInterface(machine v1alpha1.MachineSpec) v1alpha1.MachineInterfaceSpec {
	if len(machine.Interfaces) == 0 {
		return v1alpha1.MachineInterfaceSpec{}
	}
	for _, iface := range machine.Interfaces {
		if iface.Primary != nil && *iface.Primary {
			return iface
		}
	}
	keys := sortedKeys(machine.Interfaces)
	return machine.Interfaces[keys[0]]
}

func closureProvider(ci v1alpha1.ClusterInfrastructure, providers map[string]v1alpha1.InfrastructureProvider) v1alpha1.InfrastructureProvider {
	closure, _ := v1alpha1.BuildProviderClosure(ci, providers)
	name := closure.LoadBalancerProviderName
	if name == "" {
		name = closure.MachineProviderName
	}
	if name == "" {
		name = closure.NameResolutionProviderName
	}
	if name == "" {
		name = closure.RegistryProviderName
	}
	if name == "" {
		name = closure.ProxyProviderName
	}
	if name == "" {
		name = ci.Metadata.Name
	}
	return v1alpha1.InfrastructureProvider{
		Metadata: v1alpha1.Metadata{Name: name},
		Spec: v1alpha1.InfrastructureProviderSpec{
			Hosts:          closure.Hosts,
			Machine:        closure.Machine,
			LoadBalancer:   closure.LoadBalancer,
			NameResolution: closure.NameResolution,
			Registry:       closure.Registry,
			Proxy:          closure.Proxy,
		},
	}
}

func mirrorRegistryRunVars(state v1alpha1.State, env *v1alpha1.Environment, secretsDir string) []MirrorRegistryRunVars {
	if env == nil {
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
	mirror := registries.Mirror
	host := mirrorRegistryHostname(mirror.URL)
	urlPort := mirrorURLPortRender(mirror.URL)
	credPath := resolvedSecretPath(mirror.CredentialsRef.Name, secretsDir, env)
	var caCertPath, caKeyPath string
	if mirror.TrustBundleRef.Name != "" {
		caCertPath = resolvedSecretPath(mirror.TrustBundleRef.Name, secretsDir, env)
		if secret, ok := env.Spec.Secrets[mirror.TrustBundleRef.Name]; ok && secret.Generated != nil && secret.Generated.SelfSignedCertificate != nil {
			caKeyPath = filepath.Join(secretsDir, mirror.TrustBundleRef.Name+".key")
		}
	}
	regImage := componentImageURLs(env, v1alpha1.ComponentCategoryRegistry, v1alpha1.ComponentTypeMirrorRegistry)
	mirrorSet := buildMirrorSet(state, env, mirror.URL, regImage)
	var result []MirrorRegistryRunVars
	providers := append([]v1alpha1.InfrastructureProvider(nil), state.InfrastructureProviders...)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Metadata.Name < providers[j].Metadata.Name
	})
	for _, p := range providers {
		mr := v1alpha1.ProviderMirrorRegistry(p)
		if mr == nil {
			continue
		}
		port := mr.Port
		if port == 0 {
			if urlPort > 0 {
				port = urlPort
			} else {
				port = v1alpha1.DefaultMirrorRegistryPort
			}
		}
		runtime := mr.Runtime
		if runtime == "" {
			runtime = v1alpha1.ContainerRuntimePodman
		}
		result = append(result, MirrorRegistryRunVars{
			Name:                      p.Metadata.Name,
			ProviderRef:               p.Metadata.Name,
			ProviderHostRef:           mr.HostRef.Name,
			URL:                       mirror.URL,
			Host:                      host,
			Port:                      port,
			DataDir:                   mr.DataDir,
			Runtime:                   runtime,
			CredentialsSecretName:     credPath,
			TrustBundleCertSecretName: caCertPath,
			TrustBundleKeySecretName:  caKeyPath,
			Image:                     regImage,
			MirrorSet:                 mirrorSet,
		})
	}
	return result
}

func buildMirrorSet(state v1alpha1.State, env *v1alpha1.Environment, mirrorURL string, regImage ComponentImageURLs) []MirrorImageRef {
	var out []MirrorImageRef
	mirrorURL = strings.TrimRight(mirrorURL, "/")
	seenVersions := map[string]bool{}
	for _, ocp := range state.OCPClusters {
		if ocp.Spec.Install.Release == nil || ocp.Spec.Install.Release.Version == "" {
			continue
		}
		v := ocp.Spec.Install.Release.Version
		if seenVersions[v] {
			continue
		}
		seenVersions[v] = true
		out = append(out, MirrorImageRef{
			Kind:   MirrorImageKindReleasePayload,
			Public: v1alpha1.OCPReleaseSourceQuayOCPRelease + ":" + v + "-x86_64",
			Local:  mirrorURL + "/" + v1alpha1.DefaultMirroredReleasePath + ":" + v + "-x86_64",
		})
	}
	if env != nil {
		categories := sortedKeys(env.Spec.ComponentImages)
		for _, cat := range categories {
			types := env.Spec.ComponentImages[cat]
			for _, typ := range sortedKeys(types) {
				img := types[typ]
				if img.Public == "" || img.Local == "" {
					continue
				}
				if cat == v1alpha1.ComponentCategoryRegistry && typ == v1alpha1.ComponentTypeMirrorRegistry {
					continue
				}
				out = append(out, MirrorImageRef{
					Kind:   MirrorImageKindComponent,
					Public: img.Public,
					Local:  img.Local,
				})
			}
		}
	}
	if regImage.Local != "" {
		public := regImage.Public
		if public == "" {
			public = v1alpha1.DefaultMirrorRegistryImageRef
		}
		out = append(out, MirrorImageRef{
			Kind:   MirrorImageKindRegistryServer,
			Public: public,
			Local:  regImage.Local,
		})
	}
	return out
}

func mirrorURLPortRender(u string) int {
	host := u
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	idx := strings.LastIndex(host, ":")
	if idx < 0 {
		return 0
	}
	port := 0
	for _, ch := range host[idx+1:] {
		if ch < '0' || ch > '9' {
			return 0
		}
		port = port*10 + int(ch-'0')
	}
	return port
}

func providerIndex(items []v1alpha1.InfrastructureProvider) map[string]v1alpha1.InfrastructureProvider {
	result := map[string]v1alpha1.InfrastructureProvider{}
	for _, item := range items {
		result[item.Metadata.Name] = item
	}
	return result
}

func ocpByInfrastructure(items []v1alpha1.OCPCluster) map[string]v1alpha1.OCPCluster {
	result := map[string]v1alpha1.OCPCluster{}
	for _, item := range items {
		result[item.Spec.InfrastructureRef.Name] = item
	}
	return result
}

func primaryEnvironment(state v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

func componentImageURLs(env *v1alpha1.Environment, category, typ string) ComponentImageURLs {
	if env != nil {
		if types, ok := env.Spec.ComponentImages[category]; ok {
			if image, ok := types[typ]; ok {
				return ComponentImageURLs{Local: image.Local, Public: image.Public}
			}
		}
	}
	if category == v1alpha1.ComponentCategoryLoadBalancer && typ == v1alpha1.ComponentTypeHAProxy {
		return ComponentImageURLs{Public: v1alpha1.DefaultHAProxyImageRef}
	}
	if category == v1alpha1.ComponentCategoryRegistry && typ == v1alpha1.ComponentTypeMirrorRegistry {
		return ComponentImageURLs{Public: v1alpha1.DefaultMirrorRegistryImageRef}
	}
	if category == v1alpha1.ComponentCategoryProxy && typ == v1alpha1.ComponentTypeSquid {
		return ComponentImageURLs{Public: v1alpha1.DefaultSquidImageRef}
	}
	return ComponentImageURLs{}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
