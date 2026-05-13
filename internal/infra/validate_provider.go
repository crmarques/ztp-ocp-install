package infra

import (
	"fmt"

	"github.com/crmarques/gitups/api/v1alpha1"
)

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
		if p.Spec.Machine == nil && p.Spec.LoadBalancer == nil && p.Spec.NameResolution == nil && p.Spec.Registry == nil && p.Spec.Proxy == nil {
			errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec must set at least one capability sub-block (machine, loadBalancer, nameResolution, registry, proxy)", p.Metadata.Name))
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
		errs = append(errs, validateProviderProxy(p)...)
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

func validateProviderProxy(p v1alpha1.InfrastructureProvider) []string {
	if p.Spec.Proxy == nil {
		return nil
	}
	var errs []string
	if p.Spec.Proxy.Squid == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy must set exactly one of {squid}", p.Metadata.Name))
		return errs
	}
	squid := p.Spec.Proxy.Squid
	if squid.HostRef.Name == "" {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid.hostRef.name is required", p.Metadata.Name))
		return errs
	}
	host, ok := p.Spec.Hosts[squid.HostRef.Name]
	if !ok {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid.hostRef %q not defined under spec.hosts", p.Metadata.Name, squid.HostRef.Name))
		return errs
	}
	if host.SSH == nil {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid.hostRef %q must have ssh connection set", p.Metadata.Name, squid.HostRef.Name))
	}
	if !hasCapability(host.Capabilities, v1alpha1.CapabilityProxy) {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid.hostRef %q lacks capability %q", p.Metadata.Name, squid.HostRef.Name, v1alpha1.CapabilityProxy))
	}
	if squid.Port != 0 && (squid.Port < 1 || squid.Port > 65535) {
		errs = append(errs, fmt.Sprintf("InfrastructureProvider/%s spec.proxy.squid.port %d out of range", p.Metadata.Name, squid.Port))
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
