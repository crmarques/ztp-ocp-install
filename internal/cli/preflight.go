package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type preflightCheck struct {
	name   string
	ok     bool
	detail string
}

type preflightDeps struct {
	lookPath  func(name string, extraDirs []string) (string, error)
	statPath  func(path string) (os.FileInfo, error)
	tryListen func(network, address string) error
}

var defaultPreflightDeps = preflightDeps{
	lookPath:  defaultLookPath,
	statPath:  os.Stat,
	tryListen: defaultTryListen,
}

func collectPreflightChecks(state v1alpha1.State, selected []Phase, hasState bool, secretsDir string, hostStateDir string, deps preflightDeps) []preflightCheck {
	checks := []preflightCheck{
		binaryCheck("ansible-playbook", nil, deps),
		binaryCheck("python3", nil, deps),
		binaryCheck("sudo", nil, deps),
	}
	if phaseInScope("infra", selected, hasState) && stateNeedsQemuKvm(state) {
		checks = append(checks, kvmCheck(deps))
		checks = append(checks, bmcPortChecks(state, deps)...)
	}
	if phaseInScope("hub", selected, hasState) {
		checks = append(checks,
			binaryCheck("openshift-install", openshiftInstallSearchDirs(hostStateDir), deps),
			binaryCheck("oc", nil, deps),
			binaryCheck("kubectl", nil, deps),
		)
		if hasState && hubNeedsOpenSSL(state, secretsDir, deps) {
			checks = append(checks, binaryCheck("openssl", nil, deps))
		}
	}
	if phaseInScope("gitops-publish", selected, hasState) {
		checks = append(checks, binaryCheck("git", nil, deps))
	}
	if hasState {
		checks = append(checks, secretRefChecks(state, secretsDir, selected, deps)...)
	}
	return checks
}

// phaseInScope: with --phase, only that phase counts; without --phase, every
// phase is in scope iff the user supplied desired-state input. With neither,
// only universal checks run.
func phaseInScope(name string, selected []Phase, hasState bool) bool {
	if len(selected) == 0 {
		return hasState
	}
	for _, p := range selected {
		if p.Name == name {
			return true
		}
	}
	return false
}

func stateNeedsQemuKvm(state v1alpha1.State) bool {
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.MachineFlavor(p) == v1alpha1.MachineFlavorLibvirt {
			return true
		}
	}
	return false
}

func binaryCheck(name string, extraDirs []string, deps preflightDeps) preflightCheck {
	path, err := deps.lookPath(name, extraDirs)
	if err != nil {
		return preflightCheck{name: name + " on PATH", ok: false, detail: "not found"}
	}
	return preflightCheck{name: name + " on PATH", ok: true, detail: path}
}

func kvmCheck(deps preflightDeps) preflightCheck {
	if _, err := deps.statPath("/dev/kvm"); err != nil {
		return preflightCheck{name: "/dev/kvm available", ok: false, detail: "missing — qemu-kvm provider requires KVM hardware support"}
	}
	return preflightCheck{name: "/dev/kvm available", ok: true}
}

// bmcPortChecks probes the three TCP ports each enabled qemu-kvm BMC emulator
// will bind under apply: redfish on the user's bindAddress, vmedia HTTP on
// 127.0.0.1, and boot-artifacts HTTP on 0.0.0.0. We do an in-the-moment
// `net.Listen` rather than relying on cross-provider static analysis because
// stale `gitups-sushy-*`/`gitups-vmedia-*`/`gitups-boot-artifacts-*` units
// from a prior provider name (no longer in the state file) are the common
// real-world cause of the wait-tasks hanging.
func bmcPortChecks(state v1alpha1.State, deps preflightDeps) []preflightCheck {
	var checks []preflightCheck
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.ProviderMachineLibvirt(p) == nil || p.Spec.Machine.Libvirt.BMCEmulation == nil {
			continue
		}
		bmc := p.Spec.Machine.Libvirt.BMCEmulation
		if bmc.Enabled == nil || !*bmc.Enabled {
			continue
		}
		bind := bmc.BindAddress
		if bind == "" {
			bind = v1alpha1.DefaultBMCBindAddress
		}
		probes := []struct {
			label string
			addr  string
			port  int
		}{
			{"redfish", bind, bmc.Port},
			{"vmedia HTTP", "127.0.0.1", bmc.Port + 1},
			{"boot-artifacts HTTP", "0.0.0.0", bmc.Port + 2},
		}
		for _, item := range probes {
			checks = append(checks, bmcPortCheck(p.Metadata.Name, item.label, item.addr, item.port, deps))
		}
	}
	return checks
}

func bmcPortCheck(providerName, label, bindAddr string, port int, deps preflightDeps) preflightCheck {
	addr := net.JoinHostPort(bindAddr, strconv.Itoa(port))
	name := fmt.Sprintf("provider %s %s port %s free", providerName, label, addr)
	if err := deps.tryListen("tcp", addr); err != nil {
		return preflightCheck{
			name:   name,
			ok:     false,
			detail: fmt.Sprintf("cannot bind: %v — stop whatever is holding the port (e.g. a stale gitups-sushy-/gitups-vmedia-/gitups-boot-artifacts-* unit)", err),
		}
	}
	return preflightCheck{name: name, ok: true}
}

func defaultTryListen(network, address string) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	return l.Close()
}

func secretsDirCheck(secretsDir string, deps preflightDeps) preflightCheck {
	name := "secrets directory at " + secretsDir
	info, err := deps.statPath(secretsDir)
	if err != nil {
		return preflightCheck{name: name, ok: false, detail: "missing — create it, or pass `--secrets-dir=<path>` to validate --check-host and apply"}
	}
	if !info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "exists but is not a directory"}
	}
	return preflightCheck{name: name, ok: true}
}

func secretFileCheck(refName, secretsDir, label string, deps preflightDeps) preflightCheck {
	path := filepath.Join(secretsDir, refName)
	name := label + " at " + path
	info, err := deps.statPath(path)
	if err != nil {
		detail := "missing"
		switch {
		case strings.Contains(label, "pullSecretRef"):
			detail = "missing — run `gitups secrets pull-secret set --name " + refName + " --from-file <path>`"
		case strings.Contains(label, "credentialRef") || strings.Contains(label, "credentialsRef"):
			// credentialRef and credentialsRef both store a single
			// `username:password` line; `gitups secrets bmc set` is the only
			// supported writer for that shape (proxy, mirror, BMC all share it).
			detail = "missing — run `gitups secrets bmc set --name " + refName + " --from-file <path>` (or `--generate` for test fixtures)"
		}
		return preflightCheck{name: name, ok: false, detail: detail}
	}
	if info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "is a directory; expected a file"}
	}
	return preflightCheck{name: name, ok: true}
}

func generatedSecretCheck(refName, secretsDir, label string) preflightCheck {
	path := filepath.Join(secretsDir, refName)
	return preflightCheck{name: label + " at " + path, ok: true, detail: "will be generated during hub apply"}
}

// secretRefRequirement describes a single SecretRef declared somewhere in the
// desired state, the phase that needs the file present on the host, and
// whether `gitups secrets generate` will materialize it during apply.
type secretRefRequirement struct {
	refName   string
	label     string
	phase     string
	generated bool
}

// secretRefChecks walks every SecretRef the state declares and emits one
// preflight per ref whose owning phase is in scope. The secrets directory
// check is prepended only when at least one in-scope ref needs it. Refs
// covered by `generatedSecrets` are reported as informational since
// `gitups secrets generate` will create them at apply time.
func secretRefChecks(state v1alpha1.State, secretsDir string, selected []Phase, deps preflightDeps) []preflightCheck {
	requirements := collectSecretRefRequirements(state)
	var inScope []secretRefRequirement
	for _, req := range requirements {
		if !phaseInScope(req.phase, selected, true) {
			continue
		}
		inScope = append(inScope, req)
	}
	if len(inScope) == 0 {
		return nil
	}
	checks := []preflightCheck{secretsDirCheck(secretsDir, deps)}
	for _, req := range inScope {
		if req.generated {
			checks = append(checks, generatedSecretCheck(req.refName, secretsDir, req.label))
			continue
		}
		checks = append(checks, secretFileCheck(req.refName, secretsDir, req.label, deps))
	}
	return checks
}

// collectSecretRefRequirements enumerates every SecretRef across all four
// kinds. Each requirement is tagged with the apply phase that first reads
// the file: provider-host SSH keys and BMC/proxy credentials are needed
// during infra-prepare; install-config material (pull secret, cluster SSH
// key, additional trust bundle, mirror credentials) is needed during the
// hub install. Managed-cluster install refs are intentionally excluded —
// hub-side gitops manifests carry them, not local apply.
func collectSecretRefRequirements(state v1alpha1.State) []secretRefRequirement {
	generated := allGeneratedSecretNames(state)
	var out []secretRefRequirement

	if env := environmentForChecks(state); env != nil {
		if proxy := v1alpha1.OCPInstallProxyOf(*env); proxy != nil && proxy.CredentialsRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: proxy.CredentialsRef.Name,
				label:   "ocpInstall proxy credentialsRef",
				phase:   "infra",
			})
		}
		if registries := v1alpha1.OCPInstallRegistriesOf(*env); registries != nil && registries.Mirror != nil && registries.Mirror.CredentialsRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: registries.Mirror.CredentialsRef.Name,
				label:   "registry mirror credentialsRef",
				phase:   "hub",
			})
		}
	}

	for _, p := range state.InfrastructureProviders {
		if v1alpha1.ProviderMachineLibvirt(p) != nil {
			for _, hostName := range sortedMapKeys(p.Spec.Machine.Libvirt.Hosts) {
				host := p.Spec.Machine.Libvirt.Hosts[hostName]
				if host.SSHKeyRef.Name == "" {
					continue
				}
				out = append(out, secretRefRequirement{
					refName: host.SSHKeyRef.Name,
					label:   fmt.Sprintf("provider %s host %s sshKeyRef", p.Metadata.Name, hostName),
					phase:   "infra",
				})
			}
			if bmc := p.Spec.Machine.Libvirt.BMCEmulation; bmc != nil && bmc.Auth != nil && bmc.Auth.CredentialRef.Name != "" {
				out = append(out, secretRefRequirement{
					refName: bmc.Auth.CredentialRef.Name,
					label:   fmt.Sprintf("provider %s bmcEmulation credentialRef", p.Metadata.Name),
					phase:   "infra",
				})
			}
		}
		if p.Spec.Machine != nil && p.Spec.Machine.Vsphere != nil && p.Spec.Machine.Vsphere.VCenterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.Vsphere.VCenterRef.Name,
				label:   fmt.Sprintf("provider %s vsphere vCenterRef", p.Metadata.Name),
				phase:   "infra",
			})
		}
		if p.Spec.Machine != nil && p.Spec.Machine.Kubevirt != nil && p.Spec.Machine.Kubevirt.ClusterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.Kubevirt.ClusterRef.Name,
				label:   fmt.Sprintf("provider %s kubevirt clusterRef", p.Metadata.Name),
				phase:   "infra",
			})
		}
	}

	for _, ci := range state.ClusterInfrastructures {
		for _, mname := range sortedMapKeys(ci.Spec.Machines) {
			m := ci.Spec.Machines[mname]
			if m.Baremetal == nil || m.Baremetal.BMC == nil || m.Baremetal.BMC.CredentialRef.Name == "" {
				continue
			}
			out = append(out, secretRefRequirement{
				refName: m.Baremetal.BMC.CredentialRef.Name,
				label:   fmt.Sprintf("infra %s machine %s baremetal bmc credentialRef", ci.Metadata.Name, mname),
				phase:   "infra",
			})
		}
	}

	for _, cluster := range state.OCPClusters {
		if cluster.Spec.Role != v1alpha1.OCPRoleHub {
			continue
		}
		install := cluster.Spec.Install
		if install.PullSecretRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: install.PullSecretRef.Name,
				label:   cluster.Metadata.Name + " pullSecretRef",
				phase:   "hub",
			})
		}
		if install.SSHKeyRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: install.SSHKeyRef.Name,
				label:   cluster.Metadata.Name + " sshKeyRef",
				phase:   "hub",
			})
		}
		if install.AdditionalTrustBundleRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName:   install.AdditionalTrustBundleRef.Name,
				label:     cluster.Metadata.Name + " additionalTrustBundleRef",
				phase:     "hub",
				generated: generated[install.AdditionalTrustBundleRef.Name],
			})
		}
	}
	return out
}

func environmentForChecks(state v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

func allGeneratedSecretNames(state v1alpha1.State) map[string]bool {
	out := map[string]bool{}
	for _, cluster := range state.OCPClusters {
		for _, item := range cluster.Spec.Install.GeneratedSecrets {
			out[item.Name] = true
		}
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hubNeedsOpenSSL(state v1alpha1.State, secretsDir string, deps preflightDeps) bool {
	for _, cluster := range state.OCPClusters {
		if cluster.Spec.Role != v1alpha1.OCPRoleHub {
			continue
		}
		for _, item := range cluster.Spec.Install.GeneratedSecrets {
			if item.Type == v1alpha1.GeneratedSecretSelfSigned && !generatedCertificatePairExists(item.Name, secretsDir, deps) {
				return true
			}
		}
	}
	return false
}

func generatedCertificatePairExists(refName, secretsDir string, deps preflightDeps) bool {
	certPath := filepath.Join(secretsDir, refName)
	keyPath := certPath + ".key"
	certInfo, certErr := deps.statPath(certPath)
	keyInfo, keyErr := deps.statPath(keyPath)
	return certErr == nil && keyErr == nil && !certInfo.IsDir() && !keyInfo.IsDir()
}

func generatedSecretNames(install v1alpha1.OCPInstallSpec) map[string]bool {
	result := map[string]bool{}
	for _, item := range install.GeneratedSecrets {
		result[item.Name] = true
	}
	return result
}

func defaultLookPath(name string, extraDirs []string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, dir := range extraDirs {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", exec.ErrNotFound
}
