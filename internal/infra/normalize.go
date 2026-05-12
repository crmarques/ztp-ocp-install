package infra

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func Normalize(state *v1alpha1.State) {
	for i := range state.Environments {
		normalizeEnvironment(&state.Environments[i])
	}
	for i := range state.InfrastructureProviders {
		normalizeProvider(&state.InfrastructureProviders[i])
	}
	for i := range state.ClusterInfrastructures {
		ci := &state.ClusterInfrastructures[i]
		closure, _ := v1alpha1.BuildProviderClosure(*ci, providerIndex(state.InfrastructureProviders))
		provider := synthesizeClosureProvider(*ci, closure)
		normalizeClusterInfrastructure(ci, &provider)
	}
	env := primaryEnvironment(state)
	for i := range state.OCPClusters {
		ocp := &state.OCPClusters[i]
		ci := clusterInfraByName(state, ocp.Spec.InfrastructureRef.Name)
		normalizeOCPCluster(ocp, env, ci)
	}
}

func normalizeEnvironment(env *v1alpha1.Environment) {
	defaultEnvironmentKeys(env)
}

func normalizeProvider(p *v1alpha1.InfrastructureProvider) {
	for name, host := range p.Spec.Hosts {
		if host.SSH != nil && host.SSH.User == "" {
			host.SSH.User = currentUsername()
		}
		p.Spec.Hosts[name] = host
	}
	if p.Spec.Machine != nil && p.Spec.Machine.Libvirt != nil && p.Spec.Machine.Libvirt.BMCEmulation != nil {
		normalizeBMCEmulation(p.Spec.Machine.Libvirt.BMCEmulation)
	}
}

func normalizeBMCEmulation(b *v1alpha1.BMCEmulationSpec) {
	if b.Enabled == nil {
		b.Enabled = v1alpha1.BoolPtr(v1alpha1.DefaultBMCEnabled)
	}
	if b.Protocol == "" {
		b.Protocol = v1alpha1.DefaultBMCProtocol
	}
	if b.Emulator == "" {
		b.Emulator = v1alpha1.DefaultBMCEmulator
	}
	if b.BindAddress == "" {
		b.BindAddress = v1alpha1.DefaultBMCBindAddress
	}
	if b.Port == 0 {
		b.Port = v1alpha1.DefaultBMCPort
	}
}

func normalizeClusterInfrastructure(ci *v1alpha1.ClusterInfrastructure, provider *v1alpha1.InfrastructureProvider) {
	for name, m := range ci.Spec.Machines {
		applyMachineProfile(&m, provider)
		applyMachineDefaultResources(&m, provider)
		applyGeneratedMAC(&m, ci.Metadata.Name, name)
		ci.Spec.Machines[name] = m
	}
}

func applyMachineProfile(m *v1alpha1.MachineSpec, provider *v1alpha1.InfrastructureProvider) {
	if m.ProfileRef == nil || provider == nil || provider.Spec.Machine == nil || provider.Spec.Machine.Libvirt == nil {
		return
	}
	profile, ok := provider.Spec.Machine.Libvirt.MachineProfiles[m.ProfileRef.Name]
	if !ok {
		return
	}
	if m.Resources == nil {
		m.Resources = &v1alpha1.MachineResourcesSpec{}
	}
	if m.Resources.CPU == 0 {
		m.Resources.CPU = profile.CPU
	}
	if m.Resources.MemoryMiB == 0 {
		m.Resources.MemoryMiB = profile.MemoryMiB
	}
	if m.Resources.DiskGiB == 0 {
		m.Resources.DiskGiB = profile.DiskGiB
	}
}

func applyMachineDefaultResources(m *v1alpha1.MachineSpec, provider *v1alpha1.InfrastructureProvider) {
	if provider == nil || provider.Spec.Machine == nil || provider.Spec.Machine.Libvirt == nil {
		return
	}
	if m.Libvirt == nil {
		return
	}
	if m.Resources == nil {
		m.Resources = &v1alpha1.MachineResourcesSpec{}
	}
	if m.Resources.CPU == 0 {
		m.Resources.CPU = v1alpha1.DefaultNodeCPU
	}
	if m.Resources.MemoryMiB == 0 {
		m.Resources.MemoryMiB = v1alpha1.DefaultNodeMemoryMiB
	}
	if m.Resources.DiskGiB == 0 {
		m.Resources.DiskGiB = v1alpha1.DefaultNodeDiskGiB
	}
}

func applyGeneratedMAC(m *v1alpha1.MachineSpec, clusterName, machineName string) {
	if m.Libvirt == nil {
		return
	}
	for ifaceName, iface := range m.Interfaces {
		if iface.MACAddress == "" {
			iface.MACAddress = generateMAC(clusterName, machineName, ifaceName)
		}
		m.Interfaces[ifaceName] = iface
	}
}

func generateMAC(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", sum[0], sum[1], sum[2])
}

func normalizeOCPCluster(ocp *v1alpha1.OCPCluster, env *v1alpha1.Environment, ci *v1alpha1.ClusterInfrastructure) {
	if ocp.Spec.Role == "" {
		if isHubName(ocp.Metadata.Name) {
			ocp.Spec.Role = v1alpha1.OCPClusterRoleHub
		} else {
			ocp.Spec.Role = v1alpha1.OCPClusterRoleManaged
		}
	}
	if ocp.Spec.Topology == "" {
		if len(ocp.Spec.Nodes) <= 1 {
			ocp.Spec.Topology = v1alpha1.OCPTopologySingleNode
		} else {
			ocp.Spec.Topology = v1alpha1.OCPTopologyMultiNode
		}
	}
	if ocp.Spec.Install.Method == "" {
		ocp.Spec.Install.Method = v1alpha1.OCPInstallMethodAgent
	}
	applyEnvironmentInstallDefaults(ocp, env)
	applyOCPInstallEnvIntoInstall(ocp, env)
	defaultEndpointHostnames(ci, env, ocp.Metadata.Name)
	for nodeName, node := range ocp.Spec.Nodes {
		if node.MachineRef == nil || node.MachineRef.Name == "" {
			node.MachineRef = &v1alpha1.LocalObjectReference{Name: nodeName}
		}
		ocp.Spec.Nodes[nodeName] = node
	}
}

func isHubName(name string) bool {
	return name == v1alpha1.OCPClusterRoleHub || strings.HasSuffix(name, "-hub")
}

func applyEnvironmentInstallDefaults(ocp *v1alpha1.OCPCluster, env *v1alpha1.Environment) {
	if env == nil {
		return
	}
	if ocp.Spec.Install.BaseDomain == "" {
		ocp.Spec.Install.BaseDomain = env.Spec.BaseDomain
	}
	if ocp.Spec.Install.PullSecretRef.Name == "" {
		ocp.Spec.Install.PullSecretRef = env.Spec.Secrets.PullSecretRef
	}
	if ocp.Spec.Install.SSHKeyRef.Name == "" {
		ocp.Spec.Install.SSHKeyRef = env.Spec.Secrets.ClusterSSHKeyRef
	}
	if ocp.Spec.Install.Release == nil && env.Spec.OpenShift.Release != nil {
		release := *env.Spec.OpenShift.Release
		ocp.Spec.Install.Release = &release
	}
}

func applyOCPInstallEnvIntoInstall(ocp *v1alpha1.OCPCluster, env *v1alpha1.Environment) {
	if env == nil {
		return
	}
	kind := v1alpha1.OCPInstallKind(*env)
	if kind == "" || kind == v1alpha1.OCPInstallKindConnected {
		return
	}
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
	if registries == nil {
		return
	}
	if registries.Mirror != nil && registries.Mirror.TrustBundleRef.Name != "" {
		if ocp.Spec.Install.AdditionalTrustBundleRef.Name == "" {
			ocp.Spec.Install.AdditionalTrustBundleRef = registries.Mirror.TrustBundleRef
		}
	}
	if kind == v1alpha1.OCPInstallKindDisconnected {
		ocp.Spec.Install.ImageDigestSources = mergeImageDigestSources(
			ocp.Spec.Install.ImageDigestSources,
			deriveDisconnectedReleaseSources(registries),
		)
		defaultDisconnectedImageSourcePolicy(ocp.Spec.Install.ImageDigestSources)
	} else if len(ocp.Spec.Install.ImageDigestSources) == 0 && len(registries.ImageDigestSources) > 0 {
		ocp.Spec.Install.ImageDigestSources = append([]v1alpha1.ImageDigestSource(nil), registries.ImageDigestSources...)
	}
}

func deriveDisconnectedReleaseSources(registries *v1alpha1.OCPInstallRegistries) []v1alpha1.ImageDigestSource {
	if registries == nil {
		return nil
	}
	out := append([]v1alpha1.ImageDigestSource(nil), registries.ImageDigestSources...)
	known := map[string]bool{}
	for _, src := range out {
		known[src.Source] = true
	}
	mirrorURL := ""
	if registries.Mirror != nil {
		mirrorURL = strings.TrimRight(registries.Mirror.URL, "/")
	}
	if mirrorURL == "" {
		return out
	}
	defaults := []struct {
		source string
		path   string
	}{
		{v1alpha1.OCPReleaseSourceQuayOCPRelease, v1alpha1.DefaultMirroredReleasePath},
		{v1alpha1.OCPReleaseSourceQuayARTDev, v1alpha1.DefaultMirroredReleasePath},
	}
	for _, d := range defaults {
		if known[d.source] {
			continue
		}
		out = append(out, v1alpha1.ImageDigestSource{
			Source:       d.source,
			Mirrors:      []string{mirrorURL + "/" + d.path},
			SourcePolicy: v1alpha1.ImageSourcePolicyNever,
		})
	}
	return out
}

func mergeImageDigestSources(existing, derived []v1alpha1.ImageDigestSource) []v1alpha1.ImageDigestSource {
	out := append([]v1alpha1.ImageDigestSource(nil), existing...)
	known := map[string]bool{}
	for _, src := range out {
		known[src.Source] = true
	}
	for _, src := range derived {
		if known[src.Source] {
			continue
		}
		out = append(out, src)
		known[src.Source] = true
	}
	return out
}

func defaultDisconnectedImageSourcePolicy(sources []v1alpha1.ImageDigestSource) {
	for i := range sources {
		if sources[i].SourcePolicy == "" {
			sources[i].SourcePolicy = v1alpha1.ImageSourcePolicyNever
		}
	}
}

func defaultEnvironmentKeys(env *v1alpha1.Environment) {
	if env == nil {
		return
	}
	for name, key := range env.Spec.Keys {
		if key.Generated == nil {
			continue
		}
		if cert := key.Generated.SelfSignedCertificate; cert != nil && cert.ValidityDays == 0 {
			cert.ValidityDays = v1alpha1.DefaultCertificateDays
			key.Generated.SelfSignedCertificate = cert
		}
		if creds := key.Generated.Credentials; creds != nil && creds.Username == "" {
			creds.Username = "admin"
			key.Generated.Credentials = creds
		}
		env.Spec.Keys[name] = key
	}
}

func defaultEndpointHostnames(ci *v1alpha1.ClusterInfrastructure, env *v1alpha1.Environment, ocpName string) {
	if ci == nil || env == nil || env.Spec.BaseDomain == "" {
		return
	}
	if ci.Spec.Endpoints.API != nil && ci.Spec.Endpoints.API.Hostname == "" {
		ci.Spec.Endpoints.API.Hostname = fmt.Sprintf("api.%s.%s", ocpName, env.Spec.BaseDomain)
	}
	if ci.Spec.Endpoints.APIInt != nil && ci.Spec.Endpoints.APIInt.Hostname == "" {
		ci.Spec.Endpoints.APIInt.Hostname = fmt.Sprintf("api-int.%s.%s", ocpName, env.Spec.BaseDomain)
	}
	if ci.Spec.Endpoints.Ingress != nil && ci.Spec.Endpoints.Ingress.Hostname == "" {
		ci.Spec.Endpoints.Ingress.Hostname = fmt.Sprintf("*.apps.%s.%s", ocpName, env.Spec.BaseDomain)
	}
}

func primaryEnvironment(state *v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

func clusterInfraByName(state *v1alpha1.State, name string) *v1alpha1.ClusterInfrastructure {
	for i := range state.ClusterInfrastructures {
		if state.ClusterInfrastructures[i].Metadata.Name == name {
			return &state.ClusterInfrastructures[i]
		}
	}
	return nil
}

func currentUsername() string {
	for _, key := range []string{"USER", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
