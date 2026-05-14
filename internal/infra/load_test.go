package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

func TestLoadNormalizeValidateExamples(t *testing.T) {
	state, err := LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-lab-fleet"})
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
	state, err := LoadNormalizeValidate([]string{"../../test/e2e/sno-libvirt"})
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
		if got, want := machine.Libvirt.HostRef.Name, "lab-host"; got != want {
			t.Fatalf("%s machine hostRef got %q, want %q", item.Metadata.Name, got, want)
		}
		if machine.Resources == nil || machine.Resources.CPU != 9 || machine.Resources.MemoryMiB != 19456 {
			t.Fatalf("%s machine resources got %+v, want 9 CPU, 19456 MiB", item.Metadata.Name, machine.Resources)
		}
	}
}

func TestLegacyKindRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	writeFile(t, path, `apiVersion: bootwright.io/v1alpha1
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

func TestLoadRejectsMissingAPIVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-api-version.yaml")
	writeFile(t, path, `kind: Environment
metadata:
  name: bad
spec:
  baseDomain: example.com
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected error for missing apiVersion")
	}
	if !strings.Contains(err.Error(), "apiVersion is required") {
		t.Fatalf("expected apiVersion error, got %v", err)
	}
}

func TestLoadRejectsMissingSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-spec.yaml")
	writeFile(t, path, `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: bad
`)
	_, err := Load([]string{path})
	if err == nil {
		t.Fatal("expected error for missing spec")
	}
	if !strings.Contains(err.Error(), "spec is required") {
		t.Fatalf("expected spec error, got %v", err)
	}
}

func TestLoadRejectsLocalRegistryField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-localregistry.yaml")
	writeFile(t, path, `apiVersion: bootwright.io/v1alpha1
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
	writeFile(t, path, `apiVersion: bootwright.io/v1alpha1
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
	writeFile(t, path, `apiVersion: bootwright.io/v1alpha1
kind: OCPCluster
metadata:
  name: bad
spec:
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
	if got, want := host.SSH.User, "bootwright"; got != want {
		t.Fatalf("ssh user got %q, want %q", got, want)
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

func TestNormalizeDefaultsRemoteHostUserToInvokingUser(t *testing.T) {
	t.Setenv("USER", "controller-user")
	t.Setenv("LOGNAME", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.yaml")
	body := strings.Replace(validStateYAML("minimal", "minimal-provider", "192.168.150.0/24", "192.168.150.10", "192.168.150.11", "192.168.150.20"), "        user: bootwright\n", "", 1)
	writeFile(t, path, body)
	state, err := LoadNormalizeValidate([]string{path})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	host := state.InfrastructureProviders[0].Spec.Hosts["host-01"]
	if got, want := host.SSH.User, "controller-user"; got != want {
		t.Fatalf("ssh user got %q, want %q", got, want)
	}
}

func TestValidationRejectsMultipleEnvironments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "case.yaml"), validStateYAML("multi-env", "multi-env-provider", "192.168.170.0/24", "192.168.170.10", "192.168.170.11", "192.168.170.20"))
	writeFile(t, filepath.Join(dir, "second-env.yaml"), `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: env-second
spec:
  baseDomain: other.example
  ocpInstallType: connected
  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
`)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected validation error for multiple Environment documents")
	}
	if !strings.Contains(err.Error(), "exactly one Environment is supported") {
		t.Fatalf("expected single-Environment error, got %v", err)
	}
}

func TestValidationRejectsMissingEnvironment(t *testing.T) {
	dir := t.TempDir()
	body := validStateYAML("no-env", "no-env-provider", "192.168.171.0/24", "192.168.171.10", "192.168.171.11", "192.168.171.20")
	envStart := strings.Index(body, "apiVersion: bootwright.io/v1alpha1\nkind: Environment")
	envEnd := strings.Index(body, "---\napiVersion: bootwright.io/v1alpha1\nkind: InfrastructureProvider")
	if envStart < 0 || envEnd < 0 {
		t.Fatalf("fixture is missing the Environment header it should strip")
	}
	body = body[envEnd+len("---\n"):]
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected validation error when no Environment is supplied")
	}
	if !strings.Contains(err.Error(), "at least one Environment is required") {
		t.Fatalf("expected at-least-one-Environment error, got %v", err)
	}
}

func TestValidationAcceptsManyOfShareableKinds(t *testing.T) {
	state, err := LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-lab-fleet"})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	if got := len(state.Environments); got != 1 {
		t.Fatalf("expected exactly 1 Environment, got %d", got)
	}
	if got := len(state.InfrastructureProviders); got < 2 {
		t.Fatalf("expected multiple InfrastructureProviders, got %d", got)
	}
	if got := len(state.ClusterInfrastructures); got < 2 {
		t.Fatalf("expected multiple ClusterInfrastructures, got %d", got)
	}
	if got := len(state.OCPClusters); got < 2 {
		t.Fatalf("expected multiple OCPClusters, got %d", got)
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

func TestValidationRejectsLegacyOCPInstallNesting(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(
		validStateYAML("legacy-ocp-install", "legacy-provider", "192.168.154.0/24", "192.168.154.10", "192.168.154.11", "192.168.154.20"),
		"  ocpInstallType: connected\n",
		"  ocpInstall:\n    connected: {}\n",
		1,
	)
	writeFile(t, filepath.Join(dir, "legacy.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected unknown-field rejection for legacy ocpInstall nesting")
	}
	if !strings.Contains(err.Error(), "ocpInstall") {
		t.Fatalf("expected unknown-field ocpInstall error, got %v", err)
	}
}

func TestValidationRejectsAgentConfigMinimalISOOverride(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(
		validStateYAML("disconnected-full-iso", "disconnected-provider", "192.168.155.0/24", "192.168.155.10", "192.168.155.11", "192.168.155.20"),
		"  ocpInstallType: connected\n",
		"  ocpInstallType: disconnected\n  registries:\n    mirror:\n      url: registry.lab.test:5000\n      credentialsRef:\n        name: registry-lab-credentials\n      trustBundleRef:\n        name: registry-lab-ca\n",
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
	state, err := LoadNormalizeValidate([]string{"../../examples/libvirt-redfish-hub"})
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
		"  ocpInstallType: connected\n",
		"  ocpInstallType: disconnected\n  registries:\n    mirror:\n      url: registry.lab.test:5000\n      credentialsRef:\n        name: registry-lab-credentials\n      trustBundleRef:\n        name: registry-lab-ca\n    imageDigestSources:\n      - source: quay.io/openshift-release-dev/ocp-release\n        mirrors:\n          - registry.lab.test:5000/openshift/release-images\n        sourcePolicy: AllowContactingSource\n      - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev\n        mirrors:\n          - registry.lab.test:5000/openshift/release-images\n",
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
		"  ocpInstallType: connected\n",
		"  ocpInstallType: disconnected\n  registries:\n    mirror:\n      url: registry.lab.test:5000\n      credentialsRef:\n        name: registry-lab-credentials\n      trustBundleRef:\n        name: registry-lab-ca\n    imageDigestSources:\n      - source: quay.io/openshift-release-dev/ocp-release\n        mirrors:\n          - quay.io/openshift-release-dev/ocp-release\n      - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev\n        mirrors:\n          - registry.lab.test:5000/openshift/release-images\n",
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

func TestLoadRejectsOldEnvironmentSecretShapes(t *testing.T) {
	tests := map[string]string{
		"old-keys-field": `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: old-keys
spec:
  baseDomain: example.com
  ocpInstallType: connected
  keys:
    openshift-pull-secret:
      file: ./pull-secret
`,
		"old-secret-refs": `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: old-secret-refs
spec:
  baseDomain: example.com
  ocpInstallType: connected
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "env.yaml"), body)
			if _, err := LoadNormalizeValidate([]string{dir}); err == nil {
				t.Fatal("expected old Environment secret shape to be rejected")
			}
		})
	}
}

func TestValidationRejectsMissingInheritedInstallSecrets(t *testing.T) {
	dir := t.TempDir()
	body := validStateYAML("missing-defaults", "missing-defaults-provider", "192.168.166.0/24", "192.168.166.10", "192.168.166.11", "192.168.166.20")
	body = strings.Replace(body, "    openshift-pull-secret:\n      file: ./pull-secret\n", "", 1)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected missing inherited pull secret rejection")
	}
	if !strings.Contains(err.Error(), "openshift-pull-secret") || !strings.Contains(err.Error(), "spec.secrets") {
		t.Fatalf("expected missing default secret error, got %v", err)
	}
}

func validStateYAML(name string, providerName string, cidr string, apiVIP string, ingressVIP string, nodeIP string) string {
	envName := "env-" + name
	return fmt.Sprintf(`apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: %s
spec:
  baseDomain: example.com
  ocpInstallType: connected
  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
    default-key:
      file: ./default-key
    registry-lab-credentials:
      generated:
        credentials:
          username: admin
    registry-lab-ca:
      generated:
        selfSignedCertificate:
          commonName: registry.lab.test
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
  machine:
    libvirt:
      hostRefs:
        - name: host-01
      bmcEmulation: {}
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
`, envName, providerName, name, providerName, cidr, nodeIP, apiVIP, apiVIP, ingressVIP, name, name)
}

func bareMetalStateYAML() string {
	return `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: bm-env
spec:
  baseDomain: example.com
  ocpInstallType: connected
  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
    baremetal-bmc:
      generated:
        credentials:
          username: admin
---
apiVersion: bootwright.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: baremetal-provider
spec:
  machine:
    baremetal: {}
---
apiVersion: bootwright.io/v1alpha1
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
apiVersion: bootwright.io/v1alpha1
kind: OCPCluster
metadata:
  name: baremetal
spec:
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
	return `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: vmware-env
spec:
  baseDomain: example.com
  ocpInstallType: connected
  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
    example-vcenter:
      file: ./vcenter
---
apiVersion: bootwright.io/v1alpha1
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
apiVersion: bootwright.io/v1alpha1
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
apiVersion: bootwright.io/v1alpha1
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
`
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const disconnectedRegistriesBlock = `  ocpInstallType: disconnected
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
`

func disconnectedYAML(name, providerName, cidr, apiVIP, ingressVIP, nodeIP string, withMirrorRegistry bool, mirrorRegistryPort int) string {
	body := strings.Replace(
		validStateYAML(name, providerName, cidr, apiVIP, ingressVIP, nodeIP),
		"  ocpInstallType: connected\n",
		disconnectedRegistriesBlock,
		1,
	)
	if withMirrorRegistry {
		hostCaps := "      capabilities:\n        - libvirt"
		hostCapsWithRegistry := hostCaps + "\n        - mirror-registry"
		body = strings.Replace(body, hostCaps, hostCapsWithRegistry, 1)
		registryBlock := "  registry:\n    mirrorRegistry:\n      hostRef:\n        name: host-01"
		if mirrorRegistryPort > 0 {
			registryBlock += fmt.Sprintf("\n      port: %d", mirrorRegistryPort)
		}
		registryBlock += "\n"
		body = strings.Replace(
			body,
			"  machine:\n    libvirt:\n      hostRefs:\n        - name: host-01",
			"  machine:\n    libvirt:\n      hostRefs:\n        - name: host-01",
			1,
		)
		body = strings.Replace(
			body,
			"---\napiVersion: bootwright.io/v1alpha1\nkind: ClusterInfrastructure",
			registryBlock+"---\napiVersion: bootwright.io/v1alpha1\nkind: ClusterInfrastructure",
			1,
		)
	}
	return body
}

func managedProxyYAML(name, providerName, cidr, gateway, apiVIP, ingressVIP, nodeIP string) string {
	body := strings.Replace(
		validStateYAML(name, providerName, cidr, apiVIP, ingressVIP, nodeIP),
		"  ocpInstallType: connected\n",
		`  ocpInstallType: connected
  proxy:
    auth:
      proxyAuthRef:
        name: proxy-credentials
`,
		1,
	)
	body = strings.Replace(body, "    registry-lab-ca:\n", "    proxy-credentials:\n      generated:\n        credentials:\n          username: proxy\n    registry-lab-ca:\n", 1)
	body = strings.Replace(body, "        - libvirt\n", "        - libvirt\n        - proxy\n", 1)
	body = strings.Replace(body, fmt.Sprintf("      cidr: %s\n", cidr), fmt.Sprintf("      cidr: %s\n      gateway: %s\n", cidr, gateway), 1)
	body = strings.Replace(body, "      bmcEmulation: {}\n", "      bmcEmulation: {}\n  proxy:\n    squid:\n      hostRef:\n        name: host-01\n", 1)
	return body
}

func TestValidationRejectsDisconnectedWithoutMirrorRegistry(t *testing.T) {
	dir := t.TempDir()
	body := disconnectedYAML("disco-no-reg", "disco-no-reg-provider", "192.168.158.0/24", "192.168.158.10", "192.168.158.11", "192.168.158.20", false, 0)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected disconnected-without-mirrorRegistry validation error")
	}
	if !strings.Contains(err.Error(), "spec.registry.mirrorRegistry") {
		t.Fatalf("expected mirrorRegistry guidance, got %v", err)
	}
}

func TestValidationAcceptsDisconnectedWithMirrorRegistry(t *testing.T) {
	dir := t.TempDir()
	body := disconnectedYAML("disco-ok", "disco-ok-provider", "192.168.159.0/24", "192.168.159.10", "192.168.159.11", "192.168.159.20", true, 5000)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	state, err := LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	if got := v1alpha1.ProviderMirrorRegistry(state.InfrastructureProviders[0]); got == nil {
		t.Fatalf("expected mirrorRegistry on provider, got nil")
	}
}

func TestValidationRejectsMirrorRegistryWithoutHostCapability(t *testing.T) {
	dir := t.TempDir()
	body := disconnectedYAML("disco-cap", "disco-cap-provider", "192.168.160.0/24", "192.168.160.10", "192.168.160.11", "192.168.160.20", true, 5000)
	body = strings.Replace(body, "        - mirror-registry\n", "", 1)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected mirror-registry capability missing error")
	}
	if !strings.Contains(err.Error(), "lacks capability \"mirror-registry\"") {
		t.Fatalf("expected mirror-registry capability error, got %v", err)
	}
}

func TestValidationRejectsMirrorRegistryPortMismatch(t *testing.T) {
	dir := t.TempDir()
	body := disconnectedYAML("disco-port", "disco-port-provider", "192.168.161.0/24", "192.168.161.10", "192.168.161.11", "192.168.161.20", true, 6000)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected port mismatch error")
	}
	if !strings.Contains(err.Error(), "does not match Environment spec.registries.mirror.url port") {
		t.Fatalf("expected port mismatch error, got %v", err)
	}
}

func TestValidationAcceptsManagedProxyWithCredentialsAndHostCapability(t *testing.T) {
	dir := t.TempDir()
	body := managedProxyYAML("proxy-ok", "proxy-ok-provider", "192.168.162.0/24", "192.168.162.1", "192.168.162.10", "192.168.162.11", "192.168.162.20")
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	state, err := LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	squid := v1alpha1.ProviderProxySquid(state.InfrastructureProviders[0])
	if squid == nil {
		t.Fatalf("expected managed Squid proxy on provider")
	}
	if got, want := squid.Port, v1alpha1.DefaultSquidPort; got != want {
		t.Fatalf("default Squid port got %d, want %d", got, want)
	}
	if got, want := squid.Runtime, v1alpha1.ContainerRuntimePodman; got != want {
		t.Fatalf("default Squid runtime got %q, want %q", got, want)
	}
}

func TestValidationRejectsManagedProxyWithoutCredentialsRef(t *testing.T) {
	dir := t.TempDir()
	body := managedProxyYAML("proxy-no-creds", "proxy-no-creds-provider", "192.168.163.0/24", "192.168.163.1", "192.168.163.10", "192.168.163.11", "192.168.163.20")
	body = strings.Replace(body, "    auth:\n      proxyAuthRef:\n        name: proxy-credentials\n", "", 1)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected proxyAuthRef validation error")
	}
	if !strings.Contains(err.Error(), "spec.proxy.auth.proxyAuthRef.name") {
		t.Fatalf("expected managed proxy proxyAuthRef error, got %v", err)
	}
}

func TestValidationRejectsDuplicateManagedProxyProviders(t *testing.T) {
	dir := t.TempDir()
	body := managedProxyYAML("proxy-dupe", "proxy-dupe-provider", "192.168.164.0/24", "192.168.164.1", "192.168.164.10", "192.168.164.11", "192.168.164.20")
	body = strings.Replace(body, "---\napiVersion: bootwright.io/v1alpha1\nkind: ClusterInfrastructure", `---
apiVersion: bootwright.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: extra-proxy-provider
spec:
  hosts:
    proxy-02:
      ssh:
        address: 10.0.0.2
        user: bootwright
        keyRef:
          name: default-key
      capabilities:
        - proxy
  proxy:
    squid:
      hostRef:
        name: proxy-02
---
apiVersion: bootwright.io/v1alpha1
kind: ClusterInfrastructure`, 1)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected duplicate managed proxy provider validation error")
	}
	if !strings.Contains(err.Error(), "exactly one provider supplying spec.proxy.squid") {
		t.Fatalf("expected duplicate proxy provider error, got %v", err)
	}
}

func TestValidationRejectsManagedProxyUnsafeLibvirtPlacement(t *testing.T) {
	dir := t.TempDir()
	body := managedProxyYAML("proxy-placement", "proxy-placement-provider", "192.168.165.0/24", "192.168.165.1", "192.168.165.10", "192.168.165.11", "192.168.165.20")
	body = strings.Replace(body, "    host-01:\n      ssh:", "    host-02:\n      ssh:\n        address: 10.0.0.2\n        user: bootwright\n        keyRef:\n          name: default-key\n      capabilities:\n        - libvirt\n    host-01:\n      ssh:", 1)
	body = strings.Replace(body, "      hostRefs:\n        - name: host-01", "      hostRefs:\n        - name: host-01\n        - name: host-02", 1)
	body = strings.Replace(body, "      libvirt:\n        hostRef:\n          name: host-01", "      libvirt:\n        hostRef:\n          name: host-02", 1)
	writeFile(t, filepath.Join(dir, "case.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected unsafe placement validation error")
	}
	if !strings.Contains(err.Error(), "requires same libvirt/provider host placement") {
		t.Fatalf("expected unsafe placement error, got %v", err)
	}
}

func secretsFixtureYAML(secretsBlock string) string {
	secretsBlock = strings.Replace(secretsBlock, "  secrets:\n", `  secrets:
    openshift-pull-secret:
      file: ./pull-secret
    cluster-admin-key:
      file: ./ssh-key.pub
`, 1)
	return `apiVersion: bootwright.io/v1alpha1
kind: Environment
metadata:
  name: secrets-env
spec:
  baseDomain: example.com
  ocpInstallType: connected
` + secretsBlock
}

func TestValidationRejectsSecretWithBothFileAndGenerated(t *testing.T) {
	dir := t.TempDir()
	body := secretsFixtureYAML(`  secrets:
    bad:
      file: ~/example.txt
      generated:
        credentials:
          username: admin
`)
	writeFile(t, filepath.Join(dir, "env.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected dual-source rejection")
	}
	if !strings.Contains(err.Error(), "pick exactly one source") {
		t.Fatalf("expected dual-source error, got %v", err)
	}
}

func TestValidationRejectsGeneratedSecretWithBothCredentialsAndCert(t *testing.T) {
	dir := t.TempDir()
	body := secretsFixtureYAML(`  secrets:
    bad:
      generated:
        credentials:
          username: admin
        selfSignedCertificate:
          commonName: example.test
`)
	writeFile(t, filepath.Join(dir, "env.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected generated-source dual-kind rejection")
	}
	if !strings.Contains(err.Error(), "pick exactly one") {
		t.Fatalf("expected generated dual-kind error, got %v", err)
	}
}

func TestValidationRejectsGeneratedSecretSelfSignedWithoutCommonName(t *testing.T) {
	dir := t.TempDir()
	body := secretsFixtureYAML(`  secrets:
    bad:
      generated:
        selfSignedCertificate: {}
`)
	writeFile(t, filepath.Join(dir, "env.yaml"), body)
	_, err := LoadNormalizeValidate([]string{dir})
	if err == nil {
		t.Fatal("expected commonName rejection")
	}
	if !strings.Contains(err.Error(), "commonName is required") {
		t.Fatalf("expected commonName error, got %v", err)
	}
}

func TestNormalizeDefaultsGeneratedSecretValidityAndUsername(t *testing.T) {
	dir := t.TempDir()
	body := secretsFixtureYAML(`  secrets:
    cred:
      generated:
        credentials: {}
    cert:
      generated:
        selfSignedCertificate:
          commonName: example.test
`)
	writeFile(t, filepath.Join(dir, "env.yaml"), body)
	state, err := LoadNormalizeValidate([]string{dir})
	if err != nil {
		t.Fatalf("LoadNormalizeValidate returned error: %v", err)
	}
	if len(state.Environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(state.Environments))
	}
	secrets := state.Environments[0].Spec.Secrets
	if got := secrets["cred"].Generated.Credentials.Username; got != "admin" {
		t.Fatalf("credentials.username got %q, want admin (default)", got)
	}
	if got := secrets["cert"].Generated.SelfSignedCertificate.ValidityDays; got != v1alpha1.DefaultCertificateDays {
		t.Fatalf("selfSignedCertificate.validityDays got %d, want %d (default)", got, v1alpha1.DefaultCertificateDays)
	}
}
