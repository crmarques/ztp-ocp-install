package proxy

import (
	"strings"
	"testing"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

func TestResolveReturnsNilWhenProxyUnset(t *testing.T) {
	env := &v1alpha1.Environment{Spec: v1alpha1.EnvironmentSpec{BaseDomain: "example.test"}}
	if got := Resolve(v1alpha1.State{}, env); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestResolveReturnsNilWhenEnvNil(t *testing.T) {
	if got := Resolve(v1alpha1.State{}, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestResolveAutoExtendsNoProxy(t *testing.T) {
	env := &v1alpha1.Environment{
		Spec: v1alpha1.EnvironmentSpec{
			BaseDomain: "example.test",
			Proxy: &v1alpha1.EnvironmentProxySpec{
				HTTP:    "http://proxy.example.test:3128",
				NoProxy: []string{".corp.internal"},
			},
			Registries: &v1alpha1.EnvironmentRegistriesSpec{
				Mirror: &v1alpha1.EnvironmentRegistryMirrorSpec{URL: "mirror.example.test:5000"},
			},
		},
	}
	state := v1alpha1.State{
		ClusterInfrastructures: []v1alpha1.ClusterInfrastructure{{
			Spec: v1alpha1.ClusterInfrastructureSpec{
				Networks: map[string]v1alpha1.MachineNetworkSpec{
					"primary": {CIDR: "192.168.130.0/24"},
				},
				Endpoints: v1alpha1.ClusterEndpointsSpec{
					API:     &v1alpha1.EndpointSpec{Address: "192.168.130.10"},
					Ingress: &v1alpha1.EndpointSpec{Address: "192.168.130.11", Hostname: "*.apps.hub.example.test"},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
			Spec: v1alpha1.OCPClusterSpec{
				Networking: &v1alpha1.OCPNetworkingSpec{
					ClusterNetwork: []v1alpha1.OCPClusterNetworkCIDR{{CIDR: "10.128.0.0/14"}},
					ServiceNetwork: []string{"172.30.0.0/16"},
				},
			},
		}},
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"host-01": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "10.0.0.1"}},
				},
			},
		}},
	}
	eff := Resolve(state, env)
	if eff == nil {
		t.Fatal("expected non-nil")
	}
	if eff.NoProxy[0] != ".corp.internal" {
		t.Fatalf("user entry must lead: got %q", eff.NoProxy[0])
	}
	for _, want := range []string{
		".corp.internal", ".example.test", ".hub.example.test", ".svc", ".cluster.local",
		"localhost", "127.0.0.1", "::1",
		"192.168.130.0/24", "192.168.130.10", "192.168.130.11",
		"10.128.0.0/14", "172.30.0.0/16",
		"mirror.example.test", "10.0.0.1",
		".apps.hub.example.test",
	} {
		if !contains(eff.NoProxy, want) {
			t.Fatalf("missing %q in %q", want, strings.Join(eff.NoProxy, ","))
		}
	}
	for _, unwanted := range []string{"*.apps.hub.example.test"} {
		if contains(eff.NoProxy, unwanted) {
			t.Fatalf("wildcard entry %q must be stripped: %q", unwanted, strings.Join(eff.NoProxy, ","))
		}
	}
}

func TestResolveDedupsUserAndAutoEntries(t *testing.T) {
	env := &v1alpha1.Environment{
		Spec: v1alpha1.EnvironmentSpec{
			BaseDomain: "example.test",
			Proxy: &v1alpha1.EnvironmentProxySpec{
				HTTP:    "http://proxy.example.test:3128",
				NoProxy: []string{"localhost", ".example.test", "localhost"},
			},
		},
	}
	eff := Resolve(v1alpha1.State{}, env)
	count := 0
	for _, e := range eff.NoProxy {
		if e == "localhost" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one 'localhost' entry, got %d in %q", count, strings.Join(eff.NoProxy, ","))
	}
}

func TestResolveReadsAuthRef(t *testing.T) {
	env := &v1alpha1.Environment{
		Spec: v1alpha1.EnvironmentSpec{
			Proxy: &v1alpha1.EnvironmentProxySpec{
				HTTP: "http://proxy.example.test:3128",
				Auth: &v1alpha1.EnvironmentProxyAuthSpec{
					ProxyAuthRef: v1alpha1.SecretRef{Name: "proxy-auth"},
				},
			},
		},
	}
	eff := Resolve(v1alpha1.State{}, env)
	if eff.Auth.Name != "proxy-auth" {
		t.Fatalf("Auth.Name: got %q want proxy-auth", eff.Auth.Name)
	}
}

func TestIsManagedDetectsSquidProvider(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"host-01": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "10.0.0.1"}},
				},
			}},
			{Spec: v1alpha1.InfrastructureProviderSpec{
				Proxy: &v1alpha1.ProxyCapabilitySpec{
					Squid: &v1alpha1.ProxySquidSpec{
						HostRef: v1alpha1.LocalObjectReference{Name: "host-01"},
					},
				},
			}},
		},
	}
	if !IsManaged(state) {
		t.Fatal("expected IsManaged=true when a provider supplies spec.proxy.squid")
	}
}

func TestIsManagedFalseForExternalProxy(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				Proxy: &v1alpha1.EnvironmentProxySpec{HTTP: "http://proxy.example.test:3128"},
			},
		}},
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"host-01": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "10.0.0.1"}},
				},
			},
		}},
	}
	if IsManaged(state) {
		t.Fatal("expected IsManaged=false when no provider supplies spec.proxy.squid")
	}
}

func TestMirrorHostStripsPort(t *testing.T) {
	cases := map[string]string{
		"mirror.example.test:5000": "mirror.example.test",
		"mirror.example.test":      "mirror.example.test",
		"":                         "",
	}
	for in, want := range cases {
		if got := MirrorHost(in); got != want {
			t.Fatalf("MirrorHost(%q): got %q want %q", in, got, want)
		}
	}
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
