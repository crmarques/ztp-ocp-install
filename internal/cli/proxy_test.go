package cli

import (
	"os"
	"path/filepath"
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
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Restricted: &v1alpha1.RestrictedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{
							HTTPProxy:  "http://proxy.lab.test:3128",
							HTTPSProxy: "http://proxy.lab.test:3128",
							NoProxy:    []string{".lab.test", "10.0.0.0/8"},
						},
					},
				},
			},
		}},
	}
	got, err := resolveProxyEnv(state, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"HTTP_PROXY":  "http://proxy.lab.test:3128",
		"http_proxy":  "http://proxy.lab.test:3128",
		"HTTPS_PROXY": "http://proxy.lab.test:3128",
		"https_proxy": "http://proxy.lab.test:3128",
		"NO_PROXY":    ".lab.test,10.0.0.0/8",
		"no_proxy":    ".lab.test,10.0.0.0/8",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("key %q: got %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected extra keys: %+v", got)
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
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Disconnected: &v1alpha1.DisconnectedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{
							HTTPProxy:      "http://proxy.lab.test:3128",
							HTTPSProxy:     "https://proxy.lab.test:3129",
							CredentialsRef: v1alpha1.SecretRef{Name: "proxy-credentials"},
						},
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
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Restricted: &v1alpha1.RestrictedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{
							HTTPProxy:      "http://proxy.lab.test:3128",
							CredentialsRef: v1alpha1.SecretRef{Name: "missing"},
						},
					},
				},
			},
		}},
	}
	if _, err := resolveProxyEnv(state, t.TempDir()); err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestResolveProxyEnvSkipsEmptyProxy(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Restricted: &v1alpha1.RestrictedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{},
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
