package render

import (
	"sort"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/proxy"
)

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
