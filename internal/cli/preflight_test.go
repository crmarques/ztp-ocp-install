package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type fakeFileInfo struct {
	name  string
	isDir bool
	mode  os.FileMode
}

func (f fakeFileInfo) Name() string { return f.name }
func (f fakeFileInfo) Size() int64  { return 0 }
func (f fakeFileInfo) Mode() os.FileMode {
	if f.mode != 0 {
		return f.mode
	}
	if f.isDir {
		return os.ModeDir | 0o700
	}
	return 0o600
}
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func fakeDeps(present map[string]string, kvmExists bool) preflightDeps {
	return fakeDepsWithStat(present, kvmExists, nil)
}

func fakeDepsWithStat(present map[string]string, kvmExists bool, paths map[string]bool) preflightDeps {
	return fakeDepsWithListen(present, kvmExists, paths, nil)
}

func fakeDepsWithListen(present map[string]string, kvmExists bool, paths map[string]bool, busyAddrs map[string]bool) preflightDeps {
	return fakeDepsWithUnits(present, kvmExists, paths, busyAddrs, nil)
}

func fakeDepsWithUnits(present map[string]string, kvmExists bool, paths map[string]bool, busyAddrs map[string]bool, activeUnits map[string]bool) preflightDeps {
	return preflightDeps{
		lookPath: func(name string, extraDirs []string) (string, error) {
			if path, ok := present[name]; ok {
				return path, nil
			}
			return "", exec.ErrNotFound
		},
		statPath: func(path string) (os.FileInfo, error) {
			if path == "/dev/kvm" && kvmExists {
				return fakeFileInfo{name: "kvm"}, nil
			}
			if isDir, ok := paths[path]; ok {
				return fakeFileInfo{name: path, isDir: isDir}, nil
			}
			return nil, errors.New("not found")
		},
		tryListen: func(_ string, address string) error {
			if busyAddrs[address] {
				return errors.New("address already in use")
			}
			return nil
		},
		unitActive: func(unit string) bool { return activeUnits[unit] },
	}
}

func TestPreflightUniversalOnlyWithoutInputs(t *testing.T) {
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(v1alpha1.State{}, nil, false, defaultSecretsDir(), defaultHostStateDir, deps)
	if len(checks) != 2 {
		t.Fatalf("expected only universal checks, got %d: %+v", len(checks), checks)
	}
	for _, c := range checks {
		if !c.ok {
			t.Fatalf("universal check failed unexpectedly: %+v", c)
		}
	}
}

func TestPreflightHubPhaseDemandsOpenShiftCLIs(t *testing.T) {
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	hub, err := selectPhases("clusters")
	if err != nil {
		t.Fatalf("selectPhases hub: %v", err)
	}
	checks := collectPreflightChecks(v1alpha1.State{}, hub, false, defaultSecretsDir(), defaultHostStateDir, deps)
	want := map[string]bool{
		"openshift-install on PATH": false,
		"oc on PATH":                false,
		"kubectl on PATH":           false,
	}
	for _, c := range checks {
		if _, tracked := want[c.name]; tracked {
			want[c.name] = true
			if c.ok {
				t.Fatalf("expected %s to fail when missing, got ok", c.name)
			}
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing expected check %q", name)
		}
	}
}

func TestPreflightClusterPhaseChecksKvmWhenQemuKvmProvider(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{}}}},
		},
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	cluster, err := selectPhases("cluster")
	if err != nil {
		t.Fatalf("selectPhases cluster: %v", err)
	}
	checks := collectPreflightChecks(state, cluster, true, defaultSecretsDir(), defaultHostStateDir, deps)
	var found bool
	for _, c := range checks {
		if c.name == "/dev/kvm available" {
			found = true
			if c.ok {
				t.Fatalf("expected /dev/kvm check to fail when missing")
			}
		}
	}
	if !found {
		t.Fatalf("expected /dev/kvm check for libvirt provider in cluster phase, got: %+v", checks)
	}
}

func TestPreflightClusterPhaseSkipsKvmWithoutQemuKvm(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{BareMetal: &v1alpha1.MachineProviderBareMetalSpec{}}}},
		},
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	for _, c := range checks {
		if c.name == "/dev/kvm available" {
			t.Fatalf("did not expect /dev/kvm check for non-qemu-kvm state")
		}
	}
}

func TestPreflightProviderOnlyPhaseSkipsKvm(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{}}}},
		},
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	provider, err := selectPhases("provider")
	if err != nil {
		t.Fatalf("selectPhases provider: %v", err)
	}
	checks := collectPreflightChecks(state, provider, true, defaultSecretsDir(), defaultHostStateDir, deps)
	for _, c := range checks {
		if c.name == "/dev/kvm available" {
			t.Fatalf("provider-only phase must not demand /dev/kvm: %+v", c)
		}
	}
}

func TestPreflightClusterPhaseSkipsKvmForRemoteLibvirtHost(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{
				Spec: v1alpha1.InfrastructureProviderSpec{
					Hosts: map[string]v1alpha1.ProviderHostSpec{
						"remote-host": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "192.168.1.10"}},
					},
					Machine: &v1alpha1.MachineCapabilitySpec{
						Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
							HostRefs: []v1alpha1.LocalObjectReference{{Name: "remote-host"}},
						},
					},
				},
			},
		},
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
	}, false)
	cluster, err := selectPhases("cluster")
	if err != nil {
		t.Fatalf("selectPhases cluster: %v", err)
	}
	checks := collectPreflightChecks(state, cluster, true, defaultSecretsDir(), defaultHostStateDir, deps)
	for _, c := range checks {
		if c.name == "/dev/kvm available" {
			t.Fatalf("cluster phase must not check /dev/kvm when all libvirt hosts are remote: %+v", c)
		}
	}
}

func TestPreflightFailsWhenAnsibleMissing(t *testing.T) {
	deps := fakeDeps(map[string]string{
		"python3": "/usr/bin/python3",
		"sudo":    "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(v1alpha1.State{}, nil, false, defaultSecretsDir(), defaultHostStateDir, deps)
	var ansibleCheck *preflightCheck
	for i := range checks {
		if checks[i].name == "ansible-playbook on PATH" {
			ansibleCheck = &checks[i]
		}
	}
	if ansibleCheck == nil || ansibleCheck.ok {
		t.Fatalf("expected ansible-playbook check to fail, got %+v", ansibleCheck)
	}
}

func TestPreflightOCPChecksSecretsDirAndFiles(t *testing.T) {
	state := v1alpha1.State{
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef:            v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:                v1alpha1.SecretRef{Name: "ssh-key"},
						AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
					},
				},
			},
			{
				Metadata: v1alpha1.Metadata{Name: "second"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef: v1alpha1.SecretRef{Name: "second-pull"},
					},
				},
			},
		},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook":  "/usr/bin/ansible-playbook",
		"python3":           "/usr/bin/python3",
		"sudo":              "/usr/bin/sudo",
		"openshift-install": "/usr/local/bin/openshift-install",
		"oc":                "/usr/local/bin/oc",
		"kubectl":           "/usr/local/bin/kubectl",
	}, false, map[string]bool{
		"/secrets":             true,
		"/secrets/pull-secret": false,
		"/secrets/ssh-key":     false,
		"/secrets/second-pull": false,
	})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	want := map[string]bool{
		"hub pullSecretRef at /secrets/pull-secret":      true,
		"hub sshKeyRef at /secrets/ssh-key":              true,
		"hub additionalTrustBundleRef at /secrets/trust": false,
		"second pullSecretRef at /secrets/second-pull":   true,
	}
	seen := map[string]bool{}
	for _, c := range checks {
		if _, tracked := want[c.name]; !tracked {
			continue
		}
		seen[c.name] = true
		if c.ok != want[c.name] {
			t.Fatalf("check %q: ok=%v want=%v (%s)", c.name, c.ok, want[c.name], c.detail)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
	for _, c := range checks {
		if strings.HasPrefix(c.name, "secrets directory") {
			t.Fatalf("file-based reqs should not require secrets directory check: %+v", c)
		}
	}
}

func TestPreflightHubChecksDisconnectedRegistryCredentials(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "lab"},
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Disconnected: &v1alpha1.DisconnectedSpec{
						Registries: &v1alpha1.OCPInstallRegistries{
							Mirror: &v1alpha1.OCPInstallRegistryMirror{
								URL:            "registry.lab.test:5000",
								CredentialsRef: v1alpha1.SecretRef{Name: "registry-lab-credentials"},
							},
						},
					},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef: v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:     v1alpha1.SecretRef{Name: "ssh-key"},
					},
				},
			},
		},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook":  "/usr/bin/ansible-playbook",
		"python3":           "/usr/bin/python3",
		"sudo":              "/usr/bin/sudo",
		"openshift-install": "/usr/local/bin/openshift-install",
		"oc":                "/usr/local/bin/oc",
		"kubectl":           "/usr/local/bin/kubectl",
	}, false, map[string]bool{
		"/secrets":             true,
		"/secrets/pull-secret": false,
		"/secrets/ssh-key":     false,
	})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	const want = "registry mirror credentialsRef at /secrets/registry-lab-credentials"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing expected check %q in %+v", want, checks)
	}
	if found.ok {
		t.Fatalf("expected %q to fail when missing, got ok", want)
	}
	if !strings.Contains(found.detail, "gitups secrets credentials set --name registry-lab-credentials") {
		t.Fatalf("expected hint pointing at `gitups secrets credentials set`, got: %s", found.detail)
	}
}

func TestPreflightHubGeneratedTrustBundleChecksOpenSSL(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Keys: map[string]v1alpha1.EnvironmentKeySpec{
					"trust": {Generated: &v1alpha1.EnvironmentKeyGenerated{
						SelfSignedCertificate: &v1alpha1.SelfSignedCertificateSpec{CommonName: "registry.lab.test"},
					}},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef:            v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:                v1alpha1.SecretRef{Name: "ssh-key"},
						AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
					},
				},
			},
		},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook":  "/usr/bin/ansible-playbook",
		"python3":           "/usr/bin/python3",
		"sudo":              "/usr/bin/sudo",
		"openshift-install": "/usr/local/bin/openshift-install",
		"oc":                "/usr/local/bin/oc",
		"kubectl":           "/usr/local/bin/kubectl",
		"openssl":           "/usr/bin/openssl",
	}, false, map[string]bool{
		"/secrets":             true,
		"/secrets/pull-secret": false,
		"/secrets/ssh-key":     false,
	})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	want := map[string]bool{
		"openssl on PATH": true,
		"hub additionalTrustBundleRef at /secrets/trust": false,
	}
	seen := map[string]bool{}
	for _, c := range checks {
		if _, tracked := want[c.name]; !tracked {
			continue
		}
		seen[c.name] = true
		if c.ok != want[c.name] {
			t.Fatalf("check %q: ok=%v want=%v (%s)", c.name, c.ok, want[c.name], c.detail)
		}
		if c.name == "hub additionalTrustBundleRef at /secrets/trust" && !strings.Contains(c.detail, "gitups secrets generate") {
			t.Fatalf("expected generated-secret guidance, got %+v", c)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
}

func TestPreflightFailsWhenSelfSignedCertOnDiskDoesNotMatchSpec(t *testing.T) {
	secretsDir := t.TempDir()
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Keys: map[string]v1alpha1.EnvironmentKeySpec{
					"trust": {Generated: &v1alpha1.EnvironmentKeyGenerated{
						SelfSignedCertificate: &v1alpha1.SelfSignedCertificateSpec{
							CommonName:   "registry.lab.test",
							ValidityDays: v1alpha1.DefaultCertificateDays,
						},
					}},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef:            v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:                v1alpha1.SecretRef{Name: "ssh-key"},
						AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
					},
				},
			},
		},
	}
	staleRequest := generatedSelfSignedRequest{
		name: "trust",
		certificate: v1alpha1.SelfSignedCertificateSpec{
			CommonName:   "registry.other.test",
			ValidityDays: v1alpha1.DefaultCertificateDays,
		},
	}
	if _, err := materializeSelfSignedCertificate(secretsDir, staleRequest); err != nil {
		t.Fatalf("seed stale cert: %v", err)
	}
	checks := generatedSelfSignedDriftChecks(state, secretsDir)
	if len(checks) == 0 {
		t.Fatalf("expected drift check, got none")
	}
	var found *preflightCheck
	for i, c := range checks {
		if strings.Contains(c.name, "trust") {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing drift check for trust: %+v", checks)
	}
	if found.ok {
		t.Fatalf("expected drift check to fail, got %+v", *found)
	}
	wantPath := filepath.Join(secretsDir, "trust")
	if !strings.Contains(found.detail, wantPath) {
		t.Fatalf("expected detail to mention cert path %s, got %q", wantPath, found.detail)
	}
	if !strings.Contains(found.detail, "gitups secrets generate") {
		t.Fatalf("expected remediation hint, got %q", found.detail)
	}
}

func TestPreflightSelfSignedCertDriftCheckPassesWhenMatching(t *testing.T) {
	secretsDir := t.TempDir()
	cert := v1alpha1.SelfSignedCertificateSpec{
		CommonName:   "registry.lab.test",
		ValidityDays: v1alpha1.DefaultCertificateDays,
	}
	if _, err := materializeSelfSignedCertificate(secretsDir, generatedSelfSignedRequest{name: "trust", certificate: cert}); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Keys: map[string]v1alpha1.EnvironmentKeySpec{
					"trust": {Generated: &v1alpha1.EnvironmentKeyGenerated{
						SelfSignedCertificate: &cert,
					}},
				},
			},
		}},
		OCPClusters: []v1alpha1.OCPCluster{{
			Metadata: v1alpha1.Metadata{Name: "hub"},
			Spec: v1alpha1.OCPClusterSpec{
				Install: v1alpha1.OCPInstallSpec{
					AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
				},
			},
		}},
	}
	checks := generatedSelfSignedDriftChecks(state, secretsDir)
	for _, c := range checks {
		if !c.ok {
			t.Fatalf("unexpected failing check: %+v", c)
		}
	}
}

func TestPreflightProviderPhaseChecksBMCPortsAvailable(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}}},
		}},
	}
	deps := fakeDepsWithListen(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, nil)
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	want := map[string]bool{
		"provider libvirt-1-host-provider redfish port 0.0.0.0:8000 free":             false,
		"provider libvirt-1-host-provider vmedia HTTP port 127.0.0.1:8001 free":       false,
		"provider libvirt-1-host-provider boot-artifacts HTTP port 0.0.0.0:8002 free": false,
	}
	for _, c := range checks {
		if _, tracked := want[c.name]; !tracked {
			continue
		}
		want[c.name] = true
		if !c.ok {
			t.Fatalf("expected %q to pass when listener is free, got fail (%s)", c.name, c.detail)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
}

func TestPreflightProviderPhaseFailsWhenBMCPortInUse(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}}},
		}},
	}
	deps := fakeDepsWithListen(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, map[string]bool{
		"127.0.0.1:8001": true,
	})
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	const want = "provider libvirt-1-host-provider vmedia HTTP port 127.0.0.1:8001 free"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing expected check %q in %+v", want, checks)
	}
	if found.ok {
		t.Fatalf("expected %q to fail when port is busy, got ok", want)
	}
	if !strings.Contains(found.detail, "stale gitups-sushy") {
		t.Fatalf("expected hint about stale unit, got: %s", found.detail)
	}
}

func TestPreflightProviderPhaseAllowsPortHeldByMatchingGitupsUnit(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}}},
		}},
	}
	deps := fakeDepsWithUnits(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, map[string]bool{
		"0.0.0.0:8000":   true,
		"127.0.0.1:8001": true,
		"0.0.0.0:8002":   true,
	}, map[string]bool{
		"gitups-sushy-libvirt-1-host-provider.service":          true,
		"gitups-vmedia-libvirt-1-host-provider.service":         true,
		"gitups-boot-artifacts-libvirt-1-host-provider.service": true,
	})
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	want := map[string]string{
		"provider libvirt-1-host-provider redfish port 0.0.0.0:8000 free":             "gitups-sushy-libvirt-1-host-provider.service",
		"provider libvirt-1-host-provider vmedia HTTP port 127.0.0.1:8001 free":       "gitups-vmedia-libvirt-1-host-provider.service",
		"provider libvirt-1-host-provider boot-artifacts HTTP port 0.0.0.0:8002 free": "gitups-boot-artifacts-libvirt-1-host-provider.service",
	}
	seen := map[string]bool{}
	for _, c := range checks {
		expected, tracked := want[c.name]
		if !tracked {
			continue
		}
		seen[c.name] = true
		if !c.ok {
			t.Fatalf("expected %q to pass when held by matching gitups unit, got fail (%s)", c.name, c.detail)
		}
		if !strings.Contains(c.detail, expected) {
			t.Fatalf("expected detail to mention %s, got %q", expected, c.detail)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
}

func TestPreflightProviderPhaseFailsWhenPortHeldByForeignUnit(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}}},
		}},
	}
	deps := fakeDepsWithUnits(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, map[string]bool{
		"127.0.0.1:8001": true,
	}, map[string]bool{
		"gitups-vmedia-some-other-provider.service": true,
	})
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	const want = "provider libvirt-1-host-provider vmedia HTTP port 127.0.0.1:8001 free"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil || found.ok {
		t.Fatalf("expected %q to fail when held by unrelated unit, got %+v", want, found)
	}
}

func TestPreflightProviderPhaseSkipsBMCPortsWhenLibvirtRemote(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-remote-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"remote-libvirt-host": {SSH: &v1alpha1.ProviderHostSSHSpec{}},
				},
				Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
					HostRefs: []v1alpha1.LocalObjectReference{{Name: "remote-libvirt-host"}},
					BMCEmulation: &v1alpha1.BMCEmulationSpec{
						Enabled:     v1alpha1.BoolPtr(true),
						BindAddress: "0.0.0.0",
						Port:        8000,
					},
				}},
			},
		}},
	}
	deps := fakeDepsWithListen(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, map[string]bool{
		"0.0.0.0:8000":   true,
		"127.0.0.1:8001": true,
		"0.0.0.0:8002":   true,
	})
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	for _, c := range checks {
		if strings.HasPrefix(c.name, "provider libvirt-remote-provider ") && strings.Contains(c.name, " port ") {
			t.Fatalf("BMC port check should be skipped when libvirt runs on a remote SSH host: %+v", c)
		}
	}
}

func TestPreflightProviderPhaseSkipsBMCPortsWhenEmulationDisabled(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "qemu-no-bmc"},
			Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled: v1alpha1.BoolPtr(false),
					Port:    8000,
				},
			}}},
		}},
	}
	deps := fakeDepsWithListen(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, nil)
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	for _, c := range checks {
		if strings.HasPrefix(c.name, "provider qemu-no-bmc ") {
			t.Fatalf("BMC port check should be skipped when emulation disabled: %+v", c)
		}
	}
}

func TestPreflightSecretsDirSkippedWithoutHubPhase(t *testing.T) {
	state := v1alpha1.State{
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef: v1alpha1.SecretRef{Name: "pull-secret"},
					},
				},
			},
		},
	}
	provider, err := selectPhases("provider")
	if err != nil {
		t.Fatalf("selectPhases provider: %v", err)
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(state, provider, true, "/secrets", defaultHostStateDir, deps)
	for _, c := range checks {
		if strings.HasPrefix(c.name, "secrets directory") || strings.Contains(c.name, "pullSecretRef") {
			t.Fatalf("secrets check should be scoped to clusters phase, got %+v", c)
		}
	}
}

func TestPreflightChecksProxyCredentialsRef(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "lab"},
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Restricted: &v1alpha1.RestrictedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{
							HTTPProxy:      "http://proxy.lab.test:3128",
							CredentialsRef: v1alpha1.SecretRef{Name: "proxy-credentials"},
						},
					},
				},
			},
		}},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false, map[string]bool{"/secrets": true})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	const want = "ocpInstall proxy credentialsRef at /secrets/proxy-credentials"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing expected proxy credentialsRef check %q in %+v", want, checks)
	}
	if found.ok {
		t.Fatalf("expected %q to fail when missing, got ok", want)
	}
	if !strings.Contains(found.detail, "gitups secrets credentials set --name proxy-credentials") {
		t.Fatalf("expected hint pointing at `gitups secrets credentials set`, got: %s", found.detail)
	}
}

func TestPreflightProxyCredentialsScopedAwayFromHubOnlyRun(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "lab"},
			Spec: v1alpha1.EnvironmentSpec{
				OCPInstall: v1alpha1.EnvironmentOCPInstallSpec{
					Restricted: &v1alpha1.RestrictedSpec{
						Proxy: &v1alpha1.OCPInstallProxy{
							HTTPProxy:      "http://proxy.lab.test:3128",
							CredentialsRef: v1alpha1.SecretRef{Name: "proxy-credentials"},
						},
					},
				},
			},
		}},
	}
	hub, err := selectPhases("clusters")
	if err != nil {
		t.Fatalf("selectPhases hub: %v", err)
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook":  "/usr/bin/ansible-playbook",
		"python3":           "/usr/bin/python3",
		"sudo":              "/usr/bin/sudo",
		"openshift-install": "/usr/local/bin/openshift-install",
		"oc":                "/usr/local/bin/oc",
		"kubectl":           "/usr/local/bin/kubectl",
	}, false)
	checks := collectPreflightChecks(state, hub, true, "/secrets", defaultHostStateDir, deps)
	for _, c := range checks {
		if strings.Contains(c.name, "ocpInstall proxy credentialsRef") {
			t.Fatalf("proxy credentialsRef must not surface in clusters-only run: %+v", c)
		}
	}

	cluster, err := selectPhases("cluster")
	if err != nil {
		t.Fatalf("selectPhases cluster: %v", err)
	}
	checks = collectPreflightChecks(state, cluster, true, "/secrets", defaultHostStateDir, deps)
	var found bool
	for _, c := range checks {
		if strings.Contains(c.name, "ocpInstall proxy credentialsRef") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("proxy credentialsRef must surface in cluster-only run; host_proxy runs there")
	}
}

func TestPreflightChecksSSHKeyRefAtSourcePathWhenFileBased(t *testing.T) {
	state := v1alpha1.State{
		Environments: []v1alpha1.Environment{{
			Metadata: v1alpha1.Metadata{Name: "env"},
			Spec: v1alpha1.EnvironmentSpec{
				Keys: map[string]v1alpha1.EnvironmentKeySpec{
					"my-host-key": {File: "/tmp/foo"},
				},
			},
		}},
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "p"},
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"h": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "10.0.0.1", KeyRef: v1alpha1.SecretRef{Name: "my-host-key"}}},
				},
				Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
					HostRefs: []v1alpha1.LocalObjectReference{{Name: "h"}},
				}},
			},
		}},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil)
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	const want = "provider p host h sshKeyRef at /tmp/foo"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing expected ssh-key check at file-based path %q in %+v", want, checks)
	}
	if found.ok {
		t.Fatalf("expected %q to fail when /tmp/foo missing, got ok", want)
	}
	if !strings.Contains(found.detail, "Environment.spec.keys[my-host-key].file") {
		t.Fatalf("expected hint to point at Environment.spec.keys, got: %s", found.detail)
	}
}

func TestPreflightChecksProviderHostSSHKeyRef(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"local-qemu-host": {
						SSH: &v1alpha1.ProviderHostSSHSpec{
							Address: "localhost",
							KeyRef:  v1alpha1.SecretRef{Name: "local-qemu-host-admin-key"},
						},
					},
				},
				Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
					HostRefs: []v1alpha1.LocalObjectReference{{Name: "local-qemu-host"}},
				}},
			},
		}},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, map[string]bool{"/secrets": true})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	const want = "provider libvirt-1-host-provider host local-qemu-host sshKeyRef at /secrets/local-qemu-host-admin-key"
	var found *preflightCheck
	for i := range checks {
		if checks[i].name == want {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing expected host sshKeyRef check %q in %+v", want, checks)
	}
	if found.ok {
		t.Fatalf("expected %q to fail when missing, got ok", want)
	}
}

func TestPreflightChecksBMCEmulationAndMachineCredentials(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "libvirt-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{
				Hosts: map[string]v1alpha1.ProviderHostSpec{
					"local-qemu-host": {SSH: &v1alpha1.ProviderHostSSHSpec{Address: "localhost", KeyRef: v1alpha1.SecretRef{Name: "host-key"}}},
				},
				Machine: &v1alpha1.MachineCapabilitySpec{Libvirt: &v1alpha1.MachineProviderLibvirtSpec{
					HostRefs: []v1alpha1.LocalObjectReference{{Name: "local-qemu-host"}},
					BMCEmulation: &v1alpha1.BMCEmulationSpec{
						Enabled: v1alpha1.BoolPtr(false),
						Auth: &v1alpha1.BMCAuthSpec{
							CredentialRef: v1alpha1.SecretRef{Name: "qemu-1-host-bmc-credentials"},
						},
					},
				}},
			},
		}},
		ClusterInfrastructures: []v1alpha1.ClusterInfrastructure{{
			Metadata: v1alpha1.Metadata{Name: "ci"},
			Spec: v1alpha1.ClusterInfrastructureSpec{
				ProviderRefs: []v1alpha1.LocalObjectReference{{Name: "libvirt-1-host-provider"}},
				Machines: map[string]v1alpha1.MachineSpec{
					"master-0": {
						BareMetal: &v1alpha1.MachineBareMetalSpec{BMC: &v1alpha1.MachineBMCSpec{
							Address:       "redfish://10.0.0.5",
							CredentialRef: v1alpha1.SecretRef{Name: "rack-bmc-credentials"},
						}},
					},
				},
			},
		}},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, map[string]bool{"/secrets": true})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	want := map[string]bool{
		"provider libvirt-1-host-provider bmcEmulation credentialRef at /secrets/qemu-1-host-bmc-credentials": false,
		"infra ci machine master-0 baremetal bmc credentialRef at /secrets/rack-bmc-credentials":              false,
	}
	seen := map[string]bool{}
	for _, c := range checks {
		if _, tracked := want[c.name]; !tracked {
			continue
		}
		seen[c.name] = true
		if c.ok {
			t.Fatalf("expected %q to fail when missing, got ok", c.name)
		}
		if !strings.Contains(c.detail, "gitups secrets credentials set") {
			t.Fatalf("expected BMC writer hint for %q, got: %s", c.name, c.detail)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
}

func TestPreflightChecksVMwareAndOSVProviderRefs(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{
				Metadata: v1alpha1.Metadata{Name: "vmw"},
				Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{VSphere: &v1alpha1.MachineProviderVSphereSpec{
					VCenterRef: v1alpha1.SecretRef{Name: "lab-vcenter"},
					Datacenter: "dc",
					Cluster:    "cl",
				}}},
			},
			{
				Metadata: v1alpha1.Metadata{Name: "osv"},
				Spec: v1alpha1.InfrastructureProviderSpec{Machine: &v1alpha1.MachineCapabilitySpec{KubeVirt: &v1alpha1.MachineProviderKubeVirtSpec{
					ClusterRef: v1alpha1.SecretRef{Name: "lab-osv-cluster"},
					Namespace:  "lab",
				}}},
			},
		},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false, map[string]bool{"/secrets": true})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	want := []string{
		"provider vmw vsphere vCenterRef at /secrets/lab-vcenter",
		"provider osv kubevirt clusterRef at /secrets/lab-osv-cluster",
	}
	for _, w := range want {
		var found *preflightCheck
		for i := range checks {
			if checks[i].name == w {
				found = &checks[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("missing expected check %q in %+v", w, checks)
		}
		if found.ok {
			t.Fatalf("expected %q to fail when missing, got ok", w)
		}
	}
}
