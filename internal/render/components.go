package render

import "github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"

const versionLookupDate = "2026-05-06"

type ComponentPin struct {
	Name       string `yaml:"name" json:"name"`
	Version    string `yaml:"version" json:"version"`
	Source     string `yaml:"source" json:"source"`
	LookupDate string `yaml:"lookupDate" json:"lookupDate"`
}

func ComponentPins(state v1alpha1.State) []ComponentPin {
	pins := []ComponentPin{
		{
			Name:       "ansible-core",
			Version:    "2.20.5",
			Source:     "https://pypi.org/project/ansible-core/",
			LookupDate: versionLookupDate,
		},
		{
			Name:       "go.yaml.in/yaml/v3",
			Version:    "v3.0.4",
			Source:     "https://go.yaml.in/yaml/v3",
			LookupDate: versionLookupDate,
		},
	}
	if usesSushyTools(state) {
		pins = append(pins, ComponentPin{
			Name:       "sushy-tools",
			Version:    "2.2.0",
			Source:     "https://pypi.org/project/sushy-tools/",
			LookupDate: versionLookupDate,
		})
	}
	if usesManagedHAProxy(state) {
		pins = append(pins, ComponentPin{
			Name:       v1alpha1.ComponentTypeHAProxy,
			Version:    "3.3.8",
			Source:     "https://hub.docker.com/_/haproxy",
			LookupDate: versionLookupDate,
		})
	}
	if usesManagedMirrorRegistry(state) {
		pins = append(pins, ComponentPin{
			Name:       v1alpha1.ComponentTypeMirrorRegistry,
			Version:    "3.1.1",
			Source:     "https://hub.docker.com/_/registry",
			LookupDate: versionLookupDate,
		})
	}
	for _, version := range openshiftInstallVersions(state) {
		pins = append(pins, ComponentPin{
			Name:       "openshift-install",
			Version:    version,
			Source:     "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/" + version + "/",
			LookupDate: versionLookupDate,
		})
	}
	return pins
}

func openshiftInstallVersions(state v1alpha1.State) []string {
	seen := map[string]bool{}
	var versions []string
	for _, ocp := range state.OCPClusters {
		if ocp.Spec.Install.Release == nil || ocp.Spec.Install.Release.Version == "" {
			continue
		}
		if seen[ocp.Spec.Install.Release.Version] {
			continue
		}
		seen[ocp.Spec.Install.Release.Version] = true
		versions = append(versions, ocp.Spec.Install.Release.Version)
	}
	return versions
}

func usesSushyTools(state v1alpha1.State) bool {
	for _, provider := range state.InfrastructureProviders {
		if v1alpha1.ProviderMachineLibvirt(provider) != nil &&
			provider.Spec.Machine.Libvirt.BMCEmulation != nil &&
			provider.Spec.Machine.Libvirt.BMCEmulation.Enabled != nil &&
			*provider.Spec.Machine.Libvirt.BMCEmulation.Enabled &&
			provider.Spec.Machine.Libvirt.BMCEmulation.Emulator == v1alpha1.DefaultBMCEmulator {
			return true
		}
	}
	return false
}

func usesManagedHAProxy(state v1alpha1.State) bool {
	for _, infra := range state.ClusterInfrastructures {
		if len(infra.Spec.LoadBalancers) > 0 {
			return true
		}
	}
	return false
}

func usesManagedMirrorRegistry(state v1alpha1.State) bool {
	for _, provider := range state.InfrastructureProviders {
		if v1alpha1.ProviderMirrorRegistry(provider) != nil {
			return true
		}
	}
	return false
}
