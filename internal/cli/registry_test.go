package cli

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryVerifyHitsMirrorEndpoints(t *testing.T) {
	hits := map[string]int{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/openshift/release-images/manifests/4.21.10-x86_64", func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	})
	mux.HandleFunc("/v2/openshift/release/tags/list", func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"openshift/release","tags":[]}`))
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	bare := bareHostFromTestServer(t, server)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "input.yaml"), buildVerifyFixture(bare))
	secretsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "registry-lab-credentials"), []byte("user:pass\n"), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"registry", "verify",
		"-f", dir,
		"--secrets-dir", secretsDir,
		"--insecure-skip-tls-verify",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry verify exit got %d, stderr: %s, stdout: %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "all 2 image(s) reachable") {
		t.Fatalf("expected pass message, got:\n%s", stdout.String())
	}
	if hits["/v2/openshift/release-images/manifests/4.21.10-x86_64"] == 0 {
		t.Fatalf("expected manifest probe, hits=%v", hits)
	}
	if hits["/v2/openshift/release/tags/list"] == 0 {
		t.Fatalf("expected tag-list probe, hits=%v", hits)
	}
}

func TestRegistryVerifyReportsMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/openshift/release-images/manifests/4.21.10-x86_64", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/v2/openshift/release/tags/list", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	bare := bareHostFromTestServer(t, server)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "input.yaml"), buildVerifyFixture(bare))
	secretsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "registry-lab-credentials"), []byte("user:pass\n"), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"registry", "verify",
		"-f", dir,
		"--secrets-dir", secretsDir,
		"--insecure-skip-tls-verify",
	}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("registry verify exit got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "image(s) missing") {
		t.Fatalf("expected missing-image stderr, got: %s", stderr.String())
	}
}

func TestRegistryVerifyRejectsConnectedEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "input.yaml"), validStateYAMLForCLI("connected-env", "p", "192.168.190.0/24", "192.168.190.10", "192.168.190.11", "192.168.190.20"))
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"registry", "verify",
		"-f", dir,
	}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit=1 for connected env, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "registry verify only applies to disconnected") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func bareHostFromTestServer(t *testing.T, server *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return net.JoinHostPort(host, port)
}

func buildVerifyFixture(bare string) string {
	return `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: lab
spec:
  baseDomain: example.test
  ocpInstall:
    disconnected:
      registries:
        mirror:
          url: ` + bare + `
          credentialsRef:
            name: registry-lab-credentials
          trustBundle:
            generatedSelfSigned:
              secretRef:
                name: registry-lab-ca
              commonName: ` + bare + `
        imageDigestSources:
          - source: quay.io/openshift-release-dev/ocp-release
            mirrors:
              - ` + bare + `/openshift/release-images
          - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
            mirrors:
              - ` + bare + `/openshift/release
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.10
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: p
spec:
  hosts:
    h:
      ssh:
        address: 10.0.0.1
        keyRef:
          name: k
      capabilities:
        - libvirt
  machine:
    libvirt:
      hostRefs:
        - name: h
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: c
spec:
  providerRefs:
    - name: p
  networks:
    primary:
      cidr: 192.168.180.0/24
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.180.20
      libvirt:
        hostRef:
          name: h
  endpoints:
    api:
      address: 192.168.180.10
    apiInt:
      address: 192.168.180.10
    ingress:
      address: 192.168.180.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: c
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: c
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`
}

// validStateYAMLForCLI mirrors the infra-package factory but is local to the
// cli test package. Connected environments share the same base scaffold.
func validStateYAMLForCLI(name, providerName, cidr, apiVIP, ingressVIP, nodeIP string) string {
	envName := "env-" + name
	return `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: ` + envName + `
spec:
  baseDomain: example.com
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: pull-secret
    clusterSSHKeyRef:
      name: ssh-key
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: ` + providerName + `
spec:
  hosts:
    host-01:
      ssh:
        address: 10.0.0.1
        keyRef:
          name: k
      capabilities:
        - libvirt
  machine:
    libvirt:
      hostRefs:
        - name: host-01
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: ` + name + `
spec:
  providerRefs:
    - name: ` + providerName + `
  networks:
    primary:
      cidr: ` + cidr + `
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: ` + nodeIP + `
      libvirt:
        hostRef:
          name: host-01
  endpoints:
    api:
      address: ` + apiVIP + `
    apiInt:
      address: ` + apiVIP + `
    ingress:
      address: ` + ingressVIP + `
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: ` + name + `
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: ` + name + `
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
