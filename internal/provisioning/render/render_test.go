package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/infra"
	"go.yaml.in/yaml/v3"
)

func TestRenderResolvesFileBasedSecretsToSourcePath(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Secrets: map[string]v1alpha1.EnvironmentSecretSpec{
					"my-key": {File: "/tmp/foo"},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
			Spec: v1alpha1.OCPClusterSpec{
				InfrastructureRef: v1alpha1.LocalObjectReference{Name: "hub"},
				Install: v1alpha1.OCPInstallSpec{
					SSHKeyRef: v1alpha1.SecretRef{Name: "my-key"},
				},
			},
		}},
		ClusterInfrastructures: []v1alpha1.ClusterInfrastructure{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
		}},
	}
	vars := Vars(state, "/anywhere")
	if got, want := vars.BootwrightClusters[0].OCP.Install.SSHKeyRef, "/tmp/foo"; got != want {
		t.Fatalf("file-based SSHKeyRef got %q, want %q", got, want)
	}
	if got := vars.BootwrightClusters[0].OCP.Install.SSHKeyRef; strings.HasPrefix(got, "/anywhere") {
		t.Fatalf("file-based SSHKeyRef must not use secretsDir prefix, got %q", got)
	}
}

func TestRenderAllProducesGeneratedAnsibleArtifacts(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../examples/libvirt-redfish-lab-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	for _, path := range []string{
		result.EffectiveStatePath,
		result.LockPath,
		result.InventoryPath,
		result.VarsPath,
		result.ArtifactsDir,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected rendered path %s: %v", path, err)
		}
	}
	if got, want := len(result.InstallerAssets), 3; got != want {
		t.Fatalf("got %d installer assets, want %d", got, want)
	}
	for _, asset := range result.InstallerAssets {
		for _, path := range []string{asset.InstallConfigPath, asset.AgentConfigPath} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected rendered installer path %s: %v", path, err)
			}
		}
	}
	for _, path := range []string{result.EffectiveStatePath, result.LockPath, result.InventoryPath, result.VarsPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat rendered file %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("rendered file %s mode got %03o, want 600", path, got)
		}
	}
	for _, path := range []string{stateDir, result.ArtifactsDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat rendered dir %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("rendered dir %s mode got %03o, want 700", path, got)
		}
	}
	inventory := readFile(t, result.InventoryPath)
	for _, expected := range []string{
		"hub-hub-sno-host:",
		"managed-01-managed-01-host:",
		"managed-02-managed-02-host:",
		"bootwright_cluster_name: hub",
	} {
		if !strings.Contains(inventory, expected) {
			t.Fatalf("inventory missing %q\n%s", expected, inventory)
		}
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"api.hub.example.com",
		"api-int.managed-01.example.com",
		"*.apps.managed-02.example.com",
		"infrastructureHosts:",
		"virtualization:",
		"bmc:",
		"port: 8000",
		"nodes:",
		"memoryMiB: 22528",
		"macAddress: 52:54:00:",
		"name: sushy-tools",
		"version: 2.1.0",
		"nameResolution:",
		"loadBalancer:",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
	effectiveState := readFile(t, result.EffectiveStatePath)
	for _, unexpected := range []string{
		"libvirtNetwork:",
		"machineNetwork:",
		"apiVIP:",
	} {
		if strings.Contains(effectiveState, unexpected) {
			t.Fatalf("effective state contains legacy field %q\n%s", unexpected, effectiveState)
		}
	}
	lock := readFile(t, result.LockPath)
	for _, expected := range []string{
		"name: ansible-core",
		"version: 2.20.5",
		"name: go.yaml.in/yaml/v3",
		"version: v3.0.4",
	} {
		if !strings.Contains(lock, expected) {
			t.Fatalf("lock missing %q\n%s", expected, lock)
		}
	}
}

func TestRenderEmitsBMCAuthCredentialRefWhenSet(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"auth:",
		"credentialRef: libvirt-1-host-bmc-credentials",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
}

func TestRenderInstallerAssets(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../examples/libvirt-redfish-lab-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	asset := result.InstallerAssets[0]
	if got, want := asset.InstallConfigPath, filepath.Join(stateDir, "git-repos", "clusters-bootstrap", "hub", "openshift", "install-config.yaml"); got != want {
		t.Fatalf("install-config path got %q, want %q", got, want)
	}
	installConfig := readFile(t, asset.InstallConfigPath)
	for _, expected := range []string{
		"apiVersion: v1",
		"baseDomain: example.com",
		"name: hub",
		"none:",
		"machineNetwork:",
		"cidr: 192.168.122.0/24",
		"bootwright-secret-ref:openshift-pull-secret",
		"bootwright-ssh-key-ref:cluster-admin-key",
	} {
		if !strings.Contains(installConfig, expected) {
			t.Fatalf("install-config missing %q\n%s", expected, installConfig)
		}
	}
	for _, unexpected := range []string{
		"pullSecretRef:",
		"sshKeyRef:",
		"baremetal:",
		"apiVIPs:",
		"ingressVIPs:",
	} {
		if strings.Contains(installConfig, unexpected) {
			t.Fatalf("install-config contains unexpected field %q\n%s", unexpected, installConfig)
		}
	}
	multiNodeAsset := result.InstallerAssets[1]
	multiNodeConfig := readFile(t, multiNodeAsset.InstallConfigPath)
	for _, expected := range []string{
		"baremetal:",
		"apiVIPs:",
		"ingressVIPs:",
	} {
		if !strings.Contains(multiNodeConfig, expected) {
			t.Fatalf("multi-node install-config missing %q\n%s", expected, multiNodeConfig)
		}
	}
	agentConfig := readFile(t, asset.AgentConfigPath)
	for _, expected := range []string{
		"apiVersion: v1beta1",
		"kind: AgentConfig",
		"name: hub",
		"rendezvousIP: 192.168.122.20",
		"hostname: master-0",
		"role: master",
		"macAddress: 52:54:00:",
	} {
		if !strings.Contains(agentConfig, expected) {
			t.Fatalf("agent-config missing %q\n%s", expected, agentConfig)
		}
	}
}

func TestRenderInstallerOverrides(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"environment.yaml", "provider.yaml", "hub.yaml"} {
		data, err := os.ReadFile(filepath.Join("../../../examples/libvirt-redfish-lab-fleet", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	hubPath := filepath.Join(dir, "hub.yaml")
	hub, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatalf("read hub: %v", err)
	}
	body := strings.Replace(string(hub), "  install:\n    method: agent\n", `  install:
    method: agent
    imageDigestSources:
      - source: quay.io/openshift-release-dev/ocp-release
        mirrors:
          - registry.example.com:5000/openshift/release-images
    installConfigOverrides:
      fips: true
      networking:
        networkType: OVNKubernetes
    agentConfigOverrides:
      additionalNTPSources:
        - 192.168.122.1
`, 1)
	if err := os.WriteFile(hubPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	state, err := infra.LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	asset := result.InstallerAssets[0]
	installConfig := readFile(t, asset.InstallConfigPath)
	for _, expected := range []string{
		"fips: true",
		"networkType: OVNKubernetes",
		"machineNetwork:",
		"imageDigestSources:",
		"source: quay.io/openshift-release-dev/ocp-release",
		"registry.example.com:5000/openshift/release-images",
	} {
		if !strings.Contains(installConfig, expected) {
			t.Fatalf("install-config missing %q\n%s", expected, installConfig)
		}
	}
	agentConfig := readFile(t, asset.AgentConfigPath)
	for _, expected := range []string{
		"additionalNTPSources:",
		"192.168.122.1",
		"rootDeviceHints:",
		"deviceName: /dev/vda",
		"role: master",
		"interfaces:",
		"networkConfig:",
		"prefix-length: 24",
		"next-hop-address: 192.168.122.1",
	} {
		if !strings.Contains(agentConfig, expected) {
			t.Fatalf("agent-config missing %q\n%s", expected, agentConfig)
		}
	}
}

func TestRenderedArtifactsStayUnderStateDir(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../examples/libvirt-redfish-lab-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	paths := []string{result.InventoryPath, result.VarsPath}
	for _, asset := range result.InstallerAssets {
		paths = append(paths, asset.InstallConfigPath, asset.AgentConfigPath)
	}
	for _, path := range paths {
		rel, err := filepath.Rel(stateDir, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		if strings.HasPrefix(rel, "..") {
			t.Fatalf("%s is outside stateDir %s", path, stateDir)
		}
	}
}

func TestRenderMirrorRegistryRunVars(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"bootwright_mirror_registries:",
		"name: local-libvirt-1-host-provider",
		"providerRef: local-libvirt-1-host-provider",
		"providerHostRef: local-libvirt-host",
		"port: 5000",
		"credentialsSecretName: mirror-registry-credentials",
		"trustBundleCertSecretName: mirror-registry-ca",
		"trustBundleKeySecretName: mirror-registry-ca.key",
		"local: registry.mirror.local:5000/library/registry:3.1.1",
		"public: docker.io/library/registry:3.1.1@sha256:85347ed2ecde64161c7a4788a4d7d3dcc9d6f86f7be95834022e3c6a423a945a",
		"mirrorSet:",
		"kind: releasePayload",
		"public: quay.io/openshift-release-dev/ocp-release:4.21.12-x86_64",
		"local: registry.mirror.local:5000/openshift/release-images:4.21.12-x86_64",
		"kind: componentImage",
		"public: docker.io/library/haproxy:3.3.8@sha256:f14a1788b56894e7ec7b5cb0ca09dbb959b674cf3c980f92139ec008167d4a91",
		"local: registry.mirror.local:5000/library/haproxy:3.3.8",
		"kind: registryServer",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
}

func TestRenderManagedProxyRunVarsAndLibvirtIsolation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "case.yaml"), managedProxyRenderYAML("proxy-render", "proxy-render-provider", "192.168.166.0/24", "192.168.166.1", "192.168.166.10", "192.168.166.11", "192.168.166.20"))
	state, err := infra.LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"bootwright_forward_proxies:",
		"name: proxy-render-provider",
		"providerRef: proxy-render-provider",
		"providerHostRef: host-01",
		"url: http://10.0.0.1:3128",
		"port: 3128",
		"runtime: podman",
		"credentialsSecretName: proxy-credentials",
		"public: docker.io/openeuler/squid:7.5-oe2403sp3@sha256:8e16e4439a7c0d4e0e71092a1611bb89cea9929c30642c18ba991ba7a7524d87",
		"egressRestrictedToProxy: true",
		"proxyURL: http://192.168.166.1:3128",
		"proxyPort: 3128",
		"http: http://10.0.0.1:3128",
		"https: http://10.0.0.1:3128",
		"vmHttp: http://192.168.166.1:3128",
		"vmHttps: http://192.168.166.1:3128",
		"name: squid",
		"version: 7.5-oe2403sp3",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("managed proxy vars missing %q\n%s", expected, varsFile)
		}
	}
	asset := installerAssetFor(result.InstallerAssets, "proxy-render")
	assertInstallConfigProxy(t, asset.InstallConfigPath, "http://192.168.166.1:3128", "http://192.168.166.1:3128", []string{"localhost", ".example.com"})
}

func TestRenderOneHostTreatsLocalhostAsProviderHost(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	inventory := readFile(t, result.InventoryPath)
	for _, expected := range []string{
		"ansible_host: localhost",
		"bootwright_cluster_name: local-libvirt-1-host-hub",
	} {
		if !strings.Contains(inventory, expected) {
			t.Fatalf("inventory missing %q\n%s", expected, inventory)
		}
	}
	if strings.Contains(inventory, "ansible_become:") {
		t.Fatalf("provider root escalation belongs on mutating playbooks, not inventory\n%s", inventory)
	}
	playbook := readFile(t, "../../../ansible/playbooks/layers/openshift/destroy-agent.yml")
	if !strings.Contains(playbook, "become: true") {
		t.Fatalf("clusters destroy target must keep provider-host root escalation\n%s", playbook)
	}
}

func TestRenderVarsExposeOCPInstallMetadata(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home: %v", err)
	}
	for _, expected := range []string{
		"bootwright_ocp_install:",
		"mode: disconnected",
		"disconnected: true",
		"method: agent",
		"pullSecretRef: " + filepath.Join(home, ".bootwright/secrets/openshift-pull-secret"),
		"sshKeyRef: " + filepath.Join(home, ".ssh/bootwright-ssh-key.pub"),
		"releaseImageOverride: registry.mirror.local:5000/openshift/release-images:4.21.12-x86_64",
		"additionalTrustBundleRef: mirror-registry-ca",
		"version: 4.21.12",
		"channel: stable-4.21",
		"relativeDir: clusters-bootstrap.git/local-libvirt-1-host-hub/openshift",
		"relativeInstallConfigPath: clusters-bootstrap.git/local-libvirt-1-host-hub/openshift/install-config.yaml",
		"relativeAgentConfigPath: clusters-bootstrap.git/local-libvirt-1-host-hub/openshift/agent-config.yaml",
		"name: openshift-install",
		"machineRef: master-0",
		"localRegistry:",
		"url: registry.mirror.local:5000",
		"host: registry.mirror.local",
		"credentialsRef: mirror-registry-credentials",
		"trustBundleRef: mirror-registry-ca",
		"dnsHosts:",
		"ip: 192.168.130.1",
		"- registry.mirror.local",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
	if strings.Contains(varsFile, stateDir) {
		t.Fatalf("vars.yaml leaked stateDir %q (paths must be relative)\n%s", stateDir, varsFile)
	}
	lock := readFile(t, result.LockPath)
	if !strings.Contains(lock, "name: openshift-install") || !strings.Contains(lock, "version: 4.21.12") {
		t.Fatalf("lock missing openshift-install pin\n%s", lock)
	}
	asset := installerAssetFor(result.InstallerAssets, "local-libvirt-1-host-hub")
	installConfig := readFile(t, asset.InstallConfigPath)
	for _, expected := range []string{
		"imageDigestSources:",
		"registry.mirror.local:5000/openshift/release-images",
		"sourcePolicy: NeverContactSource",
		"additionalTrustBundle: <bootwright-trust-bundle-ref:mirror-registry-ca>",
		"additionalTrustBundlePolicy: Always",
	} {
		if !strings.Contains(installConfig, expected) {
			t.Fatalf("install-config missing %q\n%s", expected, installConfig)
		}
	}
	agentConfig := readFile(t, asset.AgentConfigPath)
	for _, expected := range []string{
		"minimalISO: true",
		"bootArtifactsBaseURL: http://192.168.130.1:8002/",
	} {
		if !strings.Contains(agentConfig, expected) {
			t.Fatalf("agent-config missing %q\n%s", expected, agentConfig)
		}
	}
}

func TestOCPInstallRoleDoesNotBlockPublicRegistries(t *testing.T) {
	tasksDir := "../../../ansible/roles/openshift/install_agent/tasks"
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		t.Fatalf("read tasks dir: %v", err)
	}
	blockers := []string{"127.0.0.1:1", "HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "sinkhole"}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		body := readFile(t, filepath.Join(tasksDir, e.Name()))
		for _, unexpected := range blockers {
			if strings.Contains(body, unexpected) {
				t.Fatalf("install_agent/%s contains public-registry blocking behavior %q\n%s", e.Name(), unexpected, body)
			}
		}
	}
}

func TestOCPInstallRoleDoesNotShadowEnvironmentInstallVars(t *testing.T) {
	preflight := readFile(t, "../../../ansible/roles/openshift/install_agent/tasks/preflight.yml")
	secrets := readFile(t, "../../../ansible/roles/openshift/install_agent/tasks/secrets.yml")
	combined := preflight + "\n" + secrets
	for _, expected := range []string{
		"bootwright_ocp_cluster_install: \"{{ bootwright_current_cluster.ocp.install }}\"",
		"bootwright_ocp_cluster_install.method",
		"bootwright_ocp_cluster_install.generatedSecrets",
		"bootwright_ocp_cluster_install.pullSecretRef",
		"bootwright_ocp_cluster_install.sshKeyRef",
		"bootwright_ocp_cluster_install.additionalTrustBundleRef",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("install_agent must use cluster install fact %q\n%s", expected, combined)
		}
	}
	for _, unexpected := range []string{
		"bootwright_ocp_install: \"{{ bootwright_current_cluster.ocp.install }}\"",
		"bootwright_ocp_install.method",
		"bootwright_ocp_install.generatedSecrets",
		"bootwright_ocp_install.pullSecretRef",
		"bootwright_ocp_install.sshKeyRef",
		"bootwright_ocp_install.additionalTrustBundleRef",
	} {
		if strings.Contains(combined, unexpected) {
			t.Fatalf("install_agent shadows environment install vars with %q\n%s", unexpected, combined)
		}
	}
}

func TestLibvirtSubstrateOpensBootArtifactsHTTPPort(t *testing.T) {
	tasks := readFile(t, "../../../ansible/roles/cluster_infra/substrate_libvirt/tasks/main.yml")
	networkTemplate := readFile(t, "../../../ansible/roles/cluster_infra/substrate_libvirt/templates/network.xml.j2")
	combined := tasks + "\n" + networkTemplate
	for _, expected := range []string{
		"<dhcp>",
		"<host mac='{{ node.macAddress }}'",
		"name='{{ bootwright_current_cluster.name }}-{{ node.name }}'",
		"ip='{{ node.ipAddress }}'",
		"ansible.posix.firewalld",
		"zone: libvirt",
		"interface: \"{{ bootwright_current_cluster.provider.virtualization.libvirt.bridge }}\"",
		"port: \"{{ bootwright_current_cluster.provider.bootArtifactsHttp.port | int }}/tcp\"",
		"egressRestrictedToProxy",
		"{% if not (bootwright_current_cluster.provider.virtualization.libvirt.egressRestrictedToProxy | default(false) | bool) %}",
		"<forward mode='nat'>",
		"port: \"{{ bootwright_current_cluster.provider.virtualization.libvirt.proxyPort | int }}/tcp\"",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("substrate_libvirt is missing %q\n%s", expected, combined)
		}
	}
	for _, leak := range []string{
		"provider.bmc.port",
		"provider.bmc.enabled",
		"bootwright_load_balancers",
		"Plumb cluster load balancer VIPs",
		"--get-zone-of-interface",
		"--add-port=",
		"--change-interface=",
	} {
		if strings.Contains(tasks, leak) {
			t.Fatalf("substrate_libvirt still references %q — extraction is incomplete", leak)
		}
	}
}

func TestClusterNetworkVipsOwnsVIPPlumbing(t *testing.T) {
	apply := readFile(t, "../../../ansible/roles/cluster_infra/network_vips/tasks/main.yml")
	for _, expected := range []string{
		"Attach load balancer VIPs",
		"bootwright_in_cidr",
		"bootwright_load_balancers",
	} {
		if !strings.Contains(apply, expected) {
			t.Fatalf("network_vips/tasks/main.yml missing %q\n%s", expected, apply)
		}
	}
	destroy := readFile(t, "../../../ansible/roles/cluster_infra/network_vips/tasks/destroy.yml")
	if !strings.Contains(destroy, "Detach load balancer VIPs") {
		t.Fatalf("network_vips/tasks/destroy.yml missing unplumb task\n%s", destroy)
	}
	if _, err := os.Stat("../../../ansible/roles/cluster_infra/network_vips/test_plugins/cidr.py"); err != nil {
		t.Fatalf("network_vips/test_plugins/cidr.py must own the bootwright_in_cidr plugin: %v", err)
	}
}

func TestRenderVarsExposeGeneratedSecrets(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"hub.yaml"} {
		data, err := os.ReadFile(filepath.Join("../../../examples/libvirt-redfish-lab-fleet", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	providerData, err := os.ReadFile(filepath.Join("../../../examples/libvirt-redfish-lab-fleet", "provider.yaml"))
	if err != nil {
		t.Fatalf("read provider.yaml: %v", err)
	}
	hubProvider := strings.SplitN(string(providerData), "\n---\n", 2)[0]
	hubProvider = strings.Replace(
		hubProvider,
		"      capabilities:\n        - libvirt\n        - hosts-file",
		"      capabilities:\n        - libvirt\n        - hosts-file\n        - mirror-registry",
		1,
	) + "\n  registry:\n    mirrorRegistry:\n      hostRef:\n        name: hub-sno-host\n      port: 5000\n"
	if err := os.WriteFile(filepath.Join(dir, "provider.yaml"), []byte(hubProvider), 0o644); err != nil {
		t.Fatalf("write provider.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environment.yaml"), []byte(`apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: example
spec:
  baseDomain: example.com
  ocpInstallType: disconnected
  registries:
    mirror:
      url: registry.lab.test:5000
      credentialsRef:
        name: registry-lab-credentials
      trustBundleRef:
        name: registry-lab-ca
    imageDigestSources:
      - source: quay.io/openshift-release-dev/ocp-release
        mirrors:
          - registry.lab.test:5000/openshift/release-images
      - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
        mirrors:
          - registry.lab.test:5000/openshift/release-images
  secrets:
    openshift-pull-secret:
      file: ~/.bootwright/secrets/openshift-pull-secret
    cluster-admin-key:
      file: ~/.ssh/bootwright-ssh-key.pub
    lab-provider-key:
      file: ~/.ssh/bootwright-ssh-key
    lab-bmc-credentials:
      generated:
        credentials:
          username: admin
    registry-lab-ca:
      generated:
        selfSignedCertificate:
          commonName: registry.lab.test
    registry-lab-credentials:
      generated:
        credentials:
          username: admin
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.12
`), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}
	state, err := infra.LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"generatedSecrets:",
		"name: registry-lab-ca",
		"type: self-signed-certificate",
		"commonName: registry.lab.test",
		"validityDays: 3650",
		"subjectAltName: DNS:registry.lab.test",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
}

func TestRenderManagedNetworkDetails(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{
		"../../../test/e2e/old/libvirt-1-host-1-sno-hub/cluster-infrastructure-hub.yaml",
		"../../../test/e2e/old/libvirt-1-host-1-sno-hub/ocp-cluster-hub.yaml",
		"../../../test/e2e/old/libvirt-1-host-1-sno-hub/provider.yaml",
		"../../../test/e2e/old/libvirt-1-host-1-sno-hub/environment.yaml",
	})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"public: docker.io/library/haproxy:3.3.8@sha256:f14a1788b56894e7ec7b5cb0ca09dbb959b674cf3c980f92139ec008167d4a91",
		"runtime: podman",
		"bindAddress: 192.168.130.10",
		"targetPort: 6443",
		"backends:",
		"address: 192.168.130.20",
		"console-openshift-console.apps.libvirt-1-host-hub.bootwright.test",
		"name: haproxy",
		"version: 3.3.8",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
	if strings.Contains(varsFile, "local: registry.mirror.local:5000/library/haproxy:3.3.8") {
		t.Fatalf("connected fixture rendered disconnected HAProxy image\n%s", varsFile)
	}
}

func TestRenderBareMetalProjectsPerMachineBMC(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/baremetal-redfish-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"kind: baremetal",
		"substrateRole: baremetal",
		"bmcRole: redfish",
		"bareMetal:",
		"address: redfish-virtualmedia+https://bmc-hub-0.example.test",
		"credentialRef: baremetal-redfish-bmc",
		"disableCertificateVerification: true",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
	for _, leak := range []string{
		"substrateRole: libvirt",
		"bmcRole: emulated",
		"virtualization:",
	} {
		if strings.Contains(varsFile, leak) {
			t.Fatalf("vars unexpectedly contains qemu/libvirt field %q for bare-metal fixture\n%s", leak, varsFile)
		}
	}
}

func TestRenderMultiProviderClosure(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../examples/baremetal-edge-lb-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"kind: baremetal",
		"substrateRole: baremetal",
		"providerRef: edge-haproxy-provider",
		"providerHostRef: edge-host",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("multi-provider vars missing %q\n%s", expected, varsFile)
		}
	}
	for _, leak := range []string{
		"substrateRole: libvirt",
	} {
		if strings.Contains(varsFile, leak) {
			t.Fatalf("multi-provider vars unexpectedly contains %q\n%s", leak, varsFile)
		}
	}
}

func TestResolveInstallerInlinesSecretsIntoWorkCopies(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	secretsDir := t.TempDir()
	pullSecretContent := `{"auths":{"quay.io":{"auth":"cmVkaGF0OnNlY3JldA=="}}}`
	sshKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5IFAKE testkey"
	trustBundle := "-----BEGIN CERTIFICATE-----\nMIIDfaketrust\n-----END CERTIFICATE-----\n"
	writes := map[string]string{
		"openshift-pull-secret":       pullSecretContent + "\n",
		"cluster-admin-key":           sshKey + "\n",
		"mirror-registry-ca":          trustBundle,
		"mirror-registry-credentials": "mirroruser:mirrorpass\n",
	}
	for name, content := range writes {
		if err := os.WriteFile(filepath.Join(secretsDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	env := &state.Environments[0]
	for name, key := range env.Spec.Secrets {
		key.File = ""
		env.Spec.Secrets[name] = key
	}

	stateDir := t.TempDir()
	result, err := All(stateDir, secretsDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	placeholder := readFile(t, result.InstallerAssets[0].InstallConfigPath)
	if !strings.Contains(placeholder, "bootwright-secret-ref:openshift-pull-secret") {
		t.Fatalf("placeholder install-config missing pull secret placeholder\n%s", placeholder)
	}
	if strings.Contains(placeholder, sshKey) || strings.Contains(placeholder, pullSecretContent) {
		t.Fatalf("placeholder install-config leaked secret material\n%s", placeholder)
	}

	resolved, err := ResolveInstaller(stateDir, secretsDir, state)
	if err != nil {
		t.Fatalf("ResolveInstaller returned error: %v", err)
	}
	if len(resolved.InstallerAssets) != 1 {
		t.Fatalf("expected one resolved installer asset, got %d", len(resolved.InstallerAssets))
	}
	asset := resolved.InstallerAssets[0]
	expectedWork := filepath.Join(stateDir, "runtime", state.OCPClusters[0].Metadata.Name, "installer", "install-config.yaml")
	if got := asset.EffectiveInstallConfigPath; got != expectedWork {
		t.Fatalf("effective install-config path got %q, want %q", got, expectedWork)
	}
	if rel, err := filepath.Rel(filepath.Join(stateDir, "git-repos", "clusters-bootstrap"), asset.EffectiveInstallConfigPath); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("effective install-config must live outside the bootstrap repo, got %q", asset.EffectiveInstallConfigPath)
	}
	workDirInfo, err := os.Stat(asset.WorkDir)
	if err != nil {
		t.Fatalf("stat work dir: %v", err)
	}
	if got := workDirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("work dir mode got %03o, want 700", got)
	}
	effectiveBytes, err := os.ReadFile(asset.EffectiveInstallConfigPath)
	if err != nil {
		t.Fatalf("read effective install-config: %v", err)
	}
	effective := string(effectiveBytes)
	for _, expected := range []string{
		`"auths":{`,
		`"quay.io":{"auth":"cmVkaGF0OnNlY3JldA=="}`,
		`"registry.mirror.local:5000":{"auth":"bWlycm9ydXNlcjptaXJyb3JwYXNz"}`,
		sshKey,
		"MIIDfaketrust",
		"additionalTrustBundlePolicy: Always",
	} {
		if !strings.Contains(effective, expected) {
			t.Fatalf("effective install-config missing %q\n%s", expected, effective)
		}
	}
	for _, leaked := range []string{
		"bootwright-secret-ref:",
		"<bootwright-ssh-key-ref:",
		"<bootwright-trust-bundle-ref:",
	} {
		if strings.Contains(effective, leaked) {
			t.Fatalf("effective install-config still contains placeholder %q\n%s", leaked, effective)
		}
	}
	info, err := os.Stat(asset.EffectiveInstallConfigPath)
	if err != nil {
		t.Fatalf("stat effective install-config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("effective install-config mode got %03o, want 600", got)
	}
}

func TestLoadInstallerSecretsBakesProxyCredentialsIntoURLs(t *testing.T) {
	secretsDir := t.TempDir()
	pull := `{"auths":{"quay.io":{"auth":"YWJjOmRlZg=="}}}`
	if err := os.WriteFile(filepath.Join(secretsDir, "openshift-pull-secret"), []byte(pull), 0o600); err != nil {
		t.Fatalf("write pull: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "cluster-admin-key"), []byte("ssh-ed25519 AAA fake"), 0o600); err != nil {
		t.Fatalf("write ssh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "proxy-credentials"), []byte("proxy user:pass/word"), 0o600); err != nil {
		t.Fatalf("write proxy: %v", err)
	}
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				BaseDomain:     "example.com",
				OCPInstallType: v1alpha1.OCPInstallKindDisconnected,
				Proxy: &v1alpha1.EnvironmentProxySpec{
					HTTP:    "http://proxy.example.com:3128",
					HTTPS:   "http://proxy.example.com:3128",
					NoProxy: []string{".cluster.local"},
					Auth: &v1alpha1.EnvironmentProxyAuthSpec{
						ProxyAuthRef: v1alpha1.SecretRef{Name: "proxy-credentials"},
					},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
			Spec: v1alpha1.OCPClusterSpec{
				Install: v1alpha1.OCPInstallSpec{
					PullSecretRef: v1alpha1.SecretRef{Name: "openshift-pull-secret"},
					SSHKeyRef:     v1alpha1.SecretRef{Name: "cluster-admin-key"},
				},
			},
		}},
	}
	secrets, err := LoadInstallerSecrets(state, state.OCPClusters[0], secretsDir)
	if err != nil {
		t.Fatalf("LoadInstallerSecrets returned error: %v", err)
	}
	want := "http://proxy%20user:pass%2Fword@proxy.example.com:3128"
	if secrets.ProxyHTTP != want {
		t.Fatalf("ProxyHTTP got %q, want %q", secrets.ProxyHTTP, want)
	}
	if secrets.ProxyHTTPS != want {
		t.Fatalf("ProxyHTTPS got %q, want %q", secrets.ProxyHTTPS, want)
	}
}

func TestE2EProxyInputRendersIntoOpenShiftInstallerFiles(t *testing.T) {
	fixtureDir := t.TempDir()
	copyYAMLFixture(t, "../../../test/e2e/sno-libvirt", fixtureDir)

	envPath := filepath.Join(fixtureDir, "environment.yaml")
	envBody := readFile(t, envPath)
	envBody = replaceOnce(t, envBody, `  proxy:
    noProxy:
      - 192.168.132.0/24
    auth:
      proxyAuthRef:
        name: proxy-credentials
`, `  proxy:
    http: http://proxy.bootwright.test:3128
    https: https://secure-proxy.bootwright.test:8443
    noProxy:
      - 192.168.132.0/24
    auth:
      proxyAuthRef:
        name: proxy-credentials
`)
	envBody = replaceOnce(t, envBody, "      file: ~/.ssh/bootwright-ssh-key.pub\n", "      file: secrets/cluster-admin-key\n")
	envBody = replaceOnce(t, envBody, "      file: ~/.bootwright/secrets/openshift-pull-secret\n", "      file: secrets/openshift-pull-secret\n")
	envBody = replaceOnce(t, envBody, "    proxy-credentials:\n      generated:\n        credentials:\n          username: proxy\n", "    proxy-credentials:\n      file: secrets/proxy-credentials\n")
	if err := os.WriteFile(envPath, []byte(envBody), 0o644); err != nil {
		t.Fatalf("write environment with proxy: %v", err)
	}
	providerPath := filepath.Join(fixtureDir, "provider.yaml")
	providerBody := readFile(t, providerPath)
	providerBody = replaceOnce(t, providerBody, "        - proxy\n", "")
	providerBody = replaceOnce(t, providerBody, `  proxy:
    squid:
      hostRef:
        name: lab-host
`, "")
	if err := os.WriteFile(providerPath, []byte(providerBody), 0o644); err != nil {
		t.Fatalf("write provider without managed proxy: %v", err)
	}

	secretsDir := filepath.Join(fixtureDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("create secrets dir: %v", err)
	}
	for _, item := range []struct {
		name    string
		content string
	}{
		{name: "openshift-pull-secret", content: `{"auths":{"quay.io":{"auth":"cmVkaGF0OnNlY3JldA=="}}}`},
		{name: "cluster-admin-key", content: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFake bootwright-test"},
		{name: "proxy-credentials", content: "proxy user:pass/word"},
	} {
		if err := os.WriteFile(filepath.Join(secretsDir, item.name), []byte(item.content), 0o600); err != nil {
			t.Fatalf("write %s: %v", item.name, err)
		}
	}

	state, err := infra.LoadNormalizeValidate([]string{fixtureDir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, secretsDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	if strings.Contains(varsFile, "bootwright_forward_proxies:") || strings.Contains(varsFile, "egressRestrictedToProxy: true") {
		t.Fatalf("external proxy must not render managed proxy or libvirt isolation\n%s", varsFile)
	}
	asset := installerAssetFor(result.InstallerAssets, "sno-libvirt")
	noProxy := []string{"192.168.132.0/24", "localhost", "127.0.0.1", ".bootwright.test", ".svc", ".cluster.local"}
	assertInstallConfigProxy(t, asset.InstallConfigPath, "http://proxy.bootwright.test:3128", "https://secure-proxy.bootwright.test:8443", noProxy)
	if safeConfig := readFile(t, asset.InstallConfigPath); strings.Contains(safeConfig, "proxy%20user") || strings.Contains(safeConfig, "pass%2Fword") {
		t.Fatalf("safe install-config must not contain proxy credentials\n%s", safeConfig)
	}

	resolved, err := ResolveInstaller(stateDir, secretsDir, state)
	if err != nil {
		t.Fatalf("ResolveInstaller returned error: %v", err)
	}
	resolvedAsset := installerAssetFor(resolved.InstallerAssets, "sno-libvirt")
	assertInstallConfigProxy(
		t,
		resolvedAsset.EffectiveInstallConfigPath,
		"http://proxy%20user:pass%2Fword@proxy.bootwright.test:3128",
		"https://proxy%20user:pass%2Fword@secure-proxy.bootwright.test:8443",
		noProxy,
	)
}

func TestOCPInstallCommandEnvironmentIncludesProxyEnv(t *testing.T) {
	effectiveConfig := readFile(t, "../../../ansible/roles/openshift/install_agent/tasks/effective-config.yml")
	for _, expected := range []string{
		"bootwright_proxy_env | default({})",
		"| combine(",
		"OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE",
	} {
		if !strings.Contains(effectiveConfig, expected) {
			t.Fatalf("effective-config.yml missing %q\n%s", expected, effectiveConfig)
		}
	}
}

func TestHostProxyFactsEscapesSlashInProxyCredentials(t *testing.T) {
	facts := readFile(t, "../../../ansible/roles/shared/host_proxy/tasks/facts.yml")
	for _, expected := range []string{
		"bootwright_proxy_credentials.username | urlencode | replace('/', '%2F')",
		"bootwright_proxy_credentials.password | urlencode | replace('/', '%2F')",
	} {
		if !strings.Contains(facts, expected) {
			t.Fatalf("host_proxy facts must escape slash in proxy userinfo; missing %q\n%s", expected, facts)
		}
	}
}

func TestProviderBMCEmulatedPipInstallUsesPrivatePipConfig(t *testing.T) {
	tasks := readFile(t, "../../../ansible/roles/providers/bmc_emulated/tasks/main.yml")
	for _, expected := range []string{
		"dest: \"{{ bootwright_host_state_dir }}/providers/{{ bootwright_current_provider.name }}/bmc/pip.conf\"",
		"PIP_CONFIG_FILE=\"$PIP_CONFIG\"",
		"-u HTTP_PROXY -u HTTPS_PROXY -u NO_PROXY",
		"-u http_proxy -u https_proxy -u no_proxy",
		"venv/bin/sushy-emulator",
		"executable: /bin/bash",
	} {
		if !strings.Contains(tasks, expected) {
			t.Fatalf("bmc_emulated pip install missing %q\n%s", expected, tasks)
		}
	}
	if strings.Contains(tasks, "--proxy") {
		t.Fatalf("bmc_emulated must not pass proxy credentials on pip argv\n%s", tasks)
	}
}

func TestResolveInstallerFailsWhenSecretFileMissing(t *testing.T) {
	secretsDir := t.TempDir()
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec:     v1alpha1.EnvironmentSpec{BaseDomain: "example.com"},
		}},
		OCPClusters: []v1alpha1.OCPCluster{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
			Spec: v1alpha1.OCPClusterSpec{
				InfrastructureRef: v1alpha1.LocalObjectReference{Name: "hub"},
				Install: v1alpha1.OCPInstallSpec{
					Method:        "agent",
					BaseDomain:    "example.com",
					PullSecretRef: v1alpha1.SecretRef{Name: "missing-pull-secret"},
					SSHKeyRef:     v1alpha1.SecretRef{Name: "missing-ssh-key"},
				},
				Topology: v1alpha1.OCPTopologySingleNode,
			},
		}},
		ClusterInfrastructures: []v1alpha1.ClusterInfrastructure{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
		}},
	}
	if _, err := ResolveInstaller(t.TempDir(), secretsDir, state); err == nil {
		t.Fatalf("expected ResolveInstaller to fail when pull secret file is missing")
	} else if !strings.Contains(err.Error(), "pull secret") {
		t.Fatalf("error should mention pull secret, got: %v", err)
	}
}

func TestProviderDispatchCoversAllKinds(t *testing.T) {
	clusterTasks := readFile(t, "../../../ansible/playbooks/layers/cluster_infra/apply.yml")
	if !strings.Contains(clusterTasks, "substrate_{{ bootwright_current_cluster.provider.substrateRole }}") {
		t.Fatalf("cluster infra apply playbook missing substrate dispatch fragment\n%s", clusterTasks)
	}
	providerTasks := readFile(t, "../../../ansible/playbooks/layers/providers/apply.yml")
	for _, expected := range []string{
		"proxy_squid",
		"bmc_{{ bootwright_current_provider.bmcRole }}",
		"bootwright_current_provider.bootArtifactsHttp.enabled",
	} {
		if !strings.Contains(providerTasks, expected) {
			t.Fatalf("providers apply playbook missing dispatch fragment %q\n%s", expected, providerTasks)
		}
	}
	if strings.Index(providerTasks, "proxy_squid") > strings.Index(providerTasks, "name: host_proxy") {
		t.Fatalf("providers apply playbook must run managed Squid before host_proxy\n%s", providerTasks)
	}
	for _, role := range []string{
		"cluster_infra/substrate_libvirt",
		"cluster_infra/substrate_baremetal",
		"providers/bmc_emulated",
		"providers/bmc_redfish",
		"providers/bmc_none",
		"providers/proxy_squid",
		"providers/boot_artifacts_http",
		"openshift/boot_emulated",
		"openshift/boot_redfish",
	} {
		path := "../../../ansible/roles/" + role + "/tasks/main.yml"
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected role tasks at %s: %v", path, err)
		}
	}
}

func TestProviderDestroyRemovesManagedProxyWithoutBroadNetworkBlocks(t *testing.T) {
	destroy := readFile(t, "../../../ansible/roles/providers/proxy_squid/tasks/destroy.yml")
	for _, expected := range []string{
		"bootwright_current_forward_proxy",
		"bootwright-squid-{{ bootwright_current_forward_proxy.name }}",
		"Close managed proxy firewall port",
		"Remove managed proxy state directory",
	} {
		if !strings.Contains(destroy, expected) {
			t.Fatalf("proxy_squid destroy task missing managed proxy cleanup %q\n%s", expected, destroy)
		}
	}
	for _, unexpected := range []string{
		"iptables",
		"nft ",
		"--add-rich-rule",
		"--direct",
	} {
		if strings.Contains(destroy, unexpected) {
			t.Fatalf("proxy_squid destroy task contains broad network rule %q\n%s", unexpected, destroy)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func copyYAMLFixture(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read fixture dir %s: %v", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read fixture %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", entry.Name(), err)
		}
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func managedProxyRenderYAML(name, providerName, cidr, gateway, apiVIP, ingressVIP, nodeIP string) string {
	return strings.Replace(strings.Replace(strings.Replace(strings.Replace(fmt.Sprintf(`apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: env-%s
spec:
  baseDomain: example.com
  ocpInstallType: connected
  proxy:
    auth:
      proxyAuthRef:
        name: proxy-credentials
  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
    default-key:
      file: ./default-key
    proxy-credentials:
      generated:
        credentials:
          username: proxy
---
apiVersion: bootwright.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: %s
spec:
  hosts:
    host-01:
      ssh:
        address: 10.0.0.1
        user: bootwright
        keyRef:
          name: default-key
      capabilities:
        - libvirt
        - proxy
  machine:
    libvirt:
      hostRefs:
        - name: host-01
      bmcEmulation: {}
  proxy:
    squid:
      hostRef:
        name: host-01
---
apiVersion: bootwright.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: %s
spec:
  providerRefs:
    - name: %s
  networks:
    primary:
      cidr: %s
      gateway: %s
      libvirt:
        bridge: virbr0
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: %s
      libvirt:
        hostRef:
          name: host-01
  endpoints:
    api:
      address: %s
    apiInt:
      address: %s
    ingress:
      address: %s
---
apiVersion: bootwright.io/v1alpha1
kind: OCPCluster
metadata:
  name: %s
spec:
  topology: single-node
  infrastructureRef:
    name: %s
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`, name, providerName, name, providerName, cidr, gateway, nodeIP, apiVIP, apiVIP, ingressVIP, name, name), "\t", "  ", -1), "\r\n", "\n", -1), "\r", "\n", -1), "\n\n\n", "\n\n", -1)
}

func replaceOnce(t *testing.T, input, old, new string) string {
	t.Helper()
	if !strings.Contains(input, old) {
		t.Fatalf("missing fixture fragment %q", old)
	}
	return strings.Replace(input, old, new, 1)
}

func assertInstallConfigProxy(t *testing.T, path, httpProxy, httpsProxy string, requiredNoProxy []string) {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatalf("unmarshal install-config %s: %v", path, err)
	}
	proxy, ok := doc["proxy"].(map[string]any)
	if !ok {
		t.Fatalf("install-config %s missing proxy block: %#v", path, doc["proxy"])
	}
	for _, item := range []struct {
		key  string
		want string
	}{
		{key: "httpProxy", want: httpProxy},
		{key: "httpsProxy", want: httpsProxy},
	} {
		got, ok := proxy[item.key].(string)
		if !ok || got != item.want {
			t.Fatalf("install-config %s proxy.%s got %#v, want %q", path, item.key, proxy[item.key], item.want)
		}
	}
	got, _ := proxy["noProxy"].(string)
	entries := map[string]bool{}
	for _, e := range strings.Split(got, ",") {
		entries[strings.TrimSpace(e)] = true
	}
	for _, want := range requiredNoProxy {
		if !entries[want] {
			t.Fatalf("install-config %s proxy.noProxy missing %q (got %q)", path, want, got)
		}
	}
}
