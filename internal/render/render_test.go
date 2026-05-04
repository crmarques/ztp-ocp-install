package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
)

func TestRenderAllProducesGeneratedAnsibleArtifacts(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/infra"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
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
	inventory := readFile(t, result.InventoryPath)
	for _, expected := range []string{
		"hub-hub-sno-host:",
		"spoke-01-spoke-01-host:",
		"spoke-02-spoke-02-host:",
		"gitups_cluster_name: hub",
	} {
		if !strings.Contains(inventory, expected) {
			t.Fatalf("inventory missing %q\n%s", expected, inventory)
		}
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"api.hub.example.com",
		"api-int.spoke-01.example.com",
		"*.apps.spoke-02.example.com",
		"infrastructureHosts:",
		"virtualization:",
		"bmc:",
		"port: 8000",
		"nodes:",
		"memoryMiB: 22528",
		"macAddress: 52:54:00:",
		"name: sushy-tools",
		"version: 2.2.0",
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
		"version: 2.20.4",
		"name: gopkg.in/yaml.v3",
		"version: v3.0.1",
	} {
		if !strings.Contains(lock, expected) {
			t.Fatalf("lock missing %q\n%s", expected, lock)
		}
	}
}

func TestRenderEmitsBMCAuthCredentialRefWhenSet(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/qemu-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"auth:",
		"credentialRef: qemu-1-host-bmc-credentials",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
}

func TestRenderInstallerAssets(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/infra"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	asset := result.InstallerAssets[0]
	if got, want := asset.InstallConfigPath, filepath.Join(stateDir, "clusters", "hub", "installer", "install-config.yaml"); got != want {
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
		"gitups-secret-ref:openshift-pull-secret",
		"gitups-ssh-key-ref:cluster-admin-key",
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
		data, err := os.ReadFile(filepath.Join("../../examples/infra", name))
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
	result, err := All(stateDir, state)
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
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/infra"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
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

func TestRenderOneHostUsesLocalConnection(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/qemu-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	inventory := readFile(t, result.InventoryPath)
	for _, expected := range []string{
		"ansible_host: localhost",
		"ansible_connection: local",
		"gitups_cluster_name: qemu-1-host-hub",
	} {
		if !strings.Contains(inventory, expected) {
			t.Fatalf("inventory missing %q\n%s", expected, inventory)
		}
	}
}

func TestRenderVarsExposeOCPInstallMetadata(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/qemu-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"gitups_ocp_install:",
		"mode: disconnected",
		"disconnected: true",
		"method: agent",
		"pullSecretRef: openshift-pull-secret",
		"sshKeyRef: cluster-admin-key",
		"releaseImageOverride: registry.mirror.test:5000/openshift/release-images:4.21.10-x86_64",
		"additionalTrustBundleRef: mirror-registry-ca",
		"version: 4.21.10",
		"channel: stable-4.21",
		"relativeDir: clusters/qemu-1-host-hub/installer",
		"relativeInstallConfigPath: clusters/qemu-1-host-hub/installer/install-config.yaml",
		"relativeAgentConfigPath: clusters/qemu-1-host-hub/installer/agent-config.yaml",
		"name: openshift-install",
		"machineRef: master-0",
		"localRegistry:",
		"url: registry.mirror.test:5000",
		"host: registry.mirror.test",
		"credentialsRef: mirror-registry-credentials",
		"trustBundleRef: mirror-registry-ca",
		"dnsHosts:",
		"ip: 192.168.130.1",
		"- registry.mirror.test",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
	if strings.Contains(varsFile, stateDir) {
		t.Fatalf("vars.yaml leaked stateDir %q (paths must be relative)\n%s", stateDir, varsFile)
	}
	lock := readFile(t, result.LockPath)
	if !strings.Contains(lock, "name: openshift-install") || !strings.Contains(lock, "version: 4.21.10") {
		t.Fatalf("lock missing openshift-install pin\n%s", lock)
	}
	asset := installerAssetFor(result.InstallerAssets, "qemu-1-host-hub")
	installConfig := readFile(t, asset.InstallConfigPath)
	for _, expected := range []string{
		"imageDigestSources:",
		"registry.mirror.test:5000/openshift/release-images",
		"registry.mirror.test:5000/openshift/release",
		"sourcePolicy: NeverContactSource",
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

func TestHubInstallRoleDoesNotBlockPublicRegistries(t *testing.T) {
	tasks := readFile(t, "../../ansible/roles/hub_install_agent/tasks/main.yml")
	for _, unexpected := range []string{
		"127.0.0.1:1",
		"HTTPS_PROXY",
		"HTTP_PROXY",
		"ALL_PROXY",
		"sinkhole",
	} {
		if strings.Contains(tasks, unexpected) {
			t.Fatalf("hub install role contains public-registry blocking behavior %q\n%s", unexpected, tasks)
		}
	}
}

func TestLibvirtSubstrateOpensBootArtifactsHTTPPort(t *testing.T) {
	tasks := readFile(t, "../../ansible/roles/cluster_substrate_libvirt/tasks/main.yml")
	for _, expected := range []string{
		"<dhcp>",
		"<host mac='{{ node.macAddress }}'",
		"name='{{ gitups_current_cluster.name }}-{{ node.name }}'",
		"ip='{{ node.ipAddress }}'",
		"--get-zone-of-interface={{ gitups_current_cluster.provider.virtualization.libvirt.bridge }}",
		"--add-port={{ gitups_current_cluster.provider.bootArtifactsHttp.port | int }}/tcp",
		"Plumb cluster load balancer VIPs onto libvirt bridges",
	} {
		if !strings.Contains(tasks, expected) {
			t.Fatalf("cluster_substrate_libvirt is missing %q\n%s", expected, tasks)
		}
	}
	for _, leak := range []string{
		"provider.bmc.port",
		"provider.bmc.enabled",
	} {
		if strings.Contains(tasks, leak) {
			t.Fatalf("cluster_substrate_libvirt still references BMC field %q (extraction incomplete)", leak)
		}
	}
}

func TestRenderVarsExposeGeneratedSecrets(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"provider.yaml", "hub.yaml"} {
		data, err := os.ReadFile(filepath.Join("../../examples/infra", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "environment.yaml"), []byte(`apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: example
spec:
  baseDomain: example.com
  ocpInstall:
    disconnected:
      registries:
        mirror:
          url: registry.lab.test:5000
          credentialsRef:
            name: registry-lab-credentials
          trustBundle:
            generatedSelfSigned:
              secretRef:
                name: registry-lab-ca
              commonName: registry.lab.test
        imageDigestSources:
          - source: quay.io/openshift-release-dev/ocp-release
            mirrors:
              - registry.lab.test:5000/openshift/release-images
          - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
            mirrors:
              - registry.lab.test:5000/openshift/release
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.10
`), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}
	// drop the spoke fixtures so we only need the hub for this test
	state, err := infra.LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
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
		"../../test/e2e/qemu-1-host-1-sno-hub/cluster-infrastructure-hub.yaml",
		"../../test/e2e/qemu-1-host-1-sno-hub/ocp-cluster-hub.yaml",
		"../../test/e2e/qemu-1-host-1-sno-hub/provider.yaml",
		"../../test/e2e/qemu-1-host-1-sno-hub/environment.yaml",
	})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
	if err != nil {
		t.Fatalf("render All returned error: %v", err)
	}
	varsFile := readFile(t, result.VarsPath)
	for _, expected := range []string{
		"local: registry.mirror.test:5000/library/haproxy:3.2.15",
		"public: docker.io/library/haproxy:3.2.15",
		"runtime: podman",
		"bindAddress: 192.168.130.10",
		"targetPort: 6443",
		"backends:",
		"address: 192.168.130.20",
		"console-openshift-console.apps.qemu-1-host-hub.gitups.test",
		"name: haproxy",
		"version: 3.2.15",
	} {
		if !strings.Contains(varsFile, expected) {
			t.Fatalf("vars missing %q\n%s", expected, varsFile)
		}
	}
}

func TestRenderBareMetalProjectsPerMachineBMC(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/baremetal-redfish-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, state)
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

func TestProviderDispatchCoversAllKinds(t *testing.T) {
	tasks := readFile(t, "../../ansible/playbooks/infra-prepare.yml")
	for _, expected := range []string{
		"cluster_substrate_{{ gitups_current_cluster.provider.substrateRole }}",
		"provider_bmc_{{ gitups_current_provider.bmcRole }}",
		"gitups_current_provider.bootArtifactsHttp.enabled",
	} {
		if !strings.Contains(tasks, expected) {
			t.Fatalf("infra-prepare.yml missing dispatch fragment %q\n%s", expected, tasks)
		}
	}
	// Dynamic dispatch implies every kind resolves to a real role.
	for _, role := range []string{
		"cluster_substrate_libvirt",
		"cluster_substrate_baremetal",
		"provider_bmc_emulated",
		"provider_bmc_redfish",
		"provider_bmc_none",
		"provider_boot_artifacts_http",
		"hub_boot_emulated",
		"hub_boot_redfish",
	} {
		path := "../../ansible/roles/" + role + "/tasks/main.yml"
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected role tasks at %s: %v", path, err)
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
