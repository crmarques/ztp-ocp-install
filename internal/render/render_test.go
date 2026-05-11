package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/infra"
)

func TestRenderResolvesFileBasedSecretsToSourcePath(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Keys: map[string]v1alpha1.EnvironmentKeySpec{
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
	if got, want := vars.GitupsClusters[0].OCP.Install.SSHKeyRef, "/tmp/foo"; got != want {
		t.Fatalf("file-based SSHKeyRef got %q, want %q", got, want)
	}
	if got := vars.GitupsClusters[0].OCP.Install.SSHKeyRef; strings.HasPrefix(got, "/anywhere") {
		t.Fatalf("file-based SSHKeyRef must not use secretsDir prefix, got %q", got)
	}
}

func TestRenderAllProducesGeneratedAnsibleArtifacts(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-lab-fleet"})
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
		"gitups_cluster_name: hub",
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
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/libvirt-1-host-1-sno-hub"})
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
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-lab-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	stateDir := t.TempDir()
	result, err := All(stateDir, "", state)
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
		data, err := os.ReadFile(filepath.Join("../../examples/libvirt-redfish-lab-fleet", name))
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
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-lab-fleet"})
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
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
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
		"gitups_mirror_registries:",
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

func TestRenderOneHostTreatsLocalhostAsProviderHost(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
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
		"ansible_connection: local",
		"gitups_cluster_name: local-libvirt-1-host-hub",
	} {
		if !strings.Contains(inventory, expected) {
			t.Fatalf("inventory missing %q\n%s", expected, inventory)
		}
	}
	if strings.Contains(inventory, "ansible_become:") {
		t.Fatalf("provider root escalation belongs on mutating playbooks, not inventory\n%s", inventory)
	}
	playbook := readFile(t, "../../ansible/playbooks/clusters-destroy.yml")
	if !strings.Contains(playbook, "become: true") {
		t.Fatalf("clusters-destroy.yml must keep provider-host root escalation\n%s", playbook)
	}
}

func TestRenderVarsExposeOCPInstallMetadata(t *testing.T) {
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/local-libvirt-1-host-1-sno-hub"})
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
		"gitups_ocp_install:",
		"mode: disconnected",
		"disconnected: true",
		"method: agent",
		"pullSecretRef: " + filepath.Join(home, ".gitups/secrets/openshift-pull-secret"),
		"sshKeyRef: " + filepath.Join(home, ".ssh/gitups-ssh-key.pub"),
		"releaseImageOverride: registry.mirror.local:5000/openshift/release-images:4.21.12-x86_64",
		"additionalTrustBundleRef: mirror-registry-ca",
		"version: 4.21.12",
		"channel: stable-4.21",
		"relativeDir: clusters/local-libvirt-1-host-hub/installer",
		"relativeInstallConfigPath: clusters/local-libvirt-1-host-hub/installer/install-config.yaml",
		"relativeAgentConfigPath: clusters/local-libvirt-1-host-hub/installer/agent-config.yaml",
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
		"additionalTrustBundle: <gitups-trust-bundle-ref:mirror-registry-ca>",
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
	tasksDir := "../../ansible/roles/ocp_install_agent/tasks"
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
				t.Fatalf("ocp_install_agent/%s contains public-registry blocking behavior %q\n%s", e.Name(), unexpected, body)
			}
		}
	}
}

func TestOCPInstallRoleDoesNotShadowEnvironmentInstallVars(t *testing.T) {
	preflight := readFile(t, "../../ansible/roles/ocp_install_agent/tasks/preflight.yml")
	secrets := readFile(t, "../../ansible/roles/ocp_install_agent/tasks/secrets.yml")
	combined := preflight + "\n" + secrets
	for _, expected := range []string{
		"gitups_ocp_cluster_install: \"{{ gitups_current_cluster.ocp.install }}\"",
		"gitups_ocp_cluster_install.method",
		"gitups_ocp_cluster_install.generatedSecrets",
		"gitups_ocp_cluster_install.pullSecretRef",
		"gitups_ocp_cluster_install.sshKeyRef",
		"gitups_ocp_cluster_install.additionalTrustBundleRef",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("ocp_install_agent must use cluster install fact %q\n%s", expected, combined)
		}
	}
	for _, unexpected := range []string{
		"gitups_ocp_install: \"{{ gitups_current_cluster.ocp.install }}\"",
		"gitups_ocp_install.method",
		"gitups_ocp_install.generatedSecrets",
		"gitups_ocp_install.pullSecretRef",
		"gitups_ocp_install.sshKeyRef",
		"gitups_ocp_install.additionalTrustBundleRef",
	} {
		if strings.Contains(combined, unexpected) {
			t.Fatalf("ocp_install_agent shadows environment install vars with %q\n%s", unexpected, combined)
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
		"ansible.posix.firewalld",
		"zone: libvirt",
		"interface: \"{{ gitups_current_cluster.provider.virtualization.libvirt.bridge }}\"",
		"port: \"{{ gitups_current_cluster.provider.bootArtifactsHttp.port | int }}/tcp\"",
	} {
		if !strings.Contains(tasks, expected) {
			t.Fatalf("cluster_substrate_libvirt is missing %q\n%s", expected, tasks)
		}
	}
	for _, leak := range []string{
		"provider.bmc.port",
		"provider.bmc.enabled",
		"gitups_load_balancers",
		"Plumb cluster load balancer VIPs",
		"--get-zone-of-interface",
		"--add-port=",
		"--change-interface=",
	} {
		if strings.Contains(tasks, leak) {
			t.Fatalf("cluster_substrate_libvirt still references %q — extraction is incomplete", leak)
		}
	}
}

func TestClusterNetworkVipsOwnsVIPPlumbing(t *testing.T) {
	apply := readFile(t, "../../ansible/roles/cluster_network_vips/tasks/main.yml")
	for _, expected := range []string{
		"Plumb cluster load balancer VIPs onto libvirt bridges",
		"gitups_in_cidr",
		"gitups_load_balancers",
	} {
		if !strings.Contains(apply, expected) {
			t.Fatalf("cluster_network_vips/tasks/main.yml missing %q\n%s", expected, apply)
		}
	}
	destroy := readFile(t, "../../ansible/roles/cluster_network_vips/tasks/destroy.yml")
	if !strings.Contains(destroy, "Unplumb managed load balancer VIPs from the cluster bridge") {
		t.Fatalf("cluster_network_vips/tasks/destroy.yml missing unplumb task\n%s", destroy)
	}
	if _, err := os.Stat("../../ansible/roles/cluster_network_vips/test_plugins/cidr.py"); err != nil {
		t.Fatalf("cluster_network_vips/test_plugins/cidr.py must own the gitups_in_cidr plugin: %v", err)
	}
}

func TestRenderVarsExposeGeneratedSecrets(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"hub.yaml"} {
		data, err := os.ReadFile(filepath.Join("../../examples/libvirt-redfish-lab-fleet", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	providerData, err := os.ReadFile(filepath.Join("../../examples/libvirt-redfish-lab-fleet", "provider.yaml"))
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
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  keys:
    openshift-pull-secret:
      file: ~/.gitups/secrets/openshift-pull-secret
    cluster-admin-key:
      file: ~/.ssh/gitups-ssh-key.pub
    lab-provider-key:
      file: ~/.ssh/gitups-ssh-key
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
		"../../test/e2e/old/libvirt-1-host-1-sno-hub/cluster-infrastructure-hub.yaml",
		"../../test/e2e/old/libvirt-1-host-1-sno-hub/ocp-cluster-hub.yaml",
		"../../test/e2e/old/libvirt-1-host-1-sno-hub/provider.yaml",
		"../../test/e2e/old/libvirt-1-host-1-sno-hub/environment.yaml",
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
		"console-openshift-console.apps.libvirt-1-host-hub.gitups.test",
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
	state, err := infra.LoadNormalizeValidate([]string{"../../test/e2e/old/baremetal-redfish-fleet"})
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
	state, err := infra.LoadNormalizeValidate([]string{"../../examples/baremetal-edge-lb-fleet"})
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

func TestProviderDispatchCoversAllKinds(t *testing.T) {
	clusterTasks := readFile(t, "../../ansible/playbooks/cluster-prepare.yml")
	if !strings.Contains(clusterTasks, "cluster_substrate_{{ gitups_current_cluster.provider.substrateRole }}") {
		t.Fatalf("cluster-prepare.yml missing substrate dispatch fragment\n%s", clusterTasks)
	}
	providerTasks := readFile(t, "../../ansible/playbooks/provider-prepare.yml")
	for _, expected := range []string{
		"provider_bmc_{{ gitups_current_provider.bmcRole }}",
		"gitups_current_provider.bootArtifactsHttp.enabled",
	} {
		if !strings.Contains(providerTasks, expected) {
			t.Fatalf("provider-prepare.yml missing dispatch fragment %q\n%s", expected, providerTasks)
		}
	}
	for _, role := range []string{
		"cluster_substrate_libvirt",
		"cluster_substrate_baremetal",
		"provider_bmc_emulated",
		"provider_bmc_redfish",
		"provider_bmc_none",
		"provider_boot_artifacts_http",
		"ocp_boot_emulated",
		"ocp_boot_redfish",
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
