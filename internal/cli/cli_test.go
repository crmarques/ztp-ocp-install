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

func clearGitupsEnv(t *testing.T) {
	t.Helper()
	t.Setenv(gitupsUserDirEnv, "")
	t.Setenv(gitupsStateDirEnv, "")
	t.Setenv(gitupsSecretsDirEnv, "")
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
}

func TestGitopsInitScaffoldPassesCheck(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "gitops")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"gitops", "init", "demo", "-d", outDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init code got %d, stderr: %s", code, stderr.String())
	}
	path := filepath.Join(outDir, "demo", "gitops-package-set.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scaffold missing: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"gitops", "check", "demo", "-d", outDir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("check code got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "init scaffold") {
		t.Fatalf("check should identify scaffold, stderr: %s", stderr.String())
	}
}

func TestGitupsUserDirDefaultsToUserHome(t *testing.T) {
	clearGitupsEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Join(home, ".gitups")
	if got := defaultGitupsUserDir(); got != userDir {
		t.Fatalf("defaultGitupsUserDir got %q, want %q", got, userDir)
	}
	if got := defaultStateDir(); got != filepath.Join(userDir, "state") {
		t.Fatalf("defaultStateDir got %q", got)
	}
	if got := defaultSecretsDir(); got != filepath.Join(userDir, "secrets") {
		t.Fatalf("defaultSecretsDir got %q", got)
	}
}

func TestGitupsUserDirEnvOverridesUserHome(t *testing.T) {
	clearGitupsEnv(t)
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "custom-gitups")
	t.Setenv("HOME", home)
	t.Setenv(gitupsUserDirEnv, override)

	if got := defaultGitupsUserDir(); got != override {
		t.Fatalf("defaultGitupsUserDir got %q, want %q", got, override)
	}
	if got := defaultStateDir(); got != filepath.Join(override, "state") {
		t.Fatalf("defaultStateDir got %q", got)
	}
	if got := defaultSecretsDir(); got != filepath.Join(override, "secrets") {
		t.Fatalf("defaultSecretsDir got %q", got)
	}
}

func TestGitupsStateDirEnvOverridesDefault(t *testing.T) {
	clearGitupsEnv(t)
	override := filepath.Join(t.TempDir(), "custom-state")
	t.Setenv(gitupsStateDirEnv, override)
	if got := defaultStateDir(); got != override {
		t.Fatalf("defaultStateDir got %q, want %q", got, override)
	}
}

func TestGitupsSecretsDirEnvOverridesDefault(t *testing.T) {
	clearGitupsEnv(t)
	override := filepath.Join(t.TempDir(), "custom-secrets")
	t.Setenv(gitupsSecretsDirEnv, override)
	if got := defaultSecretsDir(); got != override {
		t.Fatalf("defaultSecretsDir got %q, want %q", got, override)
	}
}

func TestProviderApplyRejectsUnsupportedProvider(t *testing.T) {
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
  keys:
    pull-secret:
      file: ./pull-secret
    ssh-key:
      file: ./ssh-key
    example-vcenter:
      file: ./vcenter
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
	code := Run(context.Background(), []string{"provider", "apply", "-f", path, "--dry-run"}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected unsupported provider failure")
	}
	if !strings.Contains(stderr.String(), `apply does not yet support provider kind "vsphere"`) {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "supported: baremetal, libvirt") {
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
    pull-secret:
      file: ./pull-secret
    ssh-key:
      file: ./ssh-key.pub
    default-key:
      file: ./default-key
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
    pull-secret:
      file: ./pull-secret
    ssh-key:
      file: ./ssh-key.pub
    default-key:
      file: ./default-key
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

func TestProviderApplyDryRunRendersAndPrintsAnsibleCommandsForAllPhases(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"provider", "apply",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
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
		"- provider [root]",
		"- cluster [root]",
		"dry-run ansible command [provider apply]: ansible-playbook",
		"playbooks/apply-infra.yml",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
	if got := strings.Count(output, "--ask-become-pass"); got != 1 {
		t.Fatalf("provider apply should ask become once, got %d prompts\n%s", got, output)
	}
	for _, unexpected := range []string{"clusters-install.yml", "gitops-publish.yml"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("provider apply leaked %q\n%s", unexpected, output)
		}
	}
}

func TestProviderApplyDryRunPassesStateSecretsAndHostStateDirs(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	secretsDir := filepath.Join(root, "secrets")
	hostStateDir := filepath.Join(root, "host-state")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"provider", "apply",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
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

func TestClustersApplyDryRunOnlyRunsClustersScope(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "playbooks/apply-clusters.yml") {
		t.Fatalf("stdout missing apply-clusters.yml: %s", output)
	}
	for _, leaked := range []string{"provider-prepare.yml", "cluster-prepare.yml", "clusters-install.yml", "gitops-publish.yml"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("clusters-scope apply leaked %s:\n%s", leaked, output)
		}
	}
}

func TestProviderDestroyRequiresYesOrDryRun(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"provider", "destroy",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
	}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected destroy without --yes/--dry-run to fail")
	}
	if !strings.Contains(stderr.String(), "destroy refused") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestProviderDestroyDryRunPrintsPhasesInReverse(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"provider", "destroy",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	clusterIdx := strings.Index(output, "- cluster [root]")
	providerIdx := strings.Index(output, "- provider [root]")
	if clusterIdx < 0 || providerIdx < 0 {
		t.Fatalf("missing destroy phase entries:\n%s", output)
	}
	if !(clusterIdx < providerIdx) {
		t.Fatalf("destroy phases not in reverse order (cluster=%d provider=%d)\n%s", clusterIdx, providerIdx, output)
	}
	for _, expected := range []string{
		"dry-run ansible command [provider destroy]: ansible-playbook",
		"playbooks/destroy-infra.yml",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
	if got := strings.Count(output, "--ask-become-pass"); got != 1 {
		t.Fatalf("provider destroy should ask become once, got %d prompts\n%s", got, output)
	}
}

func TestClustersDestroyDryRunUsesClustersWorkflow(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "destroy",
		"-f", "../../examples/libvirt-redfish-lab-fleet",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"- clusters [root]",
		"dry-run ansible command [clusters destroy]: ansible-playbook",
		"playbooks/clusters-destroy.yml",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

func TestClustersApplyDryRunUsesAnsibleBecomePrompt(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "ANSIBLE_BECOME_PASSWORD_FILE") {
		t.Fatalf("stdout must not reference ANSIBLE_BECOME_PASSWORD_FILE; ansible should prompt directly\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--ask-become-pass") {
		t.Fatalf("stdout must include --ask-become-pass for remote become prompts\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "dry-run ansible command [clusters apply]: ansible-playbook") {
		t.Fatalf("stdout must use the clusters scope playbook\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "playbooks/apply-clusters.yml") {
		t.Fatalf("stdout missing apply-clusters wrapper playbook\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "playbooks/clusters-install.yml") {
		t.Fatalf("clusters apply should run through apply-clusters.yml, not directly through clusters-install.yml\n%s", stdout.String())
	}
}

func TestClustersApplyAsRootSkipsBecomePrompt(t *testing.T) {
	orig := askBecomePassDefault
	askBecomePassDefault = func() bool { return false }
	t.Cleanup(func() { askBecomePassDefault = orig })
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
		"-f", "../../test/e2e/libvirt-1-host-1-sno-hub",
		"--state-dir", stateDir,
		"--dry-run",
	}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code got %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "--ask-become-pass") {
		t.Fatalf("root-default apply must omit --ask-become-pass\n%s", stdout.String())
	}
}

func TestClustersApplyDryRunPrintsEscalationSummary(t *testing.T) {
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
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
		"- clusters [root]",
		"[root] phases require sudo escalation",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout missing %q\n%s", expected, output)
		}
	}
}

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
		if info, err := origStat(path); err == nil {
			return info, nil
		}
		return fakeFileInfo{name: filepath.Base(path)}, nil
	}
	t.Cleanup(func() {
		defaultPreflightDeps.tryListen = origListen
		defaultPreflightDeps.statPath = origStat
	})
}

func TestClustersApplyConfirmationDecline(t *testing.T) {
	stubPreflightAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
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

func TestClustersApplyYesSkipsConfirmationAndStopsBeforeAnsible(t *testing.T) {
	stubPreflightAlwaysOK(t)
	stateDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"clusters", "apply",
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

// keep io.Discard import live across tests for parity with prior file
var _ = io.Discard
