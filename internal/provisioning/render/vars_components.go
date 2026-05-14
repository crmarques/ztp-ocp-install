package render

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

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
