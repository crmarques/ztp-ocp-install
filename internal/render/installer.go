package render

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type InstallerAsset struct {
	ClusterName       string
	Method            string
	Dir               string
	InstallConfigPath string
	AgentConfigPath   string
}

func InstallerAssets(stateDir string, state v1alpha1.State) []InstallerAsset {
	assets := make([]InstallerAsset, 0, len(state.OCPClusters))
	for _, ocp := range state.OCPClusters {
		dir := filepath.Join(stateDir, "clusters", ocp.Metadata.Name, "installer")
		assets = append(assets, InstallerAsset{
			ClusterName:       ocp.Metadata.Name,
			Method:            ocp.Spec.Install.Method,
			Dir:               dir,
			InstallConfigPath: filepath.Join(dir, "install-config.yaml"),
			AgentConfigPath:   filepath.Join(dir, "agent-config.yaml"),
		})
	}
	return assets
}

func InstallerConfig(state v1alpha1.State, ocp v1alpha1.OCPCluster) (map[string]any, error) {
	infra, err := clusterInfrastructureForOCP(state, ocp)
	if err != nil {
		return nil, err
	}
	provider, err := providerForInfrastructure(state, infra)
	if err != nil {
		return nil, err
	}
	base := map[string]any{
		"apiVersion": "v1",
		"baseDomain": ocp.Spec.Install.BaseDomain,
		"metadata": map[string]any{
			"name": ocp.Metadata.Name,
		},
		"compute": []any{
			map[string]any{
				"name":     "worker",
				"replicas": nodeRoleCount(ocp, v1alpha1.NodeRoleWorker),
			},
		},
		"controlPlane": map[string]any{
			"name":     "master",
			"replicas": nodeRoleCount(ocp, v1alpha1.NodeRoleControlPlane),
		},
		"networking": networkingConfig(infra, ocp),
		"platform":   platformConfig(provider, infra, ocp),
		"pullSecret": pullSecretPlaceholder(ocp.Spec.Install.PullSecretRef.Name),
		"sshKey":     secretRefPlaceholder("ssh-key", ocp.Spec.Install.SSHKeyRef.Name),
	}
	if ocp.Spec.Install.AdditionalTrustBundleRef.Name != "" {
		base["additionalTrustBundle"] = secretRefPlaceholder("trust-bundle", ocp.Spec.Install.AdditionalTrustBundleRef.Name)
	}
	if mirrors := imageDigestSourcesConfig(ocp.Spec.Install.ImageDigestSources); len(mirrors) > 0 {
		base["imageDigestSources"] = mirrors
	}
	if proxy := installerProxyConfig(primaryEnvironment(state)); proxy != nil {
		base["proxy"] = proxy
	}
	return mergeYAMLMaps(base, ocp.Spec.Install.InstallConfigOverrides), nil
}

// installerProxyConfig projects the Environment OCP install proxy onto an
// install-config.yaml `proxy:` map. OpenShift agent installs read this to
// route bootstrap, MCO, and registry-pull traffic through the proxy while
// preserving direct access to noProxy ranges.
func installerProxyConfig(env *v1alpha1.Environment) map[string]any {
	if env == nil {
		return nil
	}
	proxy := v1alpha1.OCPInstallProxyOf(*env)
	if proxy == nil || (proxy.HTTPProxy == "" && proxy.HTTPSProxy == "" && len(proxy.NoProxy) == 0) {
		return nil
	}
	out := map[string]any{}
	if proxy.HTTPProxy != "" {
		out["httpProxy"] = proxy.HTTPProxy
	}
	if proxy.HTTPSProxy != "" {
		out["httpsProxy"] = proxy.HTTPSProxy
	}
	if len(proxy.NoProxy) > 0 {
		out["noProxy"] = strings.Join(proxy.NoProxy, ",")
	}
	return out
}

func imageDigestSourcesConfig(sources []v1alpha1.ImageDigestSource) []any {
	if len(sources) == 0 {
		return nil
	}
	result := make([]any, 0, len(sources))
	for _, source := range sources {
		mirrors := make([]any, 0, len(source.Mirrors))
		for _, mirror := range source.Mirrors {
			mirrors = append(mirrors, mirror)
		}
		entry := map[string]any{
			"source":  source.Source,
			"mirrors": mirrors,
		}
		if source.SourcePolicy != "" {
			entry["sourcePolicy"] = source.SourcePolicy
		}
		result = append(result, entry)
	}
	return result
}

func AgentConfig(state v1alpha1.State, ocp v1alpha1.OCPCluster) (map[string]any, error) {
	infra, err := clusterInfrastructureForOCP(state, ocp)
	if err != nil {
		return nil, err
	}
	provider, err := providerForInfrastructure(state, infra)
	if err != nil {
		return nil, err
	}
	hosts, rendezvousIP := agentHosts(infra, ocp)
	base := map[string]any{
		"apiVersion":   "v1beta1",
		"kind":         "AgentConfig",
		"metadata":     map[string]any{"name": ocp.Metadata.Name},
		"rendezvousIP": rendezvousIP,
		"hosts":        hosts,
	}
	for key, value := range disconnectedBootArtifactsConfig(infra, provider, primaryEnvironment(state)) {
		base[key] = value
	}
	return mergeYAMLMaps(base, ocp.Spec.Install.AgentConfigOverrides), nil
}

// disconnectedBootArtifactsConfig wires the agent-installer minimal-ISO flow
// when the environment is disconnected and the provider exposes a BMC
// emulator. The boot-artifacts HTTP server runs on bmc.port+2 against the
// machine network gateway (a libvirt-managed bridge address). Forcing
// minimalISO=true and a provider-local bootArtifactsBaseURL prevents
// `openshift-install agent create image` from reaching public RHCOS URLs.
func disconnectedBootArtifactsConfig(infra v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, env *v1alpha1.Environment) map[string]any {
	if env == nil || v1alpha1.OCPInstallKind(*env) != v1alpha1.OCPInstallKindDisconnected {
		return nil
	}
	if v1alpha1.ProviderMachineLibvirt(provider) == nil || provider.Spec.Machine.Libvirt.BMCEmulation == nil || provider.Spec.Machine.Libvirt.BMCEmulation.Port == 0 {
		return nil
	}
	machineNetworks := machineNetworksList(infra)
	if len(machineNetworks) == 0 || machineNetworks[0].Gateway == "" {
		return nil
	}
	return map[string]any{
		"minimalISO":           true,
		"bootArtifactsBaseURL": fmt.Sprintf("http://%s:%d/", machineNetworks[0].Gateway, provider.Spec.Machine.Libvirt.BMCEmulation.Port+2),
	}
}

func clusterInfrastructureForOCP(state v1alpha1.State, ocp v1alpha1.OCPCluster) (v1alpha1.ClusterInfrastructure, error) {
	for _, infra := range state.ClusterInfrastructures {
		if infra.Metadata.Name == ocp.Spec.InfrastructureRef.Name {
			return infra, nil
		}
	}
	return v1alpha1.ClusterInfrastructure{}, fmt.Errorf("%s: infrastructureRef %q not found", ocp.Metadata.Name, ocp.Spec.InfrastructureRef.Name)
}

func providerForInfrastructure(state v1alpha1.State, infra v1alpha1.ClusterInfrastructure) (v1alpha1.InfrastructureProvider, error) {
	for _, provider := range state.InfrastructureProviders {
		if provider.Metadata.Name == infra.Spec.ProviderRef.Name {
			return provider, nil
		}
	}
	return v1alpha1.InfrastructureProvider{}, fmt.Errorf("%s: providerRef %q not found", infra.Metadata.Name, infra.Spec.ProviderRef.Name)
}

func networkingConfig(infra v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) map[string]any {
	result := map[string]any{
		"machineNetwork": machineNetworkConfig(infra),
	}
	if ocp.Spec.Networking == nil {
		return result
	}
	if ocp.Spec.Networking.NetworkType != "" {
		result["networkType"] = ocp.Spec.Networking.NetworkType
	}
	if len(ocp.Spec.Networking.ClusterNetwork) > 0 {
		result["clusterNetwork"] = clusterNetworkConfig(ocp.Spec.Networking.ClusterNetwork)
	}
	if len(ocp.Spec.Networking.ServiceNetwork) > 0 {
		result["serviceNetwork"] = serviceNetworkConfig(ocp.Spec.Networking.ServiceNetwork)
	}
	return result
}

func machineNetworkConfig(infra v1alpha1.ClusterInfrastructure) []any {
	names := sortedKeys(infra.Spec.Networks)
	result := make([]any, 0, len(names))
	for _, name := range names {
		result = append(result, map[string]any{"cidr": infra.Spec.Networks[name].CIDR})
	}
	return result
}

func clusterNetworkConfig(networks []v1alpha1.OCPClusterNetworkCIDR) []any {
	result := make([]any, 0, len(networks))
	for _, network := range networks {
		entry := map[string]any{"cidr": network.CIDR}
		if network.HostPrefix > 0 {
			entry["hostPrefix"] = network.HostPrefix
		}
		result = append(result, entry)
	}
	return result
}

func serviceNetworkConfig(networks []string) []any {
	result := make([]any, 0, len(networks))
	for _, cidr := range networks {
		result = append(result, cidr)
	}
	return result
}

func platformConfig(provider v1alpha1.InfrastructureProvider, infra v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) map[string]any {
	if ocp.Spec.Topology == v1alpha1.OCPTopologySingleNode {
		return map[string]any{"none": map[string]any{}}
	}
	apiAddress := ""
	ingressAddress := ""
	if infra.Spec.Endpoints.API != nil {
		apiAddress = infra.Spec.Endpoints.API.Address
	}
	if infra.Spec.Endpoints.Ingress != nil {
		ingressAddress = infra.Spec.Endpoints.Ingress.Address
	}
	switch v1alpha1.MachineFlavor(provider) {
	case v1alpha1.MachineFlavorLibvirt, v1alpha1.MachineFlavorBaremetal:
		return map[string]any{
			"baremetal": map[string]any{
				"apiVIPs":     []any{apiAddress},
				"ingressVIPs": []any{ingressAddress},
			},
		}
	case v1alpha1.MachineFlavorVsphere:
		return map[string]any{
			"vsphere": map[string]any{
				"apiVIPs":     []any{apiAddress},
				"ingressVIPs": []any{ingressAddress},
			},
		}
	default:
		return map[string]any{
			"none": map[string]any{},
		}
	}
}

func agentHosts(infra v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) ([]any, string) {
	nodeNames := sortedKeys(ocp.Spec.Nodes)
	hosts := make([]any, 0, len(nodeNames))
	rendezvousIP := ""
	for _, nodeName := range nodeNames {
		node := ocp.Spec.Nodes[nodeName]
		machineRefName := nodeName
		if node.MachineRef != nil && node.MachineRef.Name != "" {
			machineRefName = node.MachineRef.Name
		}
		machine := infra.Spec.Machines[machineRefName]
		host := map[string]any{
			"hostname":   nodeName,
			"role":       installerNodeRole(node.Role),
			"interfaces": agentHostInterfaces(machine),
		}
		if hints := rootDeviceHintsConfig(machine.RootDeviceHints); len(hints) > 0 {
			host["rootDeviceHints"] = hints
		}
		if networkConfig := agentNetworkConfig(machine, infra.Spec.Networks); len(networkConfig) > 0 {
			host["networkConfig"] = networkConfig
		}
		if rendezvousIP == "" && node.Role == v1alpha1.NodeRoleControlPlane {
			rendezvousIP = primaryInterface(machine).IPAddress
		}
		hosts = append(hosts, host)
	}
	return hosts, rendezvousIP
}

func agentHostInterfaces(machine v1alpha1.MachineSpec) []any {
	names := sortedKeys(machine.Interfaces)
	result := make([]any, 0, len(names))
	for _, name := range names {
		iface := machine.Interfaces[name]
		result = append(result, map[string]any{
			"name":       name,
			"macAddress": iface.MACAddress,
		})
	}
	return result
}

func agentNetworkConfig(machine v1alpha1.MachineSpec, networks map[string]v1alpha1.MachineNetworkSpec) map[string]any {
	if len(machine.Interfaces) == 0 {
		return nil
	}
	ifaceNames := sortedKeys(machine.Interfaces)
	interfaces := make([]any, 0, len(ifaceNames))
	var dnsServers []string
	var routes []any
	seenServers := map[string]bool{}
	for _, ifaceName := range ifaceNames {
		iface := machine.Interfaces[ifaceName]
		network, ok := networks[iface.NetworkRef.Name]
		if !ok {
			continue
		}
		prefix, err := netip.ParsePrefix(network.CIDR)
		if err != nil {
			continue
		}
		address, err := netip.ParseAddr(iface.IPAddress)
		if err != nil {
			continue
		}
		entry := map[string]any{
			"name":  ifaceName,
			"type":  "ethernet",
			"state": "up",
			"ipv4": map[string]any{
				"enabled": true,
				"dhcp":    false,
				"address": []any{
					map[string]any{
						"ip":            address.String(),
						"prefix-length": prefix.Bits(),
					},
				},
			},
			"ipv6": map[string]any{"enabled": false},
		}
		if iface.MACAddress != "" {
			entry["mac-address"] = iface.MACAddress
		}
		interfaces = append(interfaces, entry)
		if network.Gateway != "" {
			routes = append(routes, map[string]any{
				"destination":        "0.0.0.0/0",
				"next-hop-address":   network.Gateway,
				"next-hop-interface": ifaceName,
				"table-id":           254,
			})
		}
		for _, server := range network.DNSServers {
			if seenServers[server] {
				continue
			}
			seenServers[server] = true
			dnsServers = append(dnsServers, server)
		}
	}
	if len(interfaces) == 0 {
		return nil
	}
	result := map[string]any{"interfaces": interfaces}
	if len(dnsServers) > 0 {
		serverList := make([]any, 0, len(dnsServers))
		for _, server := range dnsServers {
			serverList = append(serverList, server)
		}
		result["dns-resolver"] = map[string]any{
			"config": map[string]any{
				"server": serverList,
			},
		}
	}
	if len(routes) > 0 {
		result["routes"] = map[string]any{"config": routes}
	}
	return result
}

func rootDeviceHintsConfig(hints *v1alpha1.RootDeviceHintsSpec) map[string]any {
	if hints == nil {
		return nil
	}
	result := map[string]any{}
	if hints.DeviceName != "" {
		result["deviceName"] = hints.DeviceName
	}
	if hints.HCTL != "" {
		result["hctl"] = hints.HCTL
	}
	if hints.Model != "" {
		result["model"] = hints.Model
	}
	if hints.Vendor != "" {
		result["vendor"] = hints.Vendor
	}
	if hints.SerialNumber != "" {
		result["serialNumber"] = hints.SerialNumber
	}
	if hints.MinSizeGigabytes > 0 {
		result["minSizeGigabytes"] = hints.MinSizeGigabytes
	}
	if hints.WWN != "" {
		result["wwn"] = hints.WWN
	}
	if hints.Rotational != nil {
		result["rotational"] = *hints.Rotational
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func installerNodeRole(role string) string {
	if role == v1alpha1.NodeRoleWorker {
		return "worker"
	}
	return "master"
}

func nodeRoleCount(ocp v1alpha1.OCPCluster, role string) int {
	count := 0
	for _, node := range ocp.Spec.Nodes {
		if node.Role == role {
			count++
		}
	}
	return count
}

func pullSecretPlaceholder(ref string) string {
	data, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			"gitups-secret-ref:" + ref: map[string]any{},
		},
	})
	if err != nil {
		return "{}"
	}
	return string(data)
}

func secretRefPlaceholder(kind string, ref string) string {
	return fmt.Sprintf("<gitups-%s-ref:%s>", kind, ref)
}

func mergeYAMLMaps(base map[string]any, override map[string]any) map[string]any {
	result := cloneYAMLMap(base)
	for key, overrideValue := range override {
		if baseMap, ok := result[key].(map[string]any); ok {
			if overrideMap, ok := overrideValue.(map[string]any); ok {
				result[key] = mergeYAMLMaps(baseMap, overrideMap)
				continue
			}
		}
		result[key] = cloneYAMLValue(overrideValue)
	}
	return result
}

func cloneYAMLMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, child := range value {
		result[key] = cloneYAMLValue(child)
	}
	return result
}

func cloneYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneYAMLMap(typed)
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, cloneYAMLValue(child))
		}
		return result
	default:
		return typed
	}
}
