package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

func TestLoadNormalizeValidateExamples(t *testing.T) {
	state, err := LoadNormalizeValidate([]string{"../../examples/infra"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	if got, want := len(state.InfrastructureProviders), 3; got != want {
		t.Fatalf("got %d providers, want %d", got, want)
	}
	if got, want := len(state.Environments), 1; got != want {
		t.Fatalf("got %d environments, want %d", got, want)
	}
	if got, want := len(state.ClusterInfrastructures), 3; got != want {
		t.Fatalf("got %d cluster infrastructures, want %d", got, want)
	}
	if got, want := len(state.OCPClusters), 3; got != want {
		t.Fatalf("got %d OCP clusters, want %d", got, want)
	}
	if got, want := state.ClusterInfrastructures[0].Metadata.Name, "hub"; got != want {
		t.Fatalf("first sorted cluster infrastructure got %q, want %q", got, want)
	}
	machineCount := 0
	for _, item := range state.ClusterInfrastructures {
		if item.APIVersion != v1alpha1.APIVersion {
			t.Fatalf("%s apiVersion got %q", item.Metadata.Name, item.APIVersion)
		}
		if item.Kind != v1alpha1.KindClusterInfrastructure {
			t.Fatalf("%s kind got %q", item.Metadata.Name, item.Kind)
		}
		machineCount += len(item.Spec.Machines)
		for name, machine := range item.Spec.Machines {
			for ifaceName, iface := range machine.Interfaces {
				if iface.MACAddress == "" {
					t.Fatalf("%s machine %s interface %s missing generated MAC address", item.Metadata.Name, name, ifaceName)
				}
			}
		}
	}
	if got, want := machineCount, 7; got != want {
		t.Fatalf("got %d machines, want %d", got, want)
	}
	for _, ocp := range state.OCPClusters {
		if ocp.Spec.Install.BaseDomain != "example.com" {
			t.Fatalf("%s baseDomain not resolved from environment: %q", ocp.Metadata.Name, ocp.Spec.Install.BaseDomain)
		}
		if ocp.Spec.Install.PullSecretRef.Name != "openshift-pull-secret" {
			t.Fatalf("%s pullSecretRef not resolved from environment: %q", ocp.Metadata.Name, ocp.Spec.Install.PullSecretRef.Name)
		}
	}
}

func TestLoadNormalizeValidateOneHostSample(t *testing.T) {
	state, err := LoadNormalizeValidate([]string{"../../test/e2e/qemu-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	if got, want := len(state.InfrastructureProviders), 1; got != want {
		t.Fatalf("got %d providers, want %d", got, want)
	}
	if got, want := len(state.ClusterInfrastructures), 1; got != want {
		t.Fatalf("got %d cluster infrastructures, want %d", got, want)
	}
	totalLBs := 0
	for _, item := range state.ClusterInfrastructures {
		totalLBs += len(item.Spec.LoadBalancers)
	}
	if got, want := totalLBs, 1; got != want {
		t.Fatalf("got %d load balancers, want %d", got, want)
	}
	if got, want := state.InfrastructureProviders[0].Spec.Machine.Libvirt.BMCEmulation.Port, 8000; got != want {
		t.Fatalf("provider BMC port got %d, want %d", got, want)
	}
	for _, item := range state.ClusterInfrastructures {
		machine, ok := item.Spec.Machines["master-0"]
		if !ok {
			t.Fatalf("%s missing master-0 machine", item.Metadata.Name)
		}
		if got, want := machine.Libvirt.HostRef.Name, "local-qemu-host"; got != want {
			t.Fatalf("%s machine hostRef got %q, want %q", item.Metadata.Name, got, want)
		}
		if machine.Resources == nil || machine.Resources.CPU != 8 || machine.Resources.MemoryMiB != 20480 {
			t.Fatalf("%s machine resources got %+v, want 8 CPU, 20480 MiB", item.Metadata.Name, machine.Resources)
		}
	}
}

func TestLegacyKindRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	writeFile(t, path, `apiVersion: gitups.io/v1alpha1
kind: Cluster
metadata:
  name: legacy
spec: {}
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected unsupported kind error for legacy Cluster")
	}
	if !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("expected unsupported-kind error, got %v", err)
	}
}

func TestLoadRejectsLocalRegistryField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-localregistry.yaml")
	writeFile(t, path, `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: legacy
spec:
  baseDomain: example.com
  localRegistry:
    enabled: true
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected error for legacy Environment.spec.localRegistry")
	}
	if !strings.Contains(err.Error(), "localRegistry") {
		t.Fatalf("expected unknown-field localRegistry error, got %v", err)
	}
}

func TestLoadRejectsProviderSpecType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "with-type.yaml")
	writeFile(t, path, `apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: bad
spec:
  type: qemu-kvm
  qemuKVM: {}
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected error for spec.type on InfrastructureProvider")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Fatalf("expected unknown-field type error, got %v", err)
	}
}

func TestLoadRejectsEndpointsOnOCPCluster(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoints-on-ocp.yaml")
	writeFile(t, path, `apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: bad
spec:
  role: hub
  infrastructureRef:
    name: bad
  endpoints:
    api:
      address: 10.0.0.10
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected error for endpoints on OCPCluster")
	}
	if !strings.Contains(err.Error(), "endpoints") {
		t.Fatalf("expected unknown-field endpoints error, got %v", err)
	}
}

func TestDefaultsAreApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.yaml")
	writeFile(t, path, validStateYAML("minimal", "minimal-provider", "192.168.150.0/24", "192.168.150.10", "192.168.150.11", "192.168.150.20"))
	state, err := LoadNormalizeValidate([]string{path})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	provider := state.InfrastructureProviders[0]
	host, ok := provider.Spec.Hosts["host-01"]
	if !ok {
		t.Fatalf("host-01 missing from provider")
	}
	if host.SSH == nil {
		t.Fatalf("host-01 missing ssh connection")
	}
	if got, want := host.SSH.User, "root"; got != want {
		t.Fatalf("default user got %q, want %q", got, want)
	}
	if provider.Spec.Machine.Libvirt.BMCEmulation.Enabled == nil || !*provider.Spec.Machine.Libvirt.BMCEmulation.Enabled {
		t.Fatalf("expected BMC enabled default")
	}
	if got, want := provider.Spec.Machine.Libvirt.BMCEmulation.Port, 8000; got != want {
		t.Fatalf("default BMC port got %d, want %d", got, want)
	}
	item := state.ClusterInfrastructures[0]
	machine, ok := item.Spec.Machines["master-0"]
	if !ok {
		t.Fatalf("master-0 missing")
	}
	if machine.Resources == nil || machine.Resources.MemoryMiB != 22528 {
		t.Fatalf("default machine memory got %+v, want 22528", machine.Resources)
	}
	for ifaceName, iface := range machine.Interfaces {
		if iface.MACAddress == "" {
			t.Fatalf("expected generated MAC for interface %s", ifaceName)
		}
	}
	ocp := state.OCPClusters[0]
	node, ok := ocp.Spec.Nodes["master-0"]
	if !ok {
		t.Fatalf("master-0 node missing")
	}
	if node.MachineRef == nil || node.MachineRef.Name != "master-0" {
		t.Fatalf("machineRef should default to node name; got %+v", node.MachineRef)
	}
}

func TestValidationRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.yaml"), validStateYAML("duplicate", "provider-a", "192.168.151.0/24", "192.168.151.10", "192.168.151.11", "192.168.151.20"))
	writeFile(t, filepath.Join(dir, "second.yaml"), validStateYAML("duplicate", "provider-b", "192.168.152.0/24", "192.168.152.10", "192.168.152.11", "192.168.152.20"))
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected duplicate name validation error")
	}
	if !strings.Contains(err.Error(), "duplicate ClusterInfrastructure") {
		t.Fatalf("expected duplicate ClusterInfrastructure name error, got %v", err)
	}
}

func TestValidationRejectsInvalidProviderHostRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	body := strings.Replace(validStateYAML("bad-host-ref", "bad-host-provider", "192.168.153.0/24", "192.168.153.10", "192.168.153.11", "192.168.153.20"), "name: host-01\n", "name: missing-host\n", 1)
	writeFile(t, path, body)
	_, err := LoadNormalizeValidate([]string{path})
	if err == nil {
		t.Fatal("expected hostRef validation error")
	}
	if !strings.Contains(err.Error(), "missing-host") {
		t.Fatalf("expected hostRef error, got %v", err)
	}
}

func TestValidationRejectsRegistriesUnderConnected(t *testing.T) {
	dir := t.TempDir()
	// `connected` is structural: it carries no fields. Registries declared next
	// to it (mode-style) become a `registries` key the schema does not know,
	// so the load-time strict decoder must reject it before normalization runs.
	body := strings.Replace(
		validStateYAML("connected-with-registries", "connected-provider", "192.168.154.0/24", "192.168.154.10", "192.168.154.11", "192.168.154.20"),
		"  ocpInstall:\n    connected: {}\n",
		"  ocpInstall:\n    connected: {}\n    registries:\n      mirror:\n        url: registry.lab.test:5000\n",
		1,
	)
	writeFile(t, filepath.Join(dir, "connected.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected unknown-field rejection for registries declared next to connected: {}")
	}
	if !strings.Contains(err.Error(), "registries") {
		t.Fatalf("expected unknown-field registries error, got %v", err)
	}
}

func TestValidationRejectsAgentConfigMinimalISOOverride(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(
		validStateYAML("disconnected-full-iso", "disconnected-provider", "192.168.155.0/24", "192.168.155.10", "192.168.155.11", "192.168.155.20"),
		"  ocpInstall:\n    connected: {}\n",
		"  ocpInstall:\n    disconnected:\n      registries:\n        mirror:\n          url: registry.lab.test:5000\n          credentialsRef:\n            name: registry-lab-credentials\n          trustBundle:\n            bundleRef:\n              name: registry-lab-ca\n",
		1,
	)
	body = strings.Replace(body, "  install:\n    method: agent\n", "  install:\n    method: agent\n    agentConfigOverrides:\n      minimalISO: false\n", 1)
	writeFile(t, filepath.Join(dir, "disconnected-full-iso.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "agentConfigOverrides[minimalISO]") {
		t.Fatalf("expected agentConfigOverrides[minimalISO] validation error, got %v", err)
	}
}

func TestDisconnectedDefaultsImageSourcesToNeverContactSource(t *testing.T) {
	state, err := LoadNormalizeValidate([]string{"../../test/e2e/qemu-1-host-1-sno-hub"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	for _, ocp := range state.OCPClusters {
		for _, source := range ocp.Spec.Install.ImageDigestSources {
			if source.SourcePolicy != v1alpha1.ImageSourcePolicyNever {
				t.Fatalf("%s imageDigestSources[%s].sourcePolicy got %q, want %q", ocp.Metadata.Name, source.Source, source.SourcePolicy, v1alpha1.ImageSourcePolicyNever)
			}
		}
	}
}

func TestValidationRejectsDisconnectedAllowContactingSource(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(
		validStateYAML("disconnected-source-policy", "disconnected-source-policy-provider", "192.168.156.0/24", "192.168.156.10", "192.168.156.11", "192.168.156.20"),
		"  ocpInstall:\n    connected: {}\n",
		"  ocpInstall:\n    disconnected:\n      registries:\n        mirror:\n          url: registry.lab.test:5000\n          credentialsRef:\n            name: registry-lab-credentials\n          trustBundle:\n            bundleRef:\n              name: registry-lab-ca\n        imageDigestSources:\n          - source: quay.io/openshift-release-dev/ocp-release\n            mirrors:\n              - registry.lab.test:5000/openshift/release-images\n            sourcePolicy: AllowContactingSource\n          - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev\n            mirrors:\n              - registry.lab.test:5000/openshift/release\n",
		1,
	)
	writeFile(t, filepath.Join(dir, "disconnected-source-policy.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected sourcePolicy validation error")
	}
	if !strings.Contains(err.Error(), "sourcePolicy must not allow contacting the source") {
		t.Fatalf("expected disconnected sourcePolicy validation error, got %v", err)
	}
}

func TestValidationRejectsDisconnectedExternalOpenShiftMirror(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(
		validStateYAML("disconnected-external-mirror", "disconnected-external-mirror-provider", "192.168.157.0/24", "192.168.157.10", "192.168.157.11", "192.168.157.20"),
		"  ocpInstall:\n    connected: {}\n",
		"  ocpInstall:\n    disconnected:\n      registries:\n        mirror:\n          url: registry.lab.test:5000\n          credentialsRef:\n            name: registry-lab-credentials\n          trustBundle:\n            bundleRef:\n              name: registry-lab-ca\n        imageDigestSources:\n          - source: quay.io/openshift-release-dev/ocp-release\n            mirrors:\n              - quay.io/openshift-release-dev/ocp-release\n          - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev\n            mirrors:\n              - registry.lab.test:5000/openshift/release\n",
		1,
	)
	writeFile(t, filepath.Join(dir, "disconnected-external-mirror.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected external mirror validation error")
	}
	if !strings.Contains(err.Error(), "must use disconnected mirror") {
		t.Fatalf("expected disconnected mirror validation error, got %v", err)
	}
}

func TestBareMetalAndVMwareSchemasValidate(t *testing.T) {
	tests := map[string]string{
		"baremetal": bareMetalStateYAML(),
		"vmware":    vmwareStateYAML(),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name+".yaml")
			writeFile(t, path, body)
			if _, err := LoadNormalizeValidate([]string{path}); err != nil {
				t.Fatalf("LoadNormalizeValidate returned error: %v", err)
			}
		})
	}
}

// minimal four-domain-layer YAML factory for table-driven tests.
func validStateYAML(name string, providerName string, cidr string, apiVIP string, ingressVIP string, nodeIP string) string {
	envName := "env-" + name
	return fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: %s
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
  name: %s
spec:
  hosts:
    host-01:
      ssh:
        address: 10.0.0.1
        keyRef:
          name: default-key
      capabilities:
        - libvirt
  machine:
    libvirt:
      hostRefs:
        - name: host-01
      bmcEmulation: {}
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: %s
spec:
  providerRefs:
    - name: %s
  networks:
    primary:
      cidr: %s
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
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: %s
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: %s
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`, envName, providerName, name, providerName, cidr, nodeIP, apiVIP, apiVIP, ingressVIP, name, name)
}

func bareMetalStateYAML() string {
	return `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: bm-env
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
  name: baremetal-provider
spec:
  machine:
    baremetal: {}
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: baremetal
spec:
  providerRefs:
    - name: baremetal-provider
  networks:
    primary:
      cidr: 192.168.180.0/24
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.180.20
          macAddress: 52:54:00:00:00:20
      baremetal:
        bootMACAddress: 52:54:00:00:00:20
        bmc:
          address: redfish-virtualmedia+https://bmc-master-0.example.com/redfish/v1/Systems/1
          credentialRef:
            name: baremetal-bmc
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
  name: baremetal
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: baremetal
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`
}

func vmwareStateYAML() string {
	return `apiVersion: gitups.io/v1alpha1
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
      cidr: 192.168.181.0/24
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.181.20
          macAddress: 52:54:00:00:00:20
      vsphere:
        folder: example/vmware
        template: rhcos-4.21
  endpoints:
    api:
      address: 192.168.181.10
    apiInt:
      address: 192.168.181.10
    ingress:
      address: 192.168.181.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: vmware
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: vmware
  install:
    method: agent
  nodes:
    master-0:
      role: control-plane
`
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
