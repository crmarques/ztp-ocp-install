package infra

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func Validate(state v1alpha1.State) error {
	var errs []string
	errs = append(errs, validateEnvironments(state.Environments)...)
	errs = append(errs, validateProviders(state.InfrastructureProviders)...)
	errs = append(errs, validateClusterInfrastructures(state)...)
	errs = append(errs, validateOCPClusters(state)...)
	errs = append(errs, validateCrossLayer(state)...)
	errs = append(errs, validateSecretReferences(state)...)
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func IsDNSLabel(s string) bool {
	return dnsLabel.MatchString(s)
}

func validateName(kind, name string) string {
	if name == "" {
		return fmt.Sprintf("%s.metadata.name is required", kind)
	}
	if !dnsLabel.MatchString(name) {
		return fmt.Sprintf("%s.metadata.name %q is not a DNS label", kind, name)
	}
	return ""
}

func validateCrossLayer(state v1alpha1.State) []string {
	var errs []string
	infraToOCP := map[string][]string{}
	for _, ocp := range state.OCPClusters {
		infraToOCP[ocp.Spec.InfrastructureRef.Name] = append(infraToOCP[ocp.Spec.InfrastructureRef.Name], ocp.Metadata.Name)
	}
	infraNames := make([]string, 0, len(state.ClusterInfrastructures))
	for _, ci := range state.ClusterInfrastructures {
		infraNames = append(infraNames, ci.Metadata.Name)
	}
	sort.Strings(infraNames)
	for _, ciName := range infraNames {
		owners := infraToOCP[ciName]
		if len(owners) == 0 {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s has no OCPCluster bound to it", ciName))
		}
		if len(owners) > 1 {
			sort.Strings(owners)
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s referenced by multiple OCPClusters: %s", ciName, strings.Join(owners, ", ")))
		}
	}
	if env := primaryEnvironment(&state); env != nil {
		registries := env.Spec.Registries
		if registries != nil && registries.Mirror != nil {
			if u := registries.Mirror.URL; u != "" {
				if _, err := url.Parse("https://" + u); err != nil {
					errs = append(errs, fmt.Sprintf("Environment/%s spec.registries.mirror.url %q invalid: %v", env.Metadata.Name, u, err))
				}
			}
		}
	}
	errs = append(errs, validateDisconnectedOpenShiftSources(state)...)
	errs = append(errs, validateMirrorRegistryPlacement(state)...)
	errs = append(errs, validateManagedProxyPlacement(state)...)
	return errs
}

func validateSecretReferences(state v1alpha1.State) []string {
	env := primaryEnvironment(&state)
	if env == nil {
		return nil
	}
	declared := map[string]bool{}
	for name := range env.Spec.Secrets {
		declared[name] = true
	}
	var errs []string
	require := func(owner string, ref v1alpha1.SecretRef) {
		if ref.Name == "" {
			return
		}
		if !dnsLabel.MatchString(ref.Name) {
			errs = append(errs, fmt.Sprintf("%s.name %q is not a DNS label", owner, ref.Name))
			return
		}
		if !declared[ref.Name] {
			errs = append(errs, fmt.Sprintf("%s %q is not declared in Environment/%s spec.secrets", owner, ref.Name, env.Metadata.Name))
		}
	}
	if env.Spec.Proxy != nil && env.Spec.Proxy.Auth != nil {
		require(fmt.Sprintf("Environment/%s spec.proxy.auth.proxyAuthRef", env.Metadata.Name), env.Spec.Proxy.Auth.ProxyAuthRef)
	}
	if registries := env.Spec.Registries; registries != nil && registries.Mirror != nil {
		owner := fmt.Sprintf("Environment/%s spec.registries.mirror", env.Metadata.Name)
		require(owner+".credentialsRef", registries.Mirror.CredentialsRef)
		require(owner+".trustBundleRef", registries.Mirror.TrustBundleRef)
	}
	for _, provider := range state.InfrastructureProviders {
		for hostName, host := range provider.Spec.Hosts {
			if host.SSH != nil {
				require(fmt.Sprintf("InfrastructureProvider/%s hosts[%s].ssh.keyRef", provider.Metadata.Name, hostName), host.SSH.KeyRef)
			}
		}
		if provider.Spec.Machine == nil {
			continue
		}
		if provider.Spec.Machine.Libvirt != nil && provider.Spec.Machine.Libvirt.BMCEmulation != nil && provider.Spec.Machine.Libvirt.BMCEmulation.Auth != nil {
			require(fmt.Sprintf("InfrastructureProvider/%s machine.libvirt.bmcEmulation.auth.credentialRef", provider.Metadata.Name), provider.Spec.Machine.Libvirt.BMCEmulation.Auth.CredentialRef)
		}
		if provider.Spec.Machine.VSphere != nil {
			require(fmt.Sprintf("InfrastructureProvider/%s machine.vsphere.vCenterRef", provider.Metadata.Name), provider.Spec.Machine.VSphere.VCenterRef)
		}
		if provider.Spec.Machine.KubeVirt != nil {
			require(fmt.Sprintf("InfrastructureProvider/%s machine.kubevirt.clusterRef", provider.Metadata.Name), provider.Spec.Machine.KubeVirt.ClusterRef)
		}
	}
	for _, ci := range state.ClusterInfrastructures {
		for machineName, machine := range ci.Spec.Machines {
			if machine.BareMetal != nil && machine.BareMetal.BMC != nil {
				require(fmt.Sprintf("ClusterInfrastructure/%s machines[%s].baremetal.bmc.credentialRef", ci.Metadata.Name, machineName), machine.BareMetal.BMC.CredentialRef)
			}
		}
	}
	for _, ocp := range state.OCPClusters {
		require(fmt.Sprintf("OCPCluster/%s install.pullSecretRef", ocp.Metadata.Name), ocp.Spec.Install.PullSecretRef)
		require(fmt.Sprintf("OCPCluster/%s install.sshKeyRef", ocp.Metadata.Name), ocp.Spec.Install.SSHKeyRef)
		require(fmt.Sprintf("OCPCluster/%s install.additionalTrustBundleRef", ocp.Metadata.Name), ocp.Spec.Install.AdditionalTrustBundleRef)
	}
	return errs
}

func validateMirrorRegistryPlacement(state v1alpha1.State) []string {
	env := primaryEnvironment(&state)
	if env == nil {
		return nil
	}
	if v1alpha1.OCPInstallKind(*env) != v1alpha1.OCPInstallKindDisconnected {
		return nil
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	var errs []string
	suppliers := []string{}
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.ProviderMirrorRegistry(p) != nil {
			suppliers = append(suppliers, p.Metadata.Name)
		}
	}
	if len(suppliers) == 0 {
		errs = append(errs, fmt.Sprintf("Environment/%s ocpInstallType=disconnected requires at least one InfrastructureProvider with spec.registry.mirrorRegistry set", env.Metadata.Name))
		return errs
	}
	if len(suppliers) > 1 {
		sort.Strings(suppliers)
		errs = append(errs, fmt.Sprintf("Environment/%s ocpInstallType=disconnected requires exactly one provider supplying spec.registry.mirrorRegistry, found %d: %s", env.Metadata.Name, len(suppliers), strings.Join(suppliers, ", ")))
	}
	urlPort := mirrorURLPort(registries.Mirror.URL)
	for _, p := range state.InfrastructureProviders {
		mr := v1alpha1.ProviderMirrorRegistry(p)
		if mr == nil || mr.Port == 0 || urlPort == 0 {
			continue
		}
		if mr.Port != urlPort {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.port %d does not match Environment spec.registries.mirror.url port %d", p.Metadata.Name, mr.Port, urlPort))
		}
	}
	return errs
}

func validateManagedProxyPlacement(state v1alpha1.State) []string {
	env := primaryEnvironment(&state)
	var p *v1alpha1.EnvironmentProxySpec
	if env != nil {
		p = env.Spec.Proxy
	}
	var errs []string
	var suppliers []v1alpha1.InfrastructureProvider
	for _, provider := range state.InfrastructureProviders {
		if v1alpha1.ProviderProxySquid(provider) != nil {
			suppliers = append(suppliers, provider)
		}
	}
	if len(suppliers) == 0 {
		return nil
	}
	supplierNames := make([]string, 0, len(suppliers))
	for _, provider := range suppliers {
		supplierNames = append(supplierNames, provider.Metadata.Name)
	}
	sort.Strings(supplierNames)
	if len(suppliers) > 1 {
		errs = append(errs, fmt.Sprintf("managed proxy requires exactly one provider supplying spec.proxy.squid, found %d: %s", len(suppliers), strings.Join(supplierNames, ", ")))
		return errs
	}
	supplier := suppliers[0]
	squid := v1alpha1.ProviderProxySquid(supplier)
	if env == nil || p == nil {
		envName := ""
		if env != nil {
			envName = "/" + env.Metadata.Name
		}
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid requires Environment%s spec.proxy.auth.proxyAuthRef.name", supplier.Metadata.Name, envName))
		return errs
	}
	if p.Auth == nil || p.Auth.ProxyAuthRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid requires Environment/%s spec.proxy.auth.proxyAuthRef.name", supplier.Metadata.Name, env.Metadata.Name))
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "http", value: p.HTTP},
		{name: "https", value: p.HTTPS},
	} {
		if field.value == "" {
			continue
		}
		errs = append(errs, validateManagedProxyURLPort(env.Metadata.Name, supplier.Metadata.Name, squid.Port, field.name, field.value)...)
	}

	providers := providerIndex(state.InfrastructureProviders)
	for _, ci := range state.ClusterInfrastructures {
		closure, closureErrs := v1alpha1.BuildProviderClosure(ci, providers)
		if len(closureErrs) > 0 {
			continue
		}
		if closure.Proxy == nil {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s does not reference the managed proxy provider %q; add it to providerRefs or remove spec.proxy.squid", ci.Metadata.Name, supplier.Metadata.Name))
			continue
		}
		if closure.ProxyProviderName != supplier.Metadata.Name {
			continue
		}
		if closure.Machine == nil || closure.Machine.Libvirt == nil {
			continue
		}
		if !clusterHasLibvirtNetwork(ci) {
			continue
		}
		if gateway := primaryMachineNetworkGateway(ci); gateway == "" && (p.HTTP == "" || p.HTTPS == "") {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s managed proxy isolation requires the primary libvirt machine network gateway when spec.proxy.http or https is omitted", ci.Metadata.Name))
		}
		for machineName, machine := range ci.Spec.Machines {
			if machine.Libvirt == nil {
				continue
			}
			if machine.Libvirt.HostRef.Name != squid.HostRef.Name {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].libvirt.hostRef %q is not safely reachable from managed proxy hostRef %q; v1 isolation requires same libvirt/provider host placement", ci.Metadata.Name, machineName, machine.Libvirt.HostRef.Name, squid.HostRef.Name))
			}
		}
	}
	return errs
}

func validateManagedProxyURLPort(envName, providerName string, proxyPort int, fieldName, raw string) []string {
	if proxyPort == 0 {
		proxyPort = v1alpha1.DefaultSquidPort
	}
	var errs []string
	u, err := url.Parse(raw)
	if err != nil {
		return []string{fmt.Sprintf("Environment/%s spec.proxy.%s %q invalid: %v", envName, fieldName, raw, err)}
	}
	if u.Scheme != "http" || u.Host == "" {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.proxy.%s must be an http URL with host for managed proxy InfrastructureProvider/%s", envName, fieldName, providerName))
	}
	urlPort := u.Port()
	if urlPort == "" {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.proxy.%s must include port %d for managed proxy InfrastructureProvider/%s", envName, fieldName, proxyPort, providerName))
		return errs
	}
	if urlPort != fmt.Sprintf("%d", proxyPort) {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.proxy.%s port %s does not match InfrastructureProvider/%s spec.proxy.squid.port %d", envName, fieldName, urlPort, providerName, proxyPort))
	}
	return errs
}

func clusterHasLibvirtNetwork(ci v1alpha1.ClusterInfrastructure) bool {
	for _, network := range ci.Spec.Networks {
		if network.Libvirt != nil {
			return true
		}
	}
	return false
}

func primaryMachineNetworkGateway(ci v1alpha1.ClusterInfrastructure) string {
	names := make([]string, 0, len(ci.Spec.Networks))
	for name := range ci.Spec.Networks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if ci.Spec.Networks[name].Libvirt != nil {
			return ci.Spec.Networks[name].Gateway
		}
	}
	return ""
}

func mirrorURLPort(u string) int {
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

func validateImageDigestSource(owner string, src v1alpha1.ImageDigestSource) []string {
	var errs []string
	if src.Source == "" {
		errs = append(errs, fmt.Sprintf("%s.imageDigestSources entry missing source", owner))
	}
	if len(src.Mirrors) == 0 {
		errs = append(errs, fmt.Sprintf("%s.imageDigestSources[%s] requires at least one mirror", owner, src.Source))
	}
	switch src.SourcePolicy {
	case "", v1alpha1.ImageSourcePolicyNever, v1alpha1.ImageSourcePolicyAllow:
	default:
		errs = append(errs, fmt.Sprintf("%s.imageDigestSources[%s].sourcePolicy %q is not one of {%s, %s}", owner, src.Source, src.SourcePolicy, v1alpha1.ImageSourcePolicyNever, v1alpha1.ImageSourcePolicyAllow))
	}
	return errs
}

func validateDisconnectedOpenShiftSources(state v1alpha1.State) []string {
	env := primaryEnvironment(&state)
	if env == nil || v1alpha1.OCPInstallKind(*env) != v1alpha1.OCPInstallKindDisconnected {
		return nil
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	mirrorURL := registries.Mirror.URL
	var errs []string
	for _, ocp := range state.OCPClusters {
		errs = append(errs, validateDisconnectedOCPImageSources(ocp, mirrorURL)...)
	}
	return errs
}

func validateDisconnectedOCPImageSources(ocp v1alpha1.OCPCluster, mirrorURL string) []string {
	var errs []string
	requiredSources := map[string]bool{
		v1alpha1.OCPReleaseSourceQuayOCPRelease: false,
		v1alpha1.OCPReleaseSourceQuayARTDev:     false,
	}
	for _, src := range ocp.Spec.Install.ImageDigestSources {
		if _, ok := requiredSources[src.Source]; ok {
			requiredSources[src.Source] = true
		}
		if src.SourcePolicy != v1alpha1.ImageSourcePolicyNever {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.imageDigestSources[%s].sourcePolicy must be %s when disconnected", ocp.Metadata.Name, src.Source, v1alpha1.ImageSourcePolicyNever))
		}
		errs = append(errs, validateMirrorRefs(fmt.Sprintf("OCPCluster/%s install.imageDigestSources[%s]", ocp.Metadata.Name, src.Source), mirrorURL, src.Mirrors)...)
	}
	for source, found := range requiredSources {
		if !found {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s disconnected install requires imageDigestSources mirror for %s", ocp.Metadata.Name, source))
		}
	}
	return errs
}

func validateMirrorRefs(owner string, mirrorURL string, mirrors []string) []string {
	var errs []string
	for _, mirror := range mirrors {
		if !isLocalMirrorRef(mirror, mirrorURL) {
			errs = append(errs, fmt.Sprintf("%s mirror %q must use disconnected mirror %q", owner, mirror, mirrorURL))
		}
	}
	return errs
}

func isLocalMirrorRef(ref string, mirrorURL string) bool {
	mirrorURL = strings.TrimRight(mirrorURL, "/")
	return ref == mirrorURL || strings.HasPrefix(ref, mirrorURL+"/")
}

func providerIndex(providers []v1alpha1.InfrastructureProvider) map[string]v1alpha1.InfrastructureProvider {
	out := map[string]v1alpha1.InfrastructureProvider{}
	for _, p := range providers {
		out[p.Metadata.Name] = p
	}
	return out
}

func clusterInfraIndex(items []v1alpha1.ClusterInfrastructure) map[string]v1alpha1.ClusterInfrastructure {
	out := map[string]v1alpha1.ClusterInfrastructure{}
	for _, ci := range items {
		out[ci.Metadata.Name] = ci
	}
	return out
}
