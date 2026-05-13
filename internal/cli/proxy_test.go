package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func TestResolveProxyEnvNoEnvironmentsReturnsNil(t *testing.T) {
	got, err := resolveProxyEnv(v1alpha1.State{}, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestResolveProxyEnvWithoutCredentials(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstallType: v1alpha1.OCPInstallKindConnected,
				Proxy: &v1alpha1.EnvironmentProxySpec{
					HTTP:    "http://proxy.lab.test:3128",
					HTTPS:   "http://proxy.lab.test:3128",
					NoProxy: []string{".lab.test", "10.0.0.0/8"},
				},
			},
		}},
	}
	got, err := resolveProxyEnv(state, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["HTTP_PROXY"] != "http://proxy.lab.test:3128" {
		t.Fatalf("HTTP_PROXY: got %q", got["HTTP_PROXY"])
	}
	if got["HTTPS_PROXY"] != "http://proxy.lab.test:3128" {
		t.Fatalf("HTTPS_PROXY: got %q", got["HTTPS_PROXY"])
	}
	for _, want := range []string{".lab.test", "10.0.0.0/8", "localhost", "127.0.0.1", ".svc", ".cluster.local"} {
		if !strings.Contains(got["NO_PROXY"], want) {
			t.Fatalf("NO_PROXY missing %q: got %q", want, got["NO_PROXY"])
		}
	}
	if got["no_proxy"] != got["NO_PROXY"] {
		t.Fatalf("lowercase no_proxy must match uppercase: %q vs %q", got["no_proxy"], got["NO_PROXY"])
	}
}

func TestResolveProxyEnvInjectsCredentials(t *testing.T) {
	secretsDir := t.TempDir()
	credPath := filepath.Join(secretsDir, "proxy-credentials")
	if err := os.WriteFile(credPath, []byte("alice:p@ss/word\n"), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstallType: v1alpha1.OCPInstallKindDisconnected,
				Proxy: &v1alpha1.EnvironmentProxySpec{
					HTTP:  "http://proxy.lab.test:3128",
					HTTPS: "https://proxy.lab.test:3129",
					Auth: &v1alpha1.EnvironmentProxyAuthSpec{
						ProxyAuthRef: v1alpha1.SecretRef{Name: "proxy-credentials"},
					},
				},
			},
		}},
	}
	got, err := resolveProxyEnv(state, secretsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantHTTP := "http://alice:p%40ss%2Fword@proxy.lab.test:3128"
	if got["HTTP_PROXY"] != wantHTTP {
		t.Fatalf("HTTP_PROXY: got %q want %q", got["HTTP_PROXY"], wantHTTP)
	}
	wantHTTPS := "https://alice:p%40ss%2Fword@proxy.lab.test:3129"
	if got["HTTPS_PROXY"] != wantHTTPS {
		t.Fatalf("HTTPS_PROXY: got %q want %q", got["HTTPS_PROXY"], wantHTTPS)
	}
}

func TestResolveProxyEnvMissingCredentialsFails(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstallType: v1alpha1.OCPInstallKindConnected,
				Proxy: &v1alpha1.EnvironmentProxySpec{
					HTTP: "http://proxy.lab.test:3128",
					Auth: &v1alpha1.EnvironmentProxyAuthSpec{
						ProxyAuthRef: v1alpha1.SecretRef{Name: "missing"},
					},
				},
			},
		}},
	}
	if _, err := resolveProxyEnv(state, t.TempDir()); err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestResolveProxyEnvSkipsManagedProxy(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstallType: v1alpha1.OCPInstallKindConnected,
				Proxy: &v1alpha1.EnvironmentProxySpec{
					HTTP:  "http://192.168.130.1:3128",
					HTTPS: "http://192.168.130.1:3128",
				},
			},
		}},
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Spec: v1alpha1.InfrastructureProviderSpec{
				Proxy: &v1alpha1.ProxyCapabilitySpec{
					Squid: &v1alpha1.ProxySquidSpec{
						HostRef: v1alpha1.LocalObjectReference{Name: "host-01"},
					},
				},
			},
		}},
	}
	got, err := resolveProxyEnv(state, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for managed proxy (bastion provisions it), got %+v", got)
	}
}

func TestResolveProxyEnvSkipsEmptyProxy(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstallType: v1alpha1.OCPInstallKindConnected,
			},
		}},
	}
	got, err := resolveProxyEnv(state, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty proxy, got %+v", got)
	}
}

func TestRedactProxyURLStripsCredentials(t *testing.T) {
	cases := map[string]string{
		"http://user:secret@proxy.test:3128": "http://***@proxy.test:3128",
		"https://proxy.test:3129":            "https://proxy.test:3129",
		"":                                   "",
	}
	for in, want := range cases {
		if got := redactProxyURL(in); got != want {
			t.Fatalf("redact %q: got %q want %q", in, got, want)
		}
	}
}

func TestMergeEnvOverridesDuplicates(t *testing.T) {
	base := []string{"HOME=/root", "HTTP_PROXY=old", "PATH=/usr/bin"}
	got := mergeEnv(base, map[string]string{"HTTP_PROXY": "new", "NO_PROXY": "localhost"})
	seen := map[string]string{}
	for _, kv := range got {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				seen[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	if seen["HTTP_PROXY"] != "new" {
		t.Fatalf("HTTP_PROXY not overridden: %q", seen["HTTP_PROXY"])
	}
	if seen["HOME"] != "/root" || seen["PATH"] != "/usr/bin" {
		t.Fatalf("unrelated keys lost: %+v", seen)
	}
	if seen["NO_PROXY"] != "localhost" {
		t.Fatalf("NO_PROXY not added: %q", seen["NO_PROXY"])
	}
}

func TestMergeBootstrapEnvStripsAmbientProxyEnv(t *testing.T) {
	base := []string{
		"HOME=/root",
		"HTTP_PROXY=http://proxy.example.test:3128",
		"HTTPS_PROXY=http://proxy.example.test:3128",
		"NO_PROXY=localhost",
		"http_proxy=http://proxy.example.test:3128",
		"https_proxy=http://proxy.example.test:3128",
		"no_proxy=localhost",
		"ALL_PROXY=http://proxy.example.test:3128",
		"all_proxy=http://proxy.example.test:3128",
		"PATH=/usr/bin",
	}
	got := mergeBootstrapEnv(base, nil)
	seen := map[string]string{}
	for _, kv := range got {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 {
			seen[kv[:eq]] = kv[eq+1:]
		}
	}
	if seen["HOME"] != "/root" || seen["PATH"] != "/usr/bin" {
		t.Fatalf("unrelated env keys lost: %+v", seen)
	}
	for key := range bootstrapProxyEnvKeys {
		if _, ok := seen[key]; ok {
			t.Fatalf("ambient proxy key %s was not stripped: %+v", key, seen)
		}
	}
}

func TestMergeBootstrapEnvUsesResolvedProxyEnv(t *testing.T) {
	base := []string{"HOME=/root", "HTTP_PROXY=http://proxy.example.test:3128", "PATH=/usr/bin"}
	got := mergeBootstrapEnv(base, map[string]string{
		"HTTP_PROXY": "http://proxy.lab.test:3128",
		"NO_PROXY":   "localhost,127.0.0.1",
	})
	seen := map[string]string{}
	for _, kv := range got {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 {
			seen[kv[:eq]] = kv[eq+1:]
		}
	}
	if seen["HTTP_PROXY"] != "http://proxy.lab.test:3128" {
		t.Fatalf("resolved HTTP_PROXY not applied: %+v", seen)
	}
	if seen["NO_PROXY"] != "localhost,127.0.0.1" {
		t.Fatalf("resolved NO_PROXY not applied: %+v", seen)
	}
	if seen["HOME"] != "/root" || seen["PATH"] != "/usr/bin" {
		t.Fatalf("unrelated env keys lost: %+v", seen)
	}
}
