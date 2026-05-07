package infra

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
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

func validateEnvironments(envs []v1alpha1.Environment) []string {
	var errs []string
	if len(envs) == 0 {
		errs = append(errs, "at least one Environment is required")
	}
	if len(envs) > 1 {
		errs = append(errs, "exactly one Environment is supported in this release")
	}
	seen := map[string]bool{}
	for _, env := range envs {
		if e := validateName("Environment", env.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[env.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate Environment %q", env.Metadata.Name))
		}
		seen[env.Metadata.Name] = true
		if env.Spec.BaseDomain == "" {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.baseDomain is required", env.Metadata.Name))
		}
		errs = append(errs, validateOCPInstall(env)...)
		errs = append(errs, validateEnvironmentSecrets(env)...)
		errs = append(errs, validateEnvironmentKeys(env)...)
		errs = append(errs, validateComponentImages(env)...)
	}
	return errs
}

func validateEnvironmentKeys(env v1alpha1.Environment) []string {
	var errs []string
	for name, key := range env.Spec.Keys {
		if !dnsLabel.MatchString(name) {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys key %q is not a DNS label", env.Metadata.Name, name))
			continue
		}
		hasFile := key.File != ""
		hasGenerated := key.Generated != nil
		switch {
		case hasFile && hasGenerated:
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s] sets both file and generated; pick exactly one source", env.Metadata.Name, name))
		case !hasFile && !hasGenerated:
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s] requires a source (file or generated)", env.Metadata.Name, name))
		case hasGenerated:
			errs = append(errs, validateGeneratedKey(env.Metadata.Name, name, key.Generated)...)
		}
	}
	return errs
}

func validateGeneratedKey(envName, keyName string, gen *v1alpha1.EnvironmentKeyGenerated) []string {
	var errs []string
	hasCreds := gen.Credentials != nil
	hasCert := gen.SelfSignedCertificate != nil
	switch {
	case hasCreds && hasCert:
		errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated sets both credentials and selfSignedCertificate; pick exactly one", envName, keyName))
	case !hasCreds && !hasCert:
		errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated requires one of {credentials, selfSignedCertificate}", envName, keyName))
	case hasCert:
		if gen.SelfSignedCertificate.CommonName == "" {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated.selfSignedCertificate.commonName is required", envName, keyName))
		}
		if gen.SelfSignedCertificate.ValidityDays < 0 {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated.selfSignedCertificate.validityDays must not be negative", envName, keyName))
		}
	case hasCreds:
		username := gen.Credentials.Username
		if username != "" && strings.TrimSpace(username) != username {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated.credentials.username must not contain leading or trailing whitespace", envName, keyName))
		}
		if strings.ContainsAny(username, ":\r\n\t ") {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.keys[%s].generated.credentials.username must not contain whitespace, colon, or newlines", envName, keyName))
		}
	}
	return errs
}

func validateComponentImages(env v1alpha1.Environment) []string {
	var errs []string
	for category, types := range env.Spec.ComponentImages {
		if category == "" || !dnsLabel.MatchString(category) {
			errs = append(errs, fmt.Sprintf("Environment/%s componentImages key %q is not a DNS label", env.Metadata.Name, category))
			continue
		}
		for typ, image := range types {
			if typ == "" || !dnsLabel.MatchString(typ) {
				errs = append(errs, fmt.Sprintf("Environment/%s componentImages[%s] key %q is not a DNS label", env.Metadata.Name, category, typ))
				continue
			}
			if image.Local == "" && image.Public == "" {
				errs = append(errs, fmt.Sprintf("Environment/%s componentImages[%s][%s] requires at least one of local or public", env.Metadata.Name, category, typ))
			}
		}
	}
	return errs
}

func validateOCPInstall(env v1alpha1.Environment) []string {
	var errs []string
	set := 0
	if env.Spec.OCPInstall.Connected != nil {
		set++
	}
	if env.Spec.OCPInstall.Restricted != nil {
		set++
	}
	if env.Spec.OCPInstall.Disconnected != nil {
		set++
	}
	if set != 1 {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.ocpInstall must set exactly one of {connected, restricted, disconnected}", env.Metadata.Name))
		return errs
	}
	switch v1alpha1.OCPInstallKind(env) {
	case v1alpha1.OCPInstallKindConnected:
	case v1alpha1.OCPInstallKindRestricted:
		errs = append(errs, validateRegistriesBlock(env, env.Spec.OCPInstall.Restricted.Registries, false)...)
	case v1alpha1.OCPInstallKindDisconnected:
		errs = append(errs, validateRegistriesBlock(env, env.Spec.OCPInstall.Disconnected.Registries, true)...)
	}
	errs = append(errs, validateOCPInstallProxy(env)...)
	return errs
}

func validateOCPInstallProxy(env v1alpha1.Environment) []string {
	proxy := v1alpha1.OCPInstallProxyOf(env)
	if proxy == nil {
		return nil
	}
	var errs []string
	owner := fmt.Sprintf("Environment/%s ocpInstall.%s.proxy", env.Metadata.Name, v1alpha1.OCPInstallKind(env))
	if proxy.CredentialsRef.Name != "" && !dnsLabel.MatchString(proxy.CredentialsRef.Name) {
		errs = append(errs, fmt.Sprintf("%s.credentialsRef.name %q is not a DNS label", owner, proxy.CredentialsRef.Name))
	}
	for _, field := range []struct{ name, value string }{
		{"httpProxy", proxy.HTTPProxy},
		{"httpsProxy", proxy.HTTPSProxy},
	} {
		if field.value == "" {
			continue
		}
		if proxyURLHasInlineCredentials(field.value) {
			errs = append(errs, fmt.Sprintf("%s.%s must not embed credentials; use credentialsRef and supply the bare URL", owner, field.name))
		}
	}
	return errs
}

func proxyURLHasInlineCredentials(url string) bool {
	idx := strings.Index(url, "://")
	if idx < 0 {
		return false
	}
	authority := url[idx+3:]
	if at := strings.Index(authority, "@"); at >= 0 {
		if slash := strings.Index(authority, "/"); slash < 0 || at < slash {
			return true
		}
	}
	return false
}

func validateRegistriesBlock(env v1alpha1.Environment, registries *v1alpha1.OCPInstallRegistries, requireMirror bool) []string {
	var errs []string
	owner := fmt.Sprintf("Environment/%s ocpInstall.%s", env.Metadata.Name, v1alpha1.OCPInstallKind(env))
	if registries == nil {
		if requireMirror {
			errs = append(errs, fmt.Sprintf("%s requires registries.mirror and trust material", owner))
		}
		return errs
	}
	if requireMirror && registries.Mirror == nil {
		errs = append(errs, fmt.Sprintf("%s requires registries.mirror", owner))
	}
	if registries.Mirror != nil {
		if registries.Mirror.URL == "" {
			errs = append(errs, fmt.Sprintf("%s.registries.mirror.url is required", owner))
		}
		if requireMirror && registries.Mirror.TrustBundleRef.Name == "" {
			errs = append(errs, fmt.Sprintf("%s.registries.mirror.trustBundleRef is required", owner))
		}
	}
	if requireMirror {
		errs = append(errs, validateDisconnectedRegistrySources(env, registries)...)
	}
	return errs
}

func validateDisconnectedRegistrySources(env v1alpha1.Environment, registries *v1alpha1.OCPInstallRegistries) []string {
	var errs []string
	if registries == nil || registries.Mirror == nil {
		return errs
	}
	owner := fmt.Sprintf("Environment/%s ocpInstall.disconnected.registries", env.Metadata.Name)
	for _, src := range registries.ImageDigestSources {
		errs = append(errs, validateImageDigestSource(owner, src)...)
		errs = append(errs, validateMirrorRefs(fmt.Sprintf("%s.imageDigestSources[%s]", owner, src.Source), registries.Mirror.URL, src.Mirrors)...)
		if src.SourcePolicy == v1alpha1.ImageSourcePolicyAllow {
			errs = append(errs, fmt.Sprintf("%s.imageDigestSources[%s].sourcePolicy must not allow contacting the source when disconnected", owner, src.Source))
		}
	}
	return errs
}

func validateEnvironmentSecrets(env v1alpha1.Environment) []string {
	var errs []string
	if env.Spec.Secrets.PullSecretRef.Name == "" {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets.pullSecretRef.name is required", env.Metadata.Name))
	}
	if env.Spec.Secrets.ClusterSSHKeyRef.Name == "" {
		errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets.clusterSSHKeyRef.name is required", env.Metadata.Name))
	}
	return errs
}

func validateProviders(providers []v1alpha1.InfrastructureProvider) []string {
	var errs []string
	seen := map[string]bool{}
	for _, p := range providers {
		if e := validateName("InfrastructureProvider", p.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[p.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate InfrastructureProvider %q", p.Metadata.Name))
		}
		seen[p.Metadata.Name] = true
		errs = append(errs, validateProviderHosts(p)...)
		if p.Spec.Machine == nil && p.Spec.LoadBalancer == nil && p.Spec.NameResolution == nil && p.Spec.Registry == nil {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec must set at least one capability sub-block (machine, loadBalancer, nameResolution, registry)", p.Metadata.Name))
		}
		if p.Spec.Machine != nil {
			set := 0
			if p.Spec.Machine.Libvirt != nil {
				set++
			}
			if p.Spec.Machine.BareMetal != nil {
				set++
			}
			if p.Spec.Machine.VSphere != nil {
				set++
			}
			if p.Spec.Machine.KubeVirt != nil {
				set++
			}
			if set != 1 {
				errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.machine must set exactly one of {libvirt, baremetal, vsphere, kubevirt}", p.Metadata.Name))
			}
			if p.Spec.Machine.Libvirt != nil {
				errs = append(errs, validateLibvirtProvider(p)...)
			}
			if p.Spec.Machine.BareMetal != nil {
				errs = append(errs, validateBareMetalProvider(p)...)
			}
			if p.Spec.Machine.VSphere != nil {
				errs = append(errs, validateVSphereProvider(p)...)
			}
			if p.Spec.Machine.KubeVirt != nil {
				errs = append(errs, validateKubeVirtProvider(p)...)
			}
		}
		errs = append(errs, validateProviderLoadBalancer(p)...)
		errs = append(errs, validateProviderNameResolution(p)...)
		errs = append(errs, validateProviderRegistry(p)...)
	}
	return errs
}

func validateBareMetalProvider(p v1alpha1.InfrastructureProvider) []string {
	protocol := p.Spec.Machine.BareMetal.BMCProtocol
	if protocol == "" || protocol == v1alpha1.DefaultBMCProtocol {
		return nil
	}
	return []string{fmt.Sprintf("InfrastructureProvider/%s machine.baremetal.bmcProtocol %q is not supported yet; only %q is implemented", p.Metadata.Name, protocol, v1alpha1.DefaultBMCProtocol)}
}

func validateProviderRegistry(p v1alpha1.InfrastructureProvider) []string {
	if p.Spec.Registry == nil {
		return nil
	}
	var errs []string
	if p.Spec.Registry.MirrorRegistry == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry must set exactly one of {mirrorRegistry}", p.Metadata.Name))
		return errs
	}
	mr := p.Spec.Registry.MirrorRegistry
	if mr.HostRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.hostRef.name is required", p.Metadata.Name))
		return errs
	}
	host, ok := p.Spec.Hosts[mr.HostRef.Name]
	if !ok {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.hostRef %q not defined under spec.hosts", p.Metadata.Name, mr.HostRef.Name))
		return errs
	}
	if host.SSH == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.hostRef %q must have ssh connection set", p.Metadata.Name, mr.HostRef.Name))
	}
	if !hasCapability(host.Capabilities, v1alpha1.CapabilityMirrorRegistry) {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.hostRef %q lacks capability %q", p.Metadata.Name, mr.HostRef.Name, v1alpha1.CapabilityMirrorRegistry))
	}
	if mr.Port != 0 && (mr.Port < 1 || mr.Port > 65535) {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.port %d out of range", p.Metadata.Name, mr.Port))
	}
	return errs
}

func validateProviderLoadBalancer(p v1alpha1.InfrastructureProvider) []string {
	if p.Spec.LoadBalancer == nil {
		return nil
	}
	var errs []string
	if p.Spec.LoadBalancer.HAProxy == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.loadBalancer must set exactly one of {haProxy}", p.Metadata.Name))
		return errs
	}
	hp := p.Spec.LoadBalancer.HAProxy
	if hp.HostRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.loadBalancer.haProxy.hostRef.name is required", p.Metadata.Name))
		return errs
	}
	host, ok := p.Spec.Hosts[hp.HostRef.Name]
	if !ok {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.loadBalancer.haProxy.hostRef %q not defined under spec.hosts", p.Metadata.Name, hp.HostRef.Name))
		return errs
	}
	if host.SSH == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.loadBalancer.haProxy.hostRef %q must have ssh connection set", p.Metadata.Name, hp.HostRef.Name))
	}
	return errs
}

func validateLibvirtProvider(p v1alpha1.InfrastructureProvider) []string {
	var errs []string
	for i, ref := range p.Spec.Machine.Libvirt.HostRefs {
		if ref.Name == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.libvirt.hostRefs[%d].name is required", p.Metadata.Name, i))
			continue
		}
		host, ok := p.Spec.Hosts[ref.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.libvirt.hostRefs[%d] %q not defined under spec.hosts", p.Metadata.Name, i, ref.Name))
			continue
		}
		if host.SSH == nil {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.libvirt.hostRefs[%d] %q must have ssh connection set", p.Metadata.Name, i, ref.Name))
		}
	}
	return errs
}

func validateProviderHosts(p v1alpha1.InfrastructureProvider) []string {
	var errs []string
	for hostName, host := range p.Spec.Hosts {
		if hostName == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s hosts has empty host key", p.Metadata.Name))
		}
		if host.SSH == nil {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s hosts[%s] must set ssh connection", p.Metadata.Name, hostName))
			continue
		}
		if host.SSH.Address == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s hosts[%s].ssh.address is required", p.Metadata.Name, hostName))
		}
		if host.SSH.User == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s hosts[%s].ssh.user is required when the invoking user cannot be detected", p.Metadata.Name, hostName))
		}
		if host.SSH.KeyRef.Name == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s hosts[%s].ssh.keyRef.name is required", p.Metadata.Name, hostName))
		}
	}
	return errs
}

func validateVSphereProvider(p v1alpha1.InfrastructureProvider) []string {
	var errs []string
	v := p.Spec.Machine.VSphere
	if v.VCenterRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.vsphere.vCenterRef.name is required", p.Metadata.Name))
	}
	for _, field := range []struct{ name, value string }{
		{"datacenter", v.Datacenter},
		{"cluster", v.Cluster},
	} {
		if field.value == "" {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.vsphere.%s is required", p.Metadata.Name, field.name))
		}
	}
	return errs
}

func validateKubeVirtProvider(p v1alpha1.InfrastructureProvider) []string {
	var errs []string
	kv := p.Spec.Machine.KubeVirt
	if kv.ClusterRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.kubevirt.clusterRef.name is required", p.Metadata.Name))
	}
	if kv.Namespace == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s machine.kubevirt.namespace is required", p.Metadata.Name))
	}
	return errs
}

func validateClusterInfrastructures(state v1alpha1.State) []string {
	var errs []string
	seen := map[string]bool{}
	providers := providerIndex(state.InfrastructureProviders)
	for _, ci := range state.ClusterInfrastructures {
		if e := validateName("ClusterInfrastructure", ci.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[ci.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate ClusterInfrastructure %q", ci.Metadata.Name))
		}
		seen[ci.Metadata.Name] = true
		if len(ci.Spec.ProviderRefs) == 0 {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs is required", ci.Metadata.Name))
			continue
		}
		closure, closureErrs := v1alpha1.BuildProviderClosure(ci, providers)
		errs = append(errs, closureErrs...)
		closureProvider := synthesizeClosureProvider(ci, closure)
		errs = append(errs, validateNetworks(ci, closureProvider)...)
		errs = append(errs, validateMachines(ci, closureProvider)...)
		errs = append(errs, validateEndpoints(ci)...)
		errs = append(errs, validateLoadBalancers(ci, closureProvider)...)
	}
	return errs
}

func synthesizeClosureProvider(ci v1alpha1.ClusterInfrastructure, closure v1alpha1.ProviderClosure) v1alpha1.InfrastructureProvider {
	return v1alpha1.InfrastructureProvider{
		Metadata: v1alpha1.Metadata{Name: strings.Join(closure.ProviderRefNames, "+")},
		Spec: v1alpha1.InfrastructureProviderSpec{
			Hosts:          closure.Hosts,
			Machine:        closure.Machine,
			LoadBalancer:   closure.LoadBalancer,
			NameResolution: closure.NameResolution,
			Registry:       closure.Registry,
		},
	}
}

func validateNetworks(ci v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider) []string {
	var errs []string
	providerKind := v1alpha1.MachineFlavor(provider)
	for name, network := range ci.Spec.Networks {
		if _, _, err := net.ParseCIDR(network.CIDR); err != nil {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].cidr %q invalid: %v", ci.Metadata.Name, name, network.CIDR, err))
		}
		if network.Gateway != "" && net.ParseIP(network.Gateway) == nil {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].gateway %q is not a valid IP", ci.Metadata.Name, name, network.Gateway))
		}
		for _, ds := range network.DNSServers {
			if net.ParseIP(ds) == nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].dnsServers %q is not a valid IP", ci.Metadata.Name, name, ds))
			}
		}
		switch providerKind {
		case v1alpha1.MachineFlavorLibvirt:
			if network.Libvirt == nil || network.Libvirt.Bridge == "" {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].libvirt.bridge is required for libvirt provider", ci.Metadata.Name, name))
			}
			if network.VSphere != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].vsphere does not match InfrastructureProvider/%s kind %q", ci.Metadata.Name, name, provider.Metadata.Name, providerKind))
			}
		case v1alpha1.MachineFlavorVSphere:
			if network.Libvirt != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].libvirt does not match InfrastructureProvider/%s kind %q", ci.Metadata.Name, name, provider.Metadata.Name, providerKind))
			}
		default:
			if network.Libvirt != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].libvirt does not match InfrastructureProvider/%s kind %q", ci.Metadata.Name, name, provider.Metadata.Name, providerKind))
			}
			if network.VSphere != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s networks[%s].vsphere does not match InfrastructureProvider/%s kind %q", ci.Metadata.Name, name, provider.Metadata.Name, providerKind))
			}
		}
	}
	return errs
}

func validateMachines(ci v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider) []string {
	var errs []string
	providerKind := v1alpha1.MachineFlavor(provider)
	for name, machine := range ci.Spec.Machines {
		set := 0
		if machine.Libvirt != nil {
			set++
		}
		if machine.BareMetal != nil {
			set++
		}
		if machine.VSphere != nil {
			set++
		}
		if set != 1 {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s] must set exactly one of {libvirt, baremetal, vsphere}", ci.Metadata.Name, name))
		}
		if machineKind := v1alpha1.MachineKind(machine); machineKind != "" && machineKind != providerKind {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s] kind %q does not match InfrastructureProvider/%s kind %q", ci.Metadata.Name, name, machineKind, provider.Metadata.Name, providerKind))
		}
		if machine.Libvirt != nil && provider.Spec.Machine != nil && provider.Spec.Machine.Libvirt != nil {
			if _, ok := provider.Spec.Hosts[machine.Libvirt.HostRef.Name]; !ok {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].libvirt.hostRef %q not defined on InfrastructureProvider/%s", ci.Metadata.Name, name, machine.Libvirt.HostRef.Name, provider.Metadata.Name))
			}
		}
		if machine.BareMetal != nil && machine.BareMetal.BMC != nil {
			if machine.BareMetal.BMC.Address == "" {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].baremetal.bmc.address is required", ci.Metadata.Name, name))
			}
			if protocol := machine.BareMetal.BMC.Protocol; protocol != "" && protocol != v1alpha1.DefaultBMCProtocol {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].baremetal.bmc.protocol %q is not supported yet; only %q is implemented", ci.Metadata.Name, name, protocol, v1alpha1.DefaultBMCProtocol))
			}
		}
		errs = append(errs, validateMachineInterfaces(ci, name, machine)...)
	}
	return errs
}

func validateMachineInterfaces(ci v1alpha1.ClusterInfrastructure, name string, machine v1alpha1.MachineSpec) []string {
	var errs []string
	if len(machine.Interfaces) == 0 {
		errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].interfaces is required", ci.Metadata.Name, name))
		return errs
	}
	primaryCount := 0
	for ifaceName, iface := range machine.Interfaces {
		if iface.Primary != nil && *iface.Primary {
			primaryCount++
		}
		if iface.NetworkRef.Name == "" {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].interfaces[%s].networkRef.name is required", ci.Metadata.Name, name, ifaceName))
			continue
		}
		network, ok := ci.Spec.Networks[iface.NetworkRef.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].interfaces[%s].networkRef %q not defined on ClusterInfrastructure", ci.Metadata.Name, name, ifaceName, iface.NetworkRef.Name))
			continue
		}
		if iface.IPAddress != "" {
			if ip := net.ParseIP(iface.IPAddress); ip == nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].interfaces[%s].ipAddress %q invalid", ci.Metadata.Name, name, ifaceName, iface.IPAddress))
			} else if _, cidr, err := net.ParseCIDR(network.CIDR); err == nil && !cidr.Contains(ip) {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s].interfaces[%s].ipAddress %q outside network %s", ci.Metadata.Name, name, ifaceName, iface.IPAddress, network.CIDR))
			}
		}
	}
	if len(machine.Interfaces) > 1 && primaryCount != 1 {
		errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s machines[%s] must mark exactly one interface as primary when multiple are defined", ci.Metadata.Name, name))
	}
	return errs
}

func validateEndpoints(ci v1alpha1.ClusterInfrastructure) []string {
	var errs []string
	for _, e := range []struct {
		name string
		spec *v1alpha1.EndpointSpec
	}{
		{v1alpha1.EndpointAPI, ci.Spec.Endpoints.API},
		{v1alpha1.EndpointAPIInt, ci.Spec.Endpoints.APIInt},
		{v1alpha1.EndpointIngress, ci.Spec.Endpoints.Ingress},
	} {
		if e.spec == nil {
			continue
		}
		if e.spec.Address == "" {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s endpoints.%s.address is required", ci.Metadata.Name, e.name))
			continue
		}
		if ip := net.ParseIP(e.spec.Address); ip == nil {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s endpoints.%s.address %q is not a valid IP", ci.Metadata.Name, e.name, e.spec.Address))
		}
	}
	return errs
}

func validateLoadBalancers(ci v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider) []string {
	var errs []string
	if len(ci.Spec.LoadBalancers) > 0 && (provider.Spec.LoadBalancer == nil || provider.Spec.LoadBalancer.HAProxy == nil) {
		errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s declares loadBalancers but InfrastructureProvider/%s does not supply spec.loadBalancer", ci.Metadata.Name, provider.Metadata.Name))
	}
	for lbName, lb := range ci.Spec.LoadBalancers {
		if len(lb.Endpoints) == 0 {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s loadBalancers[%s].endpoints is required", ci.Metadata.Name, lbName))
		}
		for _, ep := range lb.Endpoints {
			switch ep {
			case v1alpha1.EndpointAPI:
				if ci.Spec.Endpoints.API == nil {
					errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s loadBalancers[%s].endpoints references api but endpoints.api is not defined", ci.Metadata.Name, lbName))
				}
			case v1alpha1.EndpointAPIInt:
				if ci.Spec.Endpoints.APIInt == nil {
					errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s loadBalancers[%s].endpoints references apiInt but endpoints.apiInt is not defined", ci.Metadata.Name, lbName))
				}
			case v1alpha1.EndpointIngress:
				if ci.Spec.Endpoints.Ingress == nil {
					errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s loadBalancers[%s].endpoints references ingress but endpoints.ingress is not defined", ci.Metadata.Name, lbName))
				}
			default:
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s loadBalancers[%s].endpoints %q is not a known endpoint name", ci.Metadata.Name, lbName, ep))
			}
		}
	}
	return errs
}

func validateProviderNameResolution(p v1alpha1.InfrastructureProvider) []string {
	if p.Spec.NameResolution == nil {
		return nil
	}
	var errs []string
	if p.Spec.NameResolution.HostsFile == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.nameResolution must set exactly one of {hostsFile}", p.Metadata.Name))
		return errs
	}
	for i, ref := range p.Spec.NameResolution.HostsFile.HostRefs {
		host, ok := p.Spec.Hosts[ref.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s nameResolution.hostsFile.hostRefs[%d] %q not defined under spec.hosts", p.Metadata.Name, i, ref.Name))
			continue
		}
		if host.SSH == nil {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s nameResolution.hostsFile.hostRefs[%d] %q must have ssh connection set", p.Metadata.Name, i, ref.Name))
		}
		if !hasCapability(host.Capabilities, v1alpha1.CapabilityHostsFile) {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s nameResolution.hostsFile.hostRefs[%d] %q lacks capability %q", p.Metadata.Name, i, ref.Name, v1alpha1.CapabilityHostsFile))
		}
	}
	return errs
}

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func validateOCPClusters(state v1alpha1.State) []string {
	var errs []string
	seen := map[string]bool{}
	infraIndex := clusterInfraIndex(state.ClusterInfrastructures)
	for _, ocp := range state.OCPClusters {
		if e := validateName("OCPCluster", ocp.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[ocp.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate OCPCluster %q", ocp.Metadata.Name))
		}
		seen[ocp.Metadata.Name] = true
		switch ocp.Spec.Topology {
		case "", v1alpha1.OCPTopologySingleNode, v1alpha1.OCPTopologyMultiNode:
		default:
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.topology %q must be single-node or multi-node", ocp.Metadata.Name, ocp.Spec.Topology))
		}
		if ocp.Spec.Install.Method != "" && ocp.Spec.Install.Method != v1alpha1.OCPInstallMethodAgent {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.install.method %q must be %q", ocp.Metadata.Name, ocp.Spec.Install.Method, v1alpha1.OCPInstallMethodAgent))
		}
		ci, ok := infraIndex[ocp.Spec.InfrastructureRef.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.infrastructureRef %q does not match any ClusterInfrastructure", ocp.Metadata.Name, ocp.Spec.InfrastructureRef.Name))
			continue
		}
		errs = append(errs, validateNodes(ocp, ci)...)
		errs = append(errs, validateInstallOverrides(ocp)...)
	}
	return errs
}

func validateNodes(ocp v1alpha1.OCPCluster, ci v1alpha1.ClusterInfrastructure) []string {
	var errs []string
	if len(ocp.Spec.Nodes) == 0 {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.nodes is required", ocp.Metadata.Name))
		return errs
	}
	control := 0
	worker := 0
	for nodeName, node := range ocp.Spec.Nodes {
		switch node.Role {
		case v1alpha1.NodeRoleControlPlane:
			control++
		case v1alpha1.NodeRoleWorker:
			worker++
		default:
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].role %q must be control-plane or worker", ocp.Metadata.Name, nodeName, node.Role))
		}
		if node.MachineRef == nil || node.MachineRef.Name == "" {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].machineRef.name unresolved after normalization", ocp.Metadata.Name, nodeName))
			continue
		}
		if _, ok := ci.Spec.Machines[node.MachineRef.Name]; !ok {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].machineRef %q not defined on ClusterInfrastructure/%s", ocp.Metadata.Name, nodeName, node.MachineRef.Name, ci.Metadata.Name))
		}
	}
	if ocp.Spec.Topology == v1alpha1.OCPTopologySingleNode {
		if control != 1 || worker != 0 {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s topology=single-node requires exactly 1 control-plane and 0 workers (got %d/%d)", ocp.Metadata.Name, control, worker))
		}
	}
	return errs
}

var installOverrideForbiddenKeys = map[string]bool{
	"apiVersion":            true,
	"metadata":              true,
	"baseDomain":            true,
	"pullSecret":            true,
	"sshKey":                true,
	"additionalTrustBundle": true,
	"controlPlane":          true,
	"compute":               true,
	"imageDigestSources":    true,
}

var installOverrideForbiddenNestedPaths = []string{
	"networking.machineNetwork",
	"platform.baremetal.apiVIPs",
	"platform.baremetal.ingressVIPs",
	"platform.vsphere.apiVIPs",
	"platform.vsphere.ingressVIPs",
}

var agentConfigForbiddenKeys = map[string]bool{
	"apiVersion":           true,
	"kind":                 true,
	"metadata":             true,
	"rendezvousIP":         true,
	"hosts":                true,
	"minimalISO":           true,
	"bootArtifactsBaseURL": true,
}

var sensitiveOverrideKeys = []string{"password", "token", "secret", "apikey", "credential"}

func validateInstallOverrides(ocp v1alpha1.OCPCluster) []string {
	var errs []string
	for k := range ocp.Spec.Install.InstallConfigOverrides {
		if installOverrideForbiddenKeys[k] {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.installConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, k))
		}
	}
	errs = append(errs, validateSensitiveOverridePaths(fmt.Sprintf("OCPCluster/%s install.installConfigOverrides", ocp.Metadata.Name), ocp.Spec.Install.InstallConfigOverrides)...)
	for _, path := range installOverrideForbiddenNestedPaths {
		if hasNestedKey(ocp.Spec.Install.InstallConfigOverrides, path) {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.installConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, path))
		}
	}
	for k := range ocp.Spec.Install.AgentConfigOverrides {
		if agentConfigForbiddenKeys[k] {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.agentConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, k))
		}
	}
	errs = append(errs, validateSensitiveOverridePaths(fmt.Sprintf("OCPCluster/%s install.agentConfigOverrides", ocp.Metadata.Name), ocp.Spec.Install.AgentConfigOverrides)...)
	for _, src := range ocp.Spec.Install.ImageDigestSources {
		errs = append(errs, validateImageDigestSource(fmt.Sprintf("OCPCluster/%s install", ocp.Metadata.Name), src)...)
	}
	if ocp.Spec.Install.PullSecretRef.Name == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.pullSecretRef.name is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	if ocp.Spec.Install.SSHKeyRef.Name == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.sshKeyRef.name is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	if ocp.Spec.Install.BaseDomain == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.baseDomain is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	return errs
}

func validateSensitiveOverridePaths(owner string, value any) []string {
	var errs []string
	var walk func(path string, current any)
	walk = func(path string, current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, value := range typed {
				next := key
				if path != "" {
					next = path + "." + key
				}
				lower := strings.ToLower(key)
				for _, sub := range sensitiveOverrideKeys {
					if strings.Contains(lower, sub) {
						errs = append(errs, fmt.Sprintf("%s[%s] looks like a sensitive value; use SecretRef instead", owner, next))
						break
					}
				}
				walk(next, value)
			}
		case []any:
			for i, value := range typed {
				next := fmt.Sprintf("[%d]", i)
				if path != "" {
					next = fmt.Sprintf("%s[%d]", path, i)
				}
				walk(next, value)
			}
		}
	}
	walk("", value)
	return errs
}

func hasNestedKey(m map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	var cursor any = m
	for _, key := range parts {
		current, ok := cursor.(map[string]any)
		if !ok {
			return false
		}
		next, ok := current[key]
		if !ok {
			return false
		}
		cursor = next
	}
	return true
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
		registries := v1alpha1.OCPInstallRegistriesOf(*env)
		if registries != nil && registries.Mirror != nil {
			if u := registries.Mirror.URL; u != "" {
				if _, err := url.Parse("https://" + u); err != nil {
					errs = append(errs, fmt.Sprintf("Environment/%s ocpInstall.registries.mirror.url %q invalid: %v", env.Metadata.Name, u, err))
				}
			}
		}
	}
	errs = append(errs, validateDisconnectedOpenShiftSources(state)...)
	errs = append(errs, validateMirrorRegistryPlacement(state)...)
	return errs
}

func validateSecretReferences(state v1alpha1.State) []string {
	env := primaryEnvironment(&state)
	if env == nil {
		return nil
	}
	declared := map[string]bool{}
	for name := range env.Spec.Keys {
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
			errs = append(errs, fmt.Sprintf("%s %q is not declared in Environment/%s spec.keys", owner, ref.Name, env.Metadata.Name))
		}
	}
	require(fmt.Sprintf("Environment/%s spec.secrets.pullSecretRef", env.Metadata.Name), env.Spec.Secrets.PullSecretRef)
	require(fmt.Sprintf("Environment/%s spec.secrets.clusterSSHKeyRef", env.Metadata.Name), env.Spec.Secrets.ClusterSSHKeyRef)
	if proxy := v1alpha1.OCPInstallProxyOf(*env); proxy != nil {
		require(fmt.Sprintf("Environment/%s ocpInstall.%s.proxy.credentialsRef", env.Metadata.Name, v1alpha1.OCPInstallKind(*env)), proxy.CredentialsRef)
	}
	if registries := v1alpha1.OCPInstallRegistriesOf(*env); registries != nil && registries.Mirror != nil {
		owner := fmt.Sprintf("Environment/%s ocpInstall.%s.registries.mirror", env.Metadata.Name, v1alpha1.OCPInstallKind(*env))
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
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
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
		errs = append(errs, fmt.Sprintf("Environment/%s ocpInstall.disconnected requires at least one InfrastructureProvider with spec.registry.mirrorRegistry set; declare the capability or switch to restricted", env.Metadata.Name))
		return errs
	}
	if len(suppliers) > 1 {
		sort.Strings(suppliers)
		errs = append(errs, fmt.Sprintf("Environment/%s ocpInstall.disconnected requires exactly one provider supplying spec.registry.mirrorRegistry, found %d: %s", env.Metadata.Name, len(suppliers), strings.Join(suppliers, ", ")))
	}
	urlPort := mirrorURLPort(registries.Mirror.URL)
	for _, p := range state.InfrastructureProviders {
		mr := v1alpha1.ProviderMirrorRegistry(p)
		if mr == nil || mr.Port == 0 || urlPort == 0 {
			continue
		}
		if mr.Port != urlPort {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.registry.mirrorRegistry.port %d does not match Environment ocpInstall.registries.mirror.url port %d", p.Metadata.Name, mr.Port, urlPort))
		}
	}
	return errs
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
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
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
