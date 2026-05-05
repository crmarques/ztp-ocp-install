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
	// ansible-playbook is searched in the gitups-managed venv as a fallback
	// so the universal "ansible-playbook on PATH" check still passes after
	// `gitups setup controller --venv` even on a host that has no system
	// ansible-core installed.
	checks := []preflightCheck{
		binaryCheck("ansible-playbook", []string{filepath.Join(ansibleVenvDir(), "bin")}, deps),
		binaryCheck("python3", nil, deps),
		binaryCheck("sudo", nil, deps),
	}
	if phaseInScope("provider", selected, hasState) && stateNeedsQemuKvm(state) {
		// BMC emulator, vmedia HTTP, and boot-artifacts HTTP all bind during
		// the provider phase (sushy-tools, vmedia, boot-artifacts services).
		checks = append(checks, bmcPortChecks(state, deps)...)
	}
	if phaseInScope("cluster", selected, hasState) && stateNeedsQemuKvm(state) {
		// Substrate creates libvirt domains; KVM acceleration is mandatory.
		checks = append(checks, kvmCheck(deps))
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
	if hasState {
		checks = append(checks, secretRefChecks(state, secretsDir, selected, deps)...)
		checks = append(checks, generatedSelfSignedDriftChecks(state, secretsDir)...)
	}
	return checks
}

// phaseInScope returns true when the current workflow selection includes the
// named phase. With no selection, every implemented phase is in scope iff the
// user supplied desired-state input. With neither, only universal checks run.
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

// anyPhaseInScope reports true when at least one of the named phases is in
// scope under the current workflow selection. Used by secret-ref checks
// where a single ref may be read by multiple phases (host_proxy runs in
// both provider and cluster; mirror credentials are read in provider and
// hub).
func anyPhaseInScope(names []string, selected []Phase) bool {
	for _, name := range names {
		if phaseInScope(name, selected, true) {
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
			// `username:password` line; `gitups secrets credentials set` is the only
			// supported writer for that shape (proxy, mirror, BMC all share it).
			detail = "missing — run `gitups secrets credentials set --name " + refName + " --from-file <path>` (or `--generate` for test fixtures)"
		}
		return preflightCheck{name: name, ok: false, detail: detail}
	}
	if info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "is a directory; expected a file"}
	}
	return preflightCheck{name: name, ok: true}
}

func generatedSecretCheck(refName, secretsDir, label string, deps preflightDeps) preflightCheck {
	path := filepath.Join(secretsDir, refName)
	name := label + " at " + path
	info, err := deps.statPath(path)
	if err != nil {
		return preflightCheck{name: name, ok: false, detail: "missing — run `gitups secrets generate` before apply"}
	}
	if info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "is a directory; expected a generated file"}
	}
	return preflightCheck{name: name, ok: true}
}

// secretRefRequirement describes a single SecretRef declared somewhere in the
// desired state, the phases that need the file present on the host, and
// whether `gitups secrets generate` can materialize it before apply. A
// single ref may be read by multiple phases (e.g. host_proxy runs in both
// the provider and cluster phases) so phases is a list.
type secretRefRequirement struct {
	refName   string
	label     string
	phases    []string
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
		if !anyPhaseInScope(req.phases, selected) {
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
			checks = append(checks, generatedSecretCheck(req.refName, secretsDir, req.label, deps))
			continue
		}
		checks = append(checks, secretFileCheck(req.refName, secretsDir, req.label, deps))
	}
	return checks
}

// collectSecretRefRequirements enumerates every SecretRef across all four
// kinds. Each requirement is tagged with the apply phases that read the
// file:
//
//   - proxy credentialsRef: host_proxy runs in both provider and cluster
//   - registry mirror credentialsRef: provider_mirror_registry (provider) and
//     hub_install_agent merges it into install-config (hub)
//   - provider host sshKeyRef: ansible connection for any phase that targets
//     gitups_provider_hosts or gitups_infra_hosts (provider, cluster)
//   - libvirt BMC emulation credentialRef: provider_bmc_emulated (provider)
//     and hub_install_agent Redfish auth (hub)
//   - vsphere/kubevirt provider refs: provider only (substrate roles)
//   - per-machine baremetal BMC credentialRef: provider_bmc_redfish (provider)
//     and hub_install_agent Redfish auth (hub)
//   - hub install pullSecretRef / sshKeyRef / additionalTrustBundleRef: hub
//
// Managed-cluster install refs are intentionally excluded — hub-side gitops
// manifests carry them, not local apply.
func collectSecretRefRequirements(state v1alpha1.State) []secretRefRequirement {
	generated := allGeneratedSecretNames(state)
	var out []secretRefRequirement

	if env := environmentForChecks(state); env != nil {
		if proxy := v1alpha1.OCPInstallProxyOf(*env); proxy != nil && proxy.CredentialsRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: proxy.CredentialsRef.Name,
				label:   "ocpInstall proxy credentialsRef",
				phases:  []string{"provider", "cluster"},
			})
		}
		if registries := v1alpha1.OCPInstallRegistriesOf(*env); registries != nil && registries.Mirror != nil && registries.Mirror.CredentialsRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: registries.Mirror.CredentialsRef.Name,
				label:   "registry mirror credentialsRef",
				phases:  []string{"provider", "hub"},
			})
		}
	}

	for _, p := range state.InfrastructureProviders {
		for _, hostName := range sortedMapKeys(p.Spec.Hosts) {
			host := p.Spec.Hosts[hostName]
			if host.SSH == nil || host.SSH.KeyRef.Name == "" {
				continue
			}
			out = append(out, secretRefRequirement{
				refName: host.SSH.KeyRef.Name,
				label:   fmt.Sprintf("provider %s host %s sshKeyRef", p.Metadata.Name, hostName),
				phases:  []string{"provider", "cluster"},
			})
		}
		if libvirt := v1alpha1.ProviderMachineLibvirt(p); libvirt != nil {
			if bmc := libvirt.BMCEmulation; bmc != nil && bmc.Auth != nil && bmc.Auth.CredentialRef.Name != "" {
				out = append(out, secretRefRequirement{
					refName: bmc.Auth.CredentialRef.Name,
					label:   fmt.Sprintf("provider %s bmcEmulation credentialRef", p.Metadata.Name),
					phases:  []string{"provider", "hub"},
				})
			}
		}
		if p.Spec.Machine != nil && p.Spec.Machine.Vsphere != nil && p.Spec.Machine.Vsphere.VCenterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.Vsphere.VCenterRef.Name,
				label:   fmt.Sprintf("provider %s vsphere vCenterRef", p.Metadata.Name),
				phases:  []string{"provider"},
			})
		}
		if p.Spec.Machine != nil && p.Spec.Machine.Kubevirt != nil && p.Spec.Machine.Kubevirt.ClusterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.Kubevirt.ClusterRef.Name,
				label:   fmt.Sprintf("provider %s kubevirt clusterRef", p.Metadata.Name),
				phases:  []string{"provider"},
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
				phases:  []string{"provider", "hub"},
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
				phases:  []string{"hub"},
			})
		}
		if install.SSHKeyRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: install.SSHKeyRef.Name,
				label:   cluster.Metadata.Name + " sshKeyRef",
				phases:  []string{"hub"},
			})
		}
		if install.AdditionalTrustBundleRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName:   install.AdditionalTrustBundleRef.Name,
				label:     cluster.Metadata.Name + " additionalTrustBundleRef",
				phases:    []string{"hub"},
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

// allGeneratedSecretNames returns the set of SecretRef names that
// `gitups secrets generate` will materialise — every Environment.spec.keys
// entry whose source is `generated:` (cert or credentials).
func allGeneratedSecretNames(state v1alpha1.State) map[string]bool {
	out := map[string]bool{}
	if env := primaryEnvironmentForSync(state); env != nil {
		for name, key := range env.Spec.Keys {
			if key.Generated != nil {
				out[name] = true
			}
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

// hubNeedsOpenSSL reports whether the hub install will fall back to the
// in-cluster ansible cert generator (`community.crypto.openssl_*`) — i.e.
// at least one Environment.spec.keys[name].generated.selfSignedCertificate
// has not yet been materialised on the operator host. The fallback path
// requires `openssl` on PATH; the operator-side path does not.
func hubNeedsOpenSSL(state v1alpha1.State, secretsDir string, deps preflightDeps) bool {
	env := primaryEnvironmentForSync(state)
	if env == nil {
		return false
	}
	for name, key := range env.Spec.Keys {
		if key.Generated == nil || key.Generated.SelfSignedCertificate == nil {
			continue
		}
		if !generatedCertificatePairExists(name, secretsDir, deps) {
			return true
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

// generatedSelfSignedDriftChecks fails the preflight when a previously
// materialised self-signed cert on disk no longer matches the desired
// SelfSignedCertificateSpec (commonName / dnsNames / ipAddresses). Without
// this, apply happily reuses the stale cert and the failure surfaces deep
// inside ansible (e.g. mirror push fails with x509 SAN mismatch).
func generatedSelfSignedDriftChecks(state v1alpha1.State, secretsDir string) []preflightCheck {
	requests, err := generatedSelfSignedRequests(state)
	if err != nil {
		return []preflightCheck{{
			name:   "generated self-signed certificate requests are consistent",
			ok:     false,
			detail: err.Error(),
		}}
	}
	var checks []preflightCheck
	for _, req := range requests {
		certPath := filepath.Join(secretsDir, req.name)
		keyPath := certPath + ".key"
		certExists, err := regularFileExists(certPath)
		if err != nil {
			checks = append(checks, preflightCheck{
				name:   "generated self-signed certificate " + req.name + " on disk matches desired spec",
				ok:     false,
				detail: err.Error(),
			})
			continue
		}
		if !certExists {
			continue
		}
		name := "generated self-signed certificate " + req.name + " on disk matches desired spec"
		if err := verifySelfSignedCertificateMatchesRequest(certPath, req.certificate); err != nil {
			checks = append(checks, preflightCheck{
				name:   name,
				ok:     false,
				detail: fmt.Sprintf("%v — remove %s and %s, then re-run `gitups secrets generate`", err, certPath, keyPath),
			})
			continue
		}
		checks = append(checks, preflightCheck{name: name, ok: true})
	}
	return checks
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
