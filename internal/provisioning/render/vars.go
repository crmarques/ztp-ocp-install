package render

import (
	"sort"
	"strings"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/secretref"
)

// Vars projects a fully-validated State into the closed set of top-level
// facts ansible roles consume. The capability slices live in sibling files:
// vars_cluster.go, vars_provider.go, vars_proxy.go, vars_install.go,
// vars_components.go.
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
		BootwrightOCPInstall:       ocpInstallEnvVars(state, env, secretsDir),
		BootwrightProviders:        providerComponentVars(state, secretsDir),
		BootwrightLoadBalancers:    sharedLoadBalancerVars(state, env),
		BootwrightMirrorRegistries: mirrorRegistryRunVars(state, env, secretsDir),
		BootwrightForwardProxies:   forwardProxyRunVars(state, env, secretsDir),
		BootwrightClusters:         clusters,
		BootwrightComponentPins:    ComponentPins(state),
	}
}

func resolvedSecretPath(name, secretsDir string, env *v1alpha1.Environment) string {
	return secretref.ResolvePath(name, env, secretsDir)
}

func mirrorRegistryHostname(url string) string {
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		return url[:idx]
	}
	return url
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
