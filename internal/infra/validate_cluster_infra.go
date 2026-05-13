package infra

import (
	"fmt"
	"net"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

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
			Proxy:          closure.Proxy,
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
		machineKind := ""
		switch {
		case machine.Libvirt != nil:
			machineKind = v1alpha1.MachineFlavorLibvirt
		case machine.BareMetal != nil:
			machineKind = v1alpha1.MachineFlavorBareMetal
		case machine.VSphere != nil:
			machineKind = v1alpha1.MachineFlavorVSphere
		}
		if machineKind != "" && machineKind != providerKind {
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
