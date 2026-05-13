package proxy

import (
	"sort"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

// Effective is the resolved proxy configuration. NoProxy is the union of
// operator-supplied entries (kept in original order) and the auto-derived
// cluster-local set, deduplicated. A nil *Effective means no proxy is
// configured for the environment.
type Effective struct {
	HTTP    string
	HTTPS   string
	NoProxy []string
	Auth    v1alpha1.SecretRef
}

// IsManaged reports whether desired state asks Gitups to provision the
// proxy (any InfrastructureProvider supplies spec.proxy.squid). A managed
// proxy only exists after the bastion has stood it up, so callers that run
// before that — bastion bootstrap — must not route through it. When no
// provider supplies a proxy, any Environment.spec.proxy is external and is
// already reachable.
func IsManaged(state v1alpha1.State) bool {
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.ProviderProxySquid(p) != nil {
			return true
		}
	}
	return false
}

// Resolve returns the effective proxy for env, computed once from state.
// Returns nil when env is nil or has no proxy block. Callers pass the result
// down to renderers and CLI helpers; recomputing per call site would risk
// drift between consumers.
func Resolve(state v1alpha1.State, env *v1alpha1.Environment) *Effective {
	if env == nil || env.Spec.Proxy == nil {
		return nil
	}

	p := env.Spec.Proxy
	eff := &Effective{HTTP: p.HTTP, HTTPS: p.HTTPS}
	if p.Auth != nil {
		eff.Auth = p.Auth.ProxyAuthRef
	}
	eff.NoProxy = merge(p.NoProxy, auto(state, env))
	return eff
}

func auto(state v1alpha1.State, env *v1alpha1.Environment) []string {
	out := []string{"localhost", "127.0.0.1", "::1", ".svc", ".cluster.local"}
	if env.Spec.BaseDomain != "" {
		out = append(out, "."+env.Spec.BaseDomain)
	}
	for _, ci := range state.ClusterInfrastructures {
		for _, network := range ci.Spec.Networks {
			if network.CIDR != "" {
				out = append(out, network.CIDR)
			}
		}
		for _, ep := range []*v1alpha1.EndpointSpec{ci.Spec.Endpoints.API, ci.Spec.Endpoints.APIInt, ci.Spec.Endpoints.Ingress} {
			if ep == nil {
				continue
			}
			if ep.Address != "" {
				out = append(out, ep.Address)
			}
			if ep.Hostname != "" {
				out = append(out, strings.TrimPrefix(ep.Hostname, "*."))
			}
		}
	}
	for _, ocp := range state.OCPClusters {
		if ocp.Spec.Networking != nil {
			for _, c := range ocp.Spec.Networking.ClusterNetwork {
				if c.CIDR != "" {
					out = append(out, c.CIDR)
				}
			}
			out = append(out, ocp.Spec.Networking.ServiceNetwork...)
		}
		if env.Spec.BaseDomain != "" && ocp.Metadata.Name != "" {
			out = append(out, "."+ocp.Metadata.Name+"."+env.Spec.BaseDomain)
		}
	}
	if env.Spec.Registries != nil && env.Spec.Registries.Mirror != nil {
		if host := MirrorHost(env.Spec.Registries.Mirror.URL); host != "" {
			out = append(out, host)
		}
	}
	for _, p := range state.InfrastructureProviders {
		for _, h := range p.Spec.Hosts {
			if h.SSH != nil && h.SSH.Address != "" {
				out = append(out, h.SSH.Address)
			}
		}
	}
	return out
}

func merge(user, auto []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range user {
		e = strings.TrimSpace(e)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	sort.Strings(auto)
	for _, e := range auto {
		e = strings.TrimSpace(e)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// MirrorHost returns the host portion of a mirror registry URL by stripping
// the trailing :port. Empty input yields empty output.
func MirrorHost(url string) string {
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		return url[:idx]
	}
	return url
}
