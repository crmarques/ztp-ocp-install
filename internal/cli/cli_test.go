package cli

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"validate", "-f", "../../examples/infra"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "validated 1 Environment, 3 InfrastructureProvider, 3 ClusterInfrastructure, 3 OCPCluster object(s)") {
		t.Fatalf("unexpected stdout: %s", got)
	}
}

func TestPlanCommandShowsInstallerAssets(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"plan", "-f", "../../examples/infra", "--state-dir", stateDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"installer assets:",
		"hub (agent):",
		filepath.Join(stateDir, "clusters", "hub", "installer", "install-config.yaml"),
		filepath.Join(stateDir, "clusters", "hub", "installer", "agent-config.yaml"),
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

func TestInitCommandGeneratesCurrentTemplate(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "desired-state")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"init", "--template", "libvirt-redfish-hub", "--out", outDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		filepath.Join(outDir, "environment.yaml"),
		filepath.Join(outDir, "provider.yaml"),
		filepath.Join(outDir, "cluster-infrastructure-hub.yaml"),
		filepath.Join(outDir, "ocp-cluster-hub.yaml"),
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
	var validateStdout bytes.Buffer
	code = Run(context.Background(), []string{"validate", "-f", outDir}, nil, &validateStdout, &stderr)
	if code != 0 {
		t.Fatalf("generated template should validate, code=%d stderr=%s", code, stderr.String())
	}
}

func TestGitupsHomeDefaultsToUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITUPS_HOME", "")

	gitupsHome := filepath.Join(home, ".gitups")
	if got := defaultGitupsHome(); got != gitupsHome {
		t.Fatalf("defaultGitupsHome got %q, want %q", got, gitupsHome)
	}
	if got := defaultStateDir(); got != filepath.Join(gitupsHome, "state") {
		t.Fatalf("defaultStateDir got %q", got)
	}
	if got := defaultSecretsDir(); got != filepath.Join(gitupsHome, "secrets") {
		t.Fatalf("defaultSecretsDir got %q", got)
	}
}

func TestGitupsHomeEnvOverridesUserHome(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "custom-gitups")
	t.Setenv("HOME", home)
	t.Setenv("GITUPS_HOME", override)

	if got := defaultGitupsHome(); got != override {
		t.Fatalf("defaultGitupsHome got %q, want %q", got, override)
	}
	if got := defaultStateDir(); got != filepath.Join(override, "state") {
		t.Fatalf("defaultStateDir got %q", got)
	}
	if got := defaultSecretsDir(); got != filepath.Join(override, "secrets") {
		t.Fatalf("defaultSecretsDir got %q", got)
	}
}

func TestApplyRejectsUnsupportedProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vmware.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: vmware-env
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
  name: vmware-provider
spec:
  machine:
    vsphere:
      vCenterRef:
        name: example-vcenter
      datacenter: example-dc
      cluster: example-cluster
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: vmware
spec:
  providerRefs:
    - name: vmware-provider
  networks:
    primary:
      cidr: 192.168.155.0/24
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.155.20
          macAddress: 52:54:00:00:00:20
      vsphere:
        folder: example/vmware
        template: rhcos-4.21
  endpoints:
    api:
      address: 192.168.155.10
    apiInt:
      address: 192.168.155.10
    ingress:
      address: 192.168.155.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: vmware
spec:
  topology: single-node
  infrastructureRef:
    name: vmware
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"apply", "infra", "-f", path, "--dry-run"}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected unsupported provider failure")
	}
	if !strings.Contains(stderr.String(), `apply does not yet support provider kind "vsphere"`) {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "supported: libvirt") {
		t.Fatalf("error must list the supported flavors, got: %s", stderr.String())
	}
}

func TestSecretsGenerateWritesSelfSignedCertificate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: secrets-env
spec:
  baseDomain: example.com
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: pull-secret
    clusterSSHKeyRef:
      name: ssh-key
  keys:
    registry-lab-ca:
      generated:
        selfSignedCertificate:
          commonName: registry.lab.test
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: provider
spec:
  hosts:
    host-01:
      ssh:
        address: localhost
        keyRef:
          name: default-key
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
  name: hub-infra
spec:
  providerRefs:
    - name: provider
  networks:
    primary:
      cidr: 192.168.155.0/24
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        enp1s0:
          networkRef:
            name: primary
          ipAddress: 192.168.155.20
      libvirt:
        hostRef:
          name: host-01
  endpoints:
    api:
      address: 192.168.155.10
    apiInt:
      address: 192.168.155.10
    ingress:
      address: 192.168.155.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  topology: single-node
  infrastructureRef:
    name: hub-infra
  install:
    method: agent
    additionalTrustBundleRef:
      name: registry-lab-ca
  nodes:
    master-0:
      role: control-plane
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"secrets", "generate", "-f", path, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	certPath := filepath.Join(secretsDir, "registry-lab-ca")
	keyPath := certPath + ".key"
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("stat key: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("generated certificate is not PEM:\n%s", certPEM)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if got, want := cert.Subject.CommonName, "registry.lab.test"; got != want {
		t.Fatalf("common name got %q, want %q", got, want)
	}
	if got, want := strings.Join(cert.DNSNames, ","), "registry.lab.test"; got != want {
		t.Fatalf("DNSNames got %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"secrets", "generate", "-f", path, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second run code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "reused existing certificate and key") {
		t.Fatalf("expected reuse output, got %s", stdout.String())
	}

	driftPath := filepath.Join(dir, "state-drift.yaml")
	if err := os.WriteFile(driftPath, bytes.ReplaceAll(mustReadFile(t, path), []byte("registry.lab.test"), []byte("registry.other.test")), 0o644); err != nil {
		t.Fatalf("write drift fixture: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"secrets", "generate", "-f", driftPath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected drift to fail, stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no longer matches the desired spec") {
		t.Fatalf("expected drift error, stderr: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "remove") {
		t.Fatalf("expected remediation hint, stderr: %s", stderr.String())
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

const generatedCredentialsFixture = `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: cred-env
spec:
  baseDomain: example.com
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: pull-secret
    clusterSSHKeyRef:
      name: ssh-key
  keys:
    bmc-credentials:
      generated:
        credentials:
          username: admin
    mirror-creds:
      generated:
        credentials: {}
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: provider
spec:
  hosts:
    host-01:
      ssh:
        address: localhost
        keyRef:
          name: default-key
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
  name: hub-infra
spec:
  providerRefs:
    - name: provider
  networks:
    primary:
      cidr: 192.168.155.0/24
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        enp1s0:
          networkRef:
            name: primary
          ipAddress: 192.168.155.20
      libvirt:
        hostRef:
          name: host-01
  endpoints:
    api:
      address: 192.168.155.10
    apiInt:
      address: 192.168.155.10
    ingress:
      address: 192.168.155.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  topology: single-node
  infrastructureRef:
    name: hub-infra
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`

func TestSecretsGenerateMaterializesCredentials(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(statePath, []byte(generatedCredentialsFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"secrets", "generate", "-f", statePath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("first run code=%d stderr=%s", code, stderr.String())
	}
	for _, name := range []string{"bmc-credentials", "mirror-creds"} {
		body, err := os.ReadFile(filepath.Join(secretsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := strings.TrimRight(string(body), "\n")
		parts := strings.SplitN(text, ":", 2)
		if len(parts) != 2 {
			t.Fatalf("%s: expected user:pass, got %q", name, text)
		}
		// Both keys default username to "admin" — explicit on bmc-credentials,
		// defaulted by normalize for mirror-creds.
		if parts[0] != "admin" {
			t.Fatalf("%s: username got %q, want admin", name, parts[0])
		}
		if len(parts[1]) < 24 {
			t.Fatalf("%s: password too short (%d chars), want a strong random", name, len(parts[1]))
		}
		if info, err := os.Stat(filepath.Join(secretsDir, name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		} else if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s mode got %v, want 0600", name, mode)
		}
	}

	// Idempotent: rerunning preserves the existing password.
	bmcBefore, err := os.ReadFile(filepath.Join(secretsDir, "bmc-credentials"))
	if err != nil {
		t.Fatalf("read bmc before: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"secrets", "generate", "-f", statePath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("second run code=%d stderr=%s", code, stderr.String())
	}
	bmcAfter, err := os.ReadFile(filepath.Join(secretsDir, "bmc-credentials"))
	if err != nil {
		t.Fatalf("read bmc after: %v", err)
	}
	if !bytes.Equal(bmcBefore, bmcAfter) {
		t.Fatal("expected credentials to be reused on second run, got rewrite")
	}
	if !strings.Contains(stdout.String(), "reused existing credentials") {
		t.Fatalf("expected reuse output, got %s", stdout.String())
	}

	// Username drift fails fast with a remediation hint.
	driftFixture := strings.Replace(generatedCredentialsFixture, "username: admin", "username: operator", 1)
	driftPath := filepath.Join(dir, "drift.yaml")
	if err := os.WriteFile(driftPath, []byte(driftFixture), 0o644); err != nil {
		t.Fatalf("write drift fixture: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code := Run(context.Background(), []string{"secrets", "generate", "-f", driftPath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected username drift to fail, stdout=%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "operator") || !strings.Contains(stderr.String(), "remove") {
		t.Fatalf("expected drift remediation hint, stderr=%s", stderr.String())
	}
}

func TestSecretsPullSecretSetWritesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "pull-secret.json")
	first := []byte(`{"auths":{"registry.example.com":{"auth":"redacted"}}}` + "\n")
	if err := os.WriteFile(source, first, 0o644); err != nil {
		t.Fatalf("write pull secret source: %v", err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"secrets", "pull-secret", "set",
		"--name", "openshift-pull-secret",
		"--from-file", source,
		"--secrets-dir", secretsDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	target := filepath.Join(secretsDir, "openshift-pull-secret")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read stored pull secret: %v", err)
	}
	if !bytes.Equal(data, first) {
		t.Fatalf("stored pull secret got %q, want %q", data, first)
	}
	if info, err := os.Stat(secretsDir); err != nil {
		t.Fatalf("stat secrets dir: %v", err)
	} else if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("secrets dir mode got %v, want %v", got, want)
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("stat pull secret: %v", err)
	} else if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("pull secret mode got %v, want %v", got, want)
	}
	if strings.Contains(stdout.String(), "registry.example.com") || strings.Contains(stderr.String(), "registry.example.com") {
		t.Fatalf("command output leaked pull secret content\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	second := []byte(`{"auths":{"registry.example.com":{"auth":"updated"}}}` + "\n")
	if err := os.WriteFile(source, second, 0o644); err != nil {
		t.Fatalf("rewrite pull secret source: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"secrets", "pull-secret", "set",
		"--name", "openshift-pull-secret",
		"--from-file", source,
		"--secrets-dir", secretsDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second code got %d, stderr: %s", code, stderr.String())
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("read updated pull secret: %v", err)
	}
	if !bytes.Equal(data, second) {
		t.Fatalf("updated pull secret got %q, want %q", data, second)
	}
	if !strings.Contains(stdout.String(), "updated pull secret") {
		t.Fatalf("stdout missing update message: %s", stdout.String())
	}
}

func TestSecretsPullSecretSetRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "pull-secret.json")
	if err := os.WriteFile(source, []byte(`{"auths":[]}`), 0o644); err != nil {
		t.Fatalf("write invalid pull secret: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"secrets", "pull-secret", "set",
		"--name", "openshift-pull-secret",
		"--from-file", source,
		"--secrets-dir", filepath.Join(dir, "secrets"),
	}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("invalid JSON shape code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), ".auths must be a JSON object") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"secrets", "pull-secret", "set",
		"--name", "../pull-secret",
		"--from-file", source,
		"--secrets-dir", filepath.Join(dir, "secrets"),
	}, nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("invalid ref code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name must be a lowercase DNS label") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestSecretsSyncSymlinksSSHKeyAndCopiesPullSecret(t *testing.T) {
	dir := t.TempDir()
	sshSource := filepath.Join(dir, "id_demo")
	if err := os.WriteFile(sshSource, []byte("PRIVATE-KEY\n"), 0o600); err != nil {
		t.Fatalf("write ssh source: %v", err)
	}
	pullSource := filepath.Join(dir, "pull-secret.json")
	pullBytes := []byte(`{"auths":{"registry.example.com":{"auth":"redacted"}}}` + "\n")
	if err := os.WriteFile(pullSource, pullBytes, 0o644); err != nil {
		t.Fatalf("write pull source: %v", err)
	}
	statePath := filepath.Join(dir, "state.yaml")
	state := `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: sync-env
spec:
  baseDomain: example.com
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: pull-secret
    clusterSSHKeyRef:
      name: cluster-ssh-key
  keys:
    cluster-ssh-key:
      file: ` + sshSource + `
    pull-secret:
      file: ` + pullSource + `
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: provider
spec:
  hosts:
    host-01:
      ssh:
        address: localhost
        keyRef:
          name: cluster-ssh-key
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
  name: hub-infra
spec:
  providerRefs:
    - name: provider
  networks:
    primary:
      cidr: 192.168.155.0/24
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        enp1s0:
          networkRef:
            name: primary
          ipAddress: 192.168.155.20
      libvirt:
        hostRef:
          name: host-01
  endpoints:
    api:
      address: 192.168.155.10
    apiInt:
      address: 192.168.155.10
    ingress:
      address: 192.168.155.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  topology: single-node
  infrastructureRef:
    name: hub-infra
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"secrets", "sync", "-f", statePath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first sync code got %d, stderr: %s", code, stderr.String())
	}
	sshTarget := filepath.Join(secretsDir, "cluster-ssh-key")
	link, err := os.Readlink(sshTarget)
	if err != nil {
		t.Fatalf("readlink ssh target: %v", err)
	}
	if link != sshSource {
		t.Fatalf("ssh link got %q, want %q", link, sshSource)
	}
	pullTarget := filepath.Join(secretsDir, "pull-secret")
	if info, err := os.Lstat(pullTarget); err != nil {
		t.Fatalf("lstat pull target: %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("pull-secret should be a regular file, got symlink")
	}
	gotPull, err := os.ReadFile(pullTarget)
	if err != nil {
		t.Fatalf("read pull target: %v", err)
	}
	if !bytes.Equal(gotPull, pullBytes) {
		t.Fatalf("pull bytes got %q, want %q", gotPull, pullBytes)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"secrets", "sync", "-f", statePath, "--secrets-dir", secretsDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second sync code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "up-to-date") {
		t.Fatalf("second sync stdout missing up-to-date: %s", stdout.String())
	}
}

func TestSecretsBMCSetGeneratesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	secretsDir := filepath.Join(dir, "secrets")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"secrets", "credentials", "set",
		"--name", "lab-bmc",
		"--generate",
		"--secrets-dir", secretsDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	target := filepath.Join(secretsDir, "lab-bmc")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read stored bmc credentials: %v", err)
	}
	line := strings.TrimRight(string(data), "\n")
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("stored credentials %q is not username:password", line)
	}
	if parts[0] != "admin" {
		t.Fatalf("default --generate username got %q, want %q", parts[0], "admin")
	}
	if len(parts[1]) < 16 {
		t.Fatalf("generated password is suspiciously short: %d chars", len(parts[1]))
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("stat bmc credentials: %v", err)
	} else if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("bmc credentials mode got %v, want %v", got, want)
	}
	if strings.Contains(stdout.String(), parts[1]) || strings.Contains(stderr.String(), parts[1]) {
		t.Fatalf("command output leaked generated password\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"secrets", "credentials", "set",
		"--name", "lab-bmc",
		"--username", "operator",
		"--password", "hunter2",
		"--secrets-dir", secretsDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("overwrite code got %d, stderr: %s", code, stderr.String())
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("read overwritten credentials: %v", err)
	}
	if got := strings.TrimRight(string(data), "\n"); got != "operator:hunter2" {
		t.Fatalf("overwritten credentials got %q, want %q", got, "operator:hunter2")
	}
	if !strings.Contains(stdout.String(), "updated credentials") {
		t.Fatalf("stdout missing update message: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "hunter2") || strings.Contains(stderr.String(), "hunter2") {
		t.Fatalf("command output leaked password literal\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestSecretsBMCSetFromFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "bmc.creds")
	if err := os.WriteFile(source, []byte("operator:hunter2\n"), 0o600); err != nil {
		t.Fatalf("write bmc source: %v", err)
	}
	secretsDir := filepath.Join(dir, "secrets")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"secrets", "credentials", "set",
		"--name", "lab-bmc",
		"--from-file", source,
		"--secrets-dir", secretsDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(secretsDir, "lab-bmc"))
	if err != nil {
		t.Fatalf("read stored credentials: %v", err)
	}
	if got := strings.TrimRight(string(data), "\n"); got != "operator:hunter2" {
		t.Fatalf("stored credentials got %q, want %q", got, "operator:hunter2")
	}
}

func TestSecretsBMCSetRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	secretsDir := filepath.Join(dir, "secrets")
	cases := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{
			name:     "missing-input-mode",
			args:     []string{"secrets", "credentials", "set", "--name", "lab-bmc", "--secrets-dir", secretsDir},
			wantCode: 2,
			want:     "one of --from-file, --password, --password-stdin, or --generate is required",
		},
		{
			name:     "conflicting-input-mode",
			args:     []string{"secrets", "credentials", "set", "--name", "lab-bmc", "--password", "x", "--generate", "--secrets-dir", secretsDir},
			wantCode: 2,
			want:     "mutually exclusive",
		},
		{
			name:     "password-without-username",
			args:     []string{"secrets", "credentials", "set", "--name", "lab-bmc", "--password", "x", "--secrets-dir", secretsDir},
			wantCode: 2,
			want:     "--username is required with --password",
		},
		{
			name:     "invalid-name",
			args:     []string{"secrets", "credentials", "set", "--name", "../bmc", "--generate", "--secrets-dir", secretsDir},
			wantCode: 2,
			want:     "--name must be a lowercase DNS label",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, nil, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("code got %d, want %d, stderr: %s", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("expected stderr to contain %q, got %s", tt.want, stderr.String())
			}
		})
	}
}

func TestApplyDryRunRendersAndPrintsAnsibleCommandsForAllPhases(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "infra",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"rendered:",
		filepath.Join(stateDir, "ansible", "inventory.yaml"),
		"dry-run ansible command [phase=provider]: ansible-playbook",
		"playbooks/provider-prepare.yml",
		"dry-run ansible command [phase=cluster]: ansible-playbook",
		"playbooks/cluster-prepare.yml",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{"ocp-install.yml", "gitops-publish.yml"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("infra apply leaked %q\n%s", unexpected, output)
		}
	}
}

func TestApplyDryRunPassesStateSecretsAndHostStateDirs(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	secretsDir := filepath.Join(root, "secrets")
	hostStateDir := filepath.Join(root, "host-state")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "infra",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--secrets-dir", secretsDir,
		"--host-state-dir", hostStateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"gitups_state_dir=" + stateDir,
		"gitups_secrets_dir=" + secretsDir,
		"gitups_host_state_dir=" + hostStateDir,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

func TestAnsibleUsesHostStateDirVariable(t *testing.T) {
	err := filepath.Walk("../../ansible", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".yml" && filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "/var/lib/gitups") {
			t.Fatalf("%s contains hard-coded /var/lib/gitups; use gitups_host_state_dir", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ansible: %v", err)
	}
}

func TestApplyHubDryRunOnlyRunsHubScope(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "ocp",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "playbooks/ocp-install.yml") {
		t.Fatalf("stdout missing ocp-install.yml: %s", output)
	}
	for _, leaked := range []string{"provider-prepare.yml", "cluster-prepare.yml", "gitops-publish.yml"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("hub-scope apply leaked %s:\n%s", leaked, output)
		}
	}
}

func TestDestroyRequiresYesOrDryRun(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"destroy", "all",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
	}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected destroy without --yes/--dry-run to fail")
	}
	if !strings.Contains(stderr.String(), "destroy refused") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestDestroyDryRunPrintsPhasesInReverse(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"destroy", "all",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	hubIdx := strings.Index(output, "ocp-destroy.yml")
	clusterIdx := strings.Index(output, "cluster-destroy.yml")
	providerIdx := strings.Index(output, "provider-destroy.yml")
	if hubIdx < 0 || clusterIdx < 0 || providerIdx < 0 {
		t.Fatalf("missing destroy phase entries:\n%s", output)
	}
	if !(hubIdx < clusterIdx && clusterIdx < providerIdx) {
		t.Fatalf("destroy phases not in reverse order (hub=%d cluster=%d provider=%d)\n%s", hubIdx, clusterIdx, providerIdx, output)
	}
	if strings.Contains(output, "gitops-unpublish.yml") {
		t.Fatalf("destroy all must not include unfinished gitops publication teardown:\n%s", output)
	}
	if !strings.Contains(output, "dry-run: would remove state-dir: "+stateDir) {
		t.Fatalf("dry-run destroy must announce state-dir removal:\n%s", output)
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("dry-run destroy must not remove state-dir: %v", err)
	}
}

func TestDestroyRemovesStateDirOnSuccess(t *testing.T) {
	if _, err := os.Stat("/bin/true"); err != nil {
		t.Skip("/bin/true not available")
	}
	stubSudoReadyAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"destroy", "all",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--yes",
		"--ansible-playbook", "/bin/true",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "removed state-dir: "+stateDir) {
		t.Fatalf("destroy must announce state-dir removal:\n%s", stdout.String())
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("state-dir should be removed, stat err=%v", err)
	}
}

func TestDestroyScopedHubKeepsStateDir(t *testing.T) {
	if _, err := os.Stat("/bin/true"); err != nil {
		t.Skip("/bin/true not available")
	}
	stubSudoReadyAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"destroy", "ocp",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--yes",
		"--ansible-playbook", "/bin/true",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "removed state-dir") {
		t.Fatalf("scoped destroy must not remove state-dir:\n%s", stdout.String())
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("state-dir should remain, stat err=%v", err)
	}
}

func TestDestroyKeepStateDirFlagPreservesStateDir(t *testing.T) {
	if _, err := os.Stat("/bin/true"); err != nil {
		t.Skip("/bin/true not available")
	}
	stubSudoReadyAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"destroy", "all",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--yes",
		"--keep-state-dir",
		"--ansible-playbook", "/bin/true",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "removed state-dir") {
		t.Fatalf("--keep-state-dir must not remove state-dir:\n%s", stdout.String())
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("state-dir should remain, stat err=%v", err)
	}
}

func TestStatusReportsRenderedPresence(t *testing.T) {
	stateDir := t.TempDir()
	if code := Run(context.Background(), []string{
		"apply", "infra",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("render setup failed")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"status",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"desired:",
		"phases:",
		"- provider: apply=playbooks/provider-prepare.yml destroy=playbooks/provider-destroy.yml",
		"- cluster: apply=playbooks/cluster-prepare.yml destroy=playbooks/cluster-destroy.yml",
		"0 missing",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

func TestDiffReportsDriftWhenStateIsStale(t *testing.T) {
	stateDir := t.TempDir()
	if code := Run(context.Background(), []string{
		"apply", "infra",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("render setup failed")
	}
	inventory := filepath.Join(stateDir, "ansible", "inventory.yaml")
	if err := os.WriteFile(inventory, []byte("tampered: true\n"), 0o644); err != nil {
		t.Fatalf("tamper inventory: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"status",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--diff",
	}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected drift exit=1, got %d, stdout: %s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "drift detected") {
		t.Fatalf("stdout missing drift marker: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "ansible/inventory.yaml") {
		t.Fatalf("stdout missing changed file: %s", stdout.String())
	}
}

func TestDiffReportsNoDriftAfterFreshRender(t *testing.T) {
	stateDir := t.TempDir()
	if code := Run(context.Background(), []string{
		"apply", "infra",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("render setup failed")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"status",
		"-f", "../../examples/infra",
		"--state-dir", stateDir,
		"--diff",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected no drift, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "no drift") {
		t.Fatalf("stdout missing no-drift marker: %s", stdout.String())
	}
}

// TestApplyDryRunDescribesSudoExpectation pins the user-facing escalation
// summary so the gitups-runs-as-root / NOPASSWD-sudo expectation is always
// announced before any phase runs. This replaced the old
// `--ask-become-pass` flag and its in-process password file: gitups no
// longer reads or stores a sudo password.
func TestApplyDryRunDescribesSudoExpectation(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "ocp",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "ANSIBLE_BECOME_PASSWORD_FILE") {
		t.Fatalf("stdout must not reference ANSIBLE_BECOME_PASSWORD_FILE; gitups no longer writes a password file\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "--ask-become-pass") {
		t.Fatalf("stdout must not reference --ask-become-pass; flag was removed\n%s", stdout.String())
	}
}

func TestApplyDryRunPrintsEscalationSummary(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "ocp",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"apply plan:",
		"- ocp [root]",
		"[root] phases require sudo escalation",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

func stubListenAlwaysFree(t *testing.T) {
	t.Helper()
	original := defaultPreflightDeps.tryListen
	defaultPreflightDeps.tryListen = func(_ string, _ string) error { return nil }
	t.Cleanup(func() { defaultPreflightDeps.tryListen = original })
}

// stubPreflightAlwaysOK lets tests that exercise downstream apply flow assume
// preflight passes: TCP listeners are free and any lookup under the default
// secrets directory reports a regular file. Other statPath callers (/dev/kvm,
// host_state_dir) fall through to the real os.Stat so KVM-aware tests still
// observe the host's true capability.
func stubPreflightAlwaysOK(t *testing.T) {
	t.Helper()
	origListen := defaultPreflightDeps.tryListen
	origStat := defaultPreflightDeps.statPath
	secretsDir := defaultSecretsDir()
	defaultPreflightDeps.tryListen = func(_ string, _ string) error { return nil }
	defaultPreflightDeps.statPath = func(path string) (os.FileInfo, error) {
		if path == secretsDir {
			return fakeFileInfo{name: filepath.Base(path), isDir: true}, nil
		}
		if strings.HasPrefix(path, secretsDir+string(os.PathSeparator)) {
			return fakeFileInfo{name: filepath.Base(path)}, nil
		}
		return origStat(path)
	}
	t.Cleanup(func() {
		defaultPreflightDeps.tryListen = origListen
		defaultPreflightDeps.statPath = origStat
	})
}

// stubSudoReadyAlwaysOK pretends the controller already satisfies the
// "running as root or NOPASSWD sudo" precondition that ensureSudoReady
// otherwise enforces. CI hosts typically prompt for sudo, so without this
// stub apply / setup tests would all exit at the gate.
func stubSudoReadyAlwaysOK(t *testing.T) {
	t.Helper()
	orig := ensureSudoReady
	ensureSudoReady = func() error { return nil }
	t.Cleanup(func() { ensureSudoReady = orig })
}

func TestApplyConfirmationDecline(t *testing.T) {
	stubPreflightAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "ocp",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
	}, strings.NewReader("n\n"), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 on decline, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "apply aborted") {
		t.Fatalf("stderr missing 'apply aborted': %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "rendered:") {
		t.Fatalf("decline should abort before render:\n%s", stdout.String())
	}
}

func TestApplyYesSkipsConfirmationAndStopsBeforeAnsible(t *testing.T) {
	stubPreflightAlwaysOK(t)
	stubSudoReadyAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"apply", "ocp",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--yes",
		"--ansible-playbook", "/nonexistent/ansible-playbook",
	}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected ansible exec failure, stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "apply plan:") {
		t.Fatalf("--yes path still must print summary\n%s", stdout.String())
	}
	if strings.Contains(stderr.String(), "apply aborted") {
		t.Fatalf("--yes should skip confirmation, stderr: %s", stderr.String())
	}
}
