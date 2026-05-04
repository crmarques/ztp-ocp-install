package cli

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
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
	}
}

func TestPreflightUniversalOnlyWithoutInputs(t *testing.T) {
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(v1alpha1.State{}, nil, false, defaultSecretsDir(), defaultHostStateDir, deps)
	if len(checks) != 3 {
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
	hub, err := selectPhases("hub")
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

func TestPreflightInfraPhaseChecksKvmWhenQemuKvmProvider(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{}}},
		},
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
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
		t.Fatalf("expected /dev/kvm check for qemu-kvm provider, got: %+v", checks)
	}
}

func TestPreflightInfraPhaseSkipsKvmWithoutQemuKvm(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{
			{Spec: v1alpha1.InfrastructureProviderSpec{BareMetal: &v1alpha1.BareMetalProviderSpec{}}},
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

func TestPreflightHubChecksSecretsDirAndFiles(t *testing.T) {
	state := v1alpha1.State{
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Role: v1alpha1.OCPRoleHub,
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef:            v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:                v1alpha1.SecretRef{Name: "ssh-key"},
						AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
					},
				},
			},
			{
				Metadata: v1alpha1.Metadata{Name: "managed"},
				Spec: v1alpha1.OCPClusterSpec{
					Role: v1alpha1.OCPRoleManaged,
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef: v1alpha1.SecretRef{Name: "managed-pull"},
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
	want := map[string]bool{
		"secrets directory at /secrets":                  true,
		"hub pullSecretRef at /secrets/pull-secret":      true,
		"hub sshKeyRef at /secrets/ssh-key":              true,
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
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
	for _, c := range checks {
		if strings.HasPrefix(c.name, "managed ") {
			t.Fatalf("managed cluster secrets should not be checked at hub phase: %+v", c)
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
					Role: v1alpha1.OCPRoleHub,
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
	if !strings.Contains(found.detail, "gitups secrets bmc set --name registry-lab-credentials") {
		t.Fatalf("expected hint pointing at `gitups secrets bmc set`, got: %s", found.detail)
	}
}

func TestPreflightHubGeneratedTrustBundleChecksOpenSSL(t *testing.T) {
	state := v1alpha1.State{
		OCPClusters: []v1alpha1.OCPCluster{
			{
				Metadata: v1alpha1.Metadata{Name: "hub"},
				Spec: v1alpha1.OCPClusterSpec{
					Role: v1alpha1.OCPRoleHub,
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef:            v1alpha1.SecretRef{Name: "pull-secret"},
						SSHKeyRef:                v1alpha1.SecretRef{Name: "ssh-key"},
						AdditionalTrustBundleRef: v1alpha1.SecretRef{Name: "trust"},
						GeneratedSecrets: []v1alpha1.GeneratedSecretSpec{
							{
								Name: "trust",
								Type: v1alpha1.GeneratedSecretSelfSigned,
							},
						},
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
		"hub additionalTrustBundleRef at /secrets/trust": true,
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
		if c.name == "hub additionalTrustBundleRef at /secrets/trust" && !strings.Contains(c.detail, "will be generated") {
			t.Fatalf("expected generated detail, got %+v", c)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing expected check %q in %+v", name, checks)
		}
	}
}

func TestPreflightInfraPhaseChecksBMCPortsAvailable(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "qemu-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}},
		}},
	}
	deps := fakeDepsWithListen(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, nil, nil)
	checks := collectPreflightChecks(state, nil, true, defaultSecretsDir(), defaultHostStateDir, deps)
	want := map[string]bool{
		"provider qemu-1-host-provider redfish port 0.0.0.0:8000 free":             false,
		"provider qemu-1-host-provider vmedia HTTP port 127.0.0.1:8001 free":       false,
		"provider qemu-1-host-provider boot-artifacts HTTP port 0.0.0.0:8002 free": false,
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

func TestPreflightInfraPhaseFailsWhenBMCPortInUse(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "qemu-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled:     v1alpha1.BoolPtr(true),
					BindAddress: "0.0.0.0",
					Port:        8000,
				},
			}},
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
	const want = "provider qemu-1-host-provider vmedia HTTP port 127.0.0.1:8001 free"
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

func TestPreflightInfraPhaseSkipsBMCPortsWhenEmulationDisabled(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "qemu-no-bmc"},
			Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled: v1alpha1.BoolPtr(false),
					Port:    8000,
				},
			}},
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
					Role: v1alpha1.OCPRoleHub,
					Install: v1alpha1.OCPInstallSpec{
						PullSecretRef: v1alpha1.SecretRef{Name: "pull-secret"},
					},
				},
			},
		},
	}
	infra, err := selectPhases("infra")
	if err != nil {
		t.Fatalf("selectPhases infra: %v", err)
	}
	deps := fakeDeps(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, false)
	checks := collectPreflightChecks(state, infra, true, "/secrets", defaultHostStateDir, deps)
	for _, c := range checks {
		if strings.HasPrefix(c.name, "secrets directory") || strings.Contains(c.name, "pullSecretRef") {
			t.Fatalf("secrets check should be scoped to hub phase, got %+v", c)
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
	if !strings.Contains(found.detail, "gitups secrets bmc set --name proxy-credentials") {
		t.Fatalf("expected hint pointing at `gitups secrets bmc set`, got: %s", found.detail)
	}
}

func TestPreflightProxyCredentialsScopedToInfraPhase(t *testing.T) {
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
	hub, err := selectPhases("hub")
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
			t.Fatalf("proxy credentialsRef should be scoped to infra phase, leaked into hub-only run: %+v", c)
		}
	}
}

func TestPreflightChecksProviderHostSSHKeyRef(t *testing.T) {
	state := v1alpha1.State{
		InfrastructureProviders: []v1alpha1.InfrastructureProvider{{
			Metadata: v1alpha1.Metadata{Name: "qemu-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{
				Hosts: map[string]v1alpha1.QemuKVMHostSpec{
					"local-qemu-host": {
						Address:   "localhost",
						SSHKeyRef: v1alpha1.SecretRef{Name: "local-qemu-host-admin-key"},
					},
				},
			}},
		}},
	}
	deps := fakeDepsWithStat(map[string]string{
		"ansible-playbook": "/usr/bin/ansible-playbook",
		"python3":          "/usr/bin/python3",
		"sudo":             "/usr/bin/sudo",
	}, true, map[string]bool{"/secrets": true})
	checks := collectPreflightChecks(state, nil, true, "/secrets", defaultHostStateDir, deps)
	const want = "provider qemu-1-host-provider host local-qemu-host sshKeyRef at /secrets/local-qemu-host-admin-key"
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
			Metadata: v1alpha1.Metadata{Name: "qemu-1-host-provider"},
			Spec: v1alpha1.InfrastructureProviderSpec{QemuKVM: &v1alpha1.QemuKVMProviderSpec{
				Hosts: map[string]v1alpha1.QemuKVMHostSpec{
					"local-qemu-host": {Address: "localhost", SSHKeyRef: v1alpha1.SecretRef{Name: "host-key"}},
				},
				BMCEmulation: &v1alpha1.BMCEmulationSpec{
					Enabled: v1alpha1.BoolPtr(false),
					Auth: &v1alpha1.BMCAuthSpec{
						CredentialRef: v1alpha1.SecretRef{Name: "qemu-1-host-bmc-credentials"},
					},
				},
			}},
		}},
		ClusterInfrastructures: []v1alpha1.ClusterInfrastructure{{
			Metadata: v1alpha1.Metadata{Name: "ci"},
			Spec: v1alpha1.ClusterInfrastructureSpec{
				ProviderRef: v1alpha1.LocalObjectReference{Name: "qemu-1-host-provider"},
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
		"provider qemu-1-host-provider bmcEmulation credentialRef at /secrets/qemu-1-host-bmc-credentials":     false,
		"infra ci machine master-0 bareMetal bmc credentialRef at /secrets/rack-bmc-credentials": false,
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
		if !strings.Contains(c.detail, "gitups secrets bmc set") {
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
				Spec: v1alpha1.InfrastructureProviderSpec{VMware: &v1alpha1.VMwareProviderSpec{
					VCenterRef: v1alpha1.SecretRef{Name: "lab-vcenter"},
					Datacenter: "dc",
					Cluster:    "cl",
				}},
			},
			{
				Metadata: v1alpha1.Metadata{Name: "osv"},
				Spec: v1alpha1.InfrastructureProviderSpec{OpenShiftVirtualization: &v1alpha1.OpenShiftVirtualizationSpec{
					ClusterRef: v1alpha1.SecretRef{Name: "lab-osv-cluster"},
					Namespace:  "lab",
				}},
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
		"provider vmw vmware vCenterRef at /secrets/lab-vcenter",
		"provider osv openShiftVirtualization clusterRef at /secrets/lab-osv-cluster",
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
