package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func collectBastionChecks(state v1alpha1.State, hostStateDir string, deps preflightDeps) []preflightCheck {
	checks := []preflightCheck{
		pythonVersionCheck(),
		binaryCheck("ansible-playbook", []string{filepath.Join(ansibleVenvDir(), "bin")}, deps),
		binaryCheck("tar", nil, deps),
	}
	if os.Getuid() != 0 {
		checks = append(checks, binaryCheck("sudo", nil, deps))
	}
	if stateOpenshiftReleaseVersion(state) != "" {
		checks = append(checks,
			binaryCheck("openshift-install", openshiftInstallSearchDirs(hostStateDir), deps),
			binaryCheck("oc", openshiftInstallSearchDirs(hostStateDir), deps),
			binaryCheck("kubectl", openshiftInstallSearchDirs(hostStateDir), deps),
		)
	}
	return checks
}

func pythonVersionCheck() preflightCheck {
	name := "python3 version >= 3.12"
	for _, bin := range []string{"python3.12", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		out, err := exec.Command(bin, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		major, minor, err := parsePythonVersion(strings.TrimSpace(string(out)))
		if err != nil {
			continue
		}
		ver := fmt.Sprintf("%d.%d", major, minor)
		if major > 3 || (major == 3 && minor >= 12) {
			return preflightCheck{name: name, ok: true, detail: bin + " " + ver}
		}
		return preflightCheck{name: name, ok: false, detail: bin + " is " + ver + "; run `gitups apply bastion`"}
	}
	return preflightCheck{name: name, ok: false, detail: "python3 not found; run `gitups apply bastion`"}
}

func parsePythonVersion(s string) (major, minor int, err error) {
	s = strings.TrimPrefix(s, "Python ")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("unexpected version string: %q", s)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse major: %w", err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse minor: %w", err)
	}
	return major, minor, nil
}

type preflightCheck struct {
	name   string
	ok     bool
	detail string
}

type preflightDeps struct {
	lookPath   func(name string, extraDirs []string) (string, error)
	statPath   func(path string) (os.FileInfo, error)
	tryListen  func(network, address string) error
	unitActive func(unit string) bool
}

var defaultPreflightDeps = preflightDeps{
	lookPath:   defaultLookPath,
	statPath:   os.Stat,
	tryListen:  defaultTryListen,
	unitActive: defaultUnitActive,
}

func collectPreflightChecks(state v1alpha1.State, selected []Phase, hasState bool, secretsDir string, hostStateDir string, deps preflightDeps) []preflightCheck {
	checks := []preflightCheck{
		binaryCheck("ansible-playbook", []string{filepath.Join(ansibleVenvDir(), "bin")}, deps),
		binaryCheck("python3", nil, deps),
	}
	if phaseInScope("provider", selected, hasState) && stateNeedsLibvirt(state) {
		checks = append(checks, bmcPortChecks(state, deps)...)
	}
	if phaseInScope("cluster", selected, hasState) && stateNeedsLocalLibvirt(state) {
		checks = append(checks, kvmCheck(deps))
	}
	if phaseInScope("clusters", selected, hasState) {
		checks = append(checks,
			binaryCheck("openshift-install", openshiftInstallSearchDirs(hostStateDir), deps),
			binaryCheck("oc", openshiftInstallSearchDirs(hostStateDir), deps),
			binaryCheck("kubectl", openshiftInstallSearchDirs(hostStateDir), deps),
		)
		if hasState && clustersNeedOpenSSL(state, secretsDir, deps) {
			checks = append(checks, binaryCheck("openssl", nil, deps))
		}
	}
	if hasState {
		checks = append(checks, secretRefChecks(state, secretsDir, selected, deps)...)
		checks = append(checks, generatedSelfSignedDriftChecks(state, secretsDir)...)
	}
	return checks
}

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

func anyPhaseInScope(names []string, selected []Phase) bool {
	for _, name := range names {
		if phaseInScope(name, selected, true) {
			return true
		}
	}
	return false
}

func stateNeedsLibvirt(state v1alpha1.State) bool {
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.MachineFlavor(p) == v1alpha1.MachineFlavorLibvirt {
			return true
		}
	}
	return false
}

func stateNeedsLocalLibvirt(state v1alpha1.State) bool {
	for _, p := range state.InfrastructureProviders {
		libvirt := v1alpha1.ProviderMachineLibvirt(p)
		if libvirt == nil {
			continue
		}
		if !providerLibvirtIsRemote(p) {
			return true
		}
	}
	return false
}

func providerLibvirtIsRemote(p v1alpha1.InfrastructureProvider) bool {
	libvirt := v1alpha1.ProviderMachineLibvirt(p)
	if libvirt == nil || len(libvirt.HostRefs) == 0 {
		return false
	}
	for _, ref := range libvirt.HostRefs {
		host, ok := p.Spec.Hosts[ref.Name]
		if !ok || host.SSH == nil {
			return false
		}
	}
	return true
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
		return preflightCheck{name: "/dev/kvm available", ok: false, detail: "missing — libvirt provider requires KVM hardware acceleration"}
	}
	return preflightCheck{name: "/dev/kvm available", ok: true}
}

func bmcPortChecks(state v1alpha1.State, deps preflightDeps) []preflightCheck {
	var checks []preflightCheck
	for _, p := range state.InfrastructureProviders {
		if v1alpha1.ProviderMachineLibvirt(p) == nil || p.Spec.Machine.Libvirt.BMCEmulation == nil {
			continue
		}
		if providerLibvirtIsRemote(p) {
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
			label    string
			addr     string
			port     int
			unitKind string
		}{
			{"redfish", bind, bmc.Port, "sushy"},
			{"vmedia HTTP", "127.0.0.1", bmc.Port + 1, "vmedia"},
			{"boot-artifacts HTTP", "0.0.0.0", bmc.Port + 2, "boot-artifacts"},
		}
		for _, item := range probes {
			checks = append(checks, bmcPortCheck(p.Metadata.Name, item.label, item.addr, item.port, item.unitKind, deps))
		}
	}
	return checks
}

func bmcPortCheck(providerName, label, bindAddr string, port int, unitKind string, deps preflightDeps) preflightCheck {
	addr := net.JoinHostPort(bindAddr, strconv.Itoa(port))
	name := fmt.Sprintf("provider %s %s port %s free", providerName, label, addr)
	if err := deps.tryListen("tcp", addr); err != nil {
		expectedUnit := fmt.Sprintf("gitups-%s-%s.service", unitKind, providerName)
		if deps.unitActive != nil && deps.unitActive(expectedUnit) {
			return preflightCheck{
				name:   name,
				ok:     true,
				detail: fmt.Sprintf("held by %s (will be reconfigured)", expectedUnit),
			}
		}
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

func defaultUnitActive(unit string) bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	cmd := exec.Command("systemctl", "is-active", "--quiet", unit)
	return cmd.Run() == nil
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
	if got := info.Mode().Perm(); got != 0o700 {
		return preflightCheck{name: name, ok: false, detail: fmt.Sprintf("mode %04o; expected 0700", got)}
	}
	return preflightCheck{name: name, ok: true}
}

func secretFileCheck(refName, path, label string, publicKey bool, deps preflightDeps) preflightCheck {
	name := label + " at " + path
	info, err := deps.statPath(path)
	if err != nil {
		detail := "missing"
		switch {
		case strings.Contains(label, "pullSecretRef"):
			detail = "missing — run `gitups secret set " + refName + " --pull-secret <path>`"
		case strings.Contains(label, "credentialRef") || strings.Contains(label, "credentialsRef") || strings.Contains(label, "proxyAuthRef"):
			detail = "missing — run `gitups secret set " + refName + " --from-file <path>` (or `--generate` for test fixtures)"
		case strings.Contains(label, "sshKeyRef"):
			detail = "missing — ensure the file exists at the path declared in Environment.spec.secrets[" + refName + "].file"
		}
		return preflightCheck{name: name, ok: false, detail: detail}
	}
	if info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "is a directory; expected a file"}
	}
	if !publicKey {
		if got := info.Mode().Perm(); got != 0o600 {
			return preflightCheck{name: name, ok: false, detail: fmt.Sprintf("mode %04o; expected 0600", got)}
		}
	}
	return preflightCheck{name: name, ok: true}
}

func generatedSecretCheck(path, label string, deps preflightDeps) preflightCheck {
	name := label + " at " + path
	info, err := deps.statPath(path)
	if err != nil {
		return preflightCheck{name: name, ok: false, detail: "missing — run `gitups secret generate` before apply"}
	}
	if info.IsDir() {
		return preflightCheck{name: name, ok: false, detail: "is a directory; expected a generated file"}
	}
	if got := info.Mode().Perm(); got != 0o600 {
		return preflightCheck{name: name, ok: false, detail: fmt.Sprintf("mode %04o; expected 0600", got)}
	}
	return preflightCheck{name: name, ok: true}
}
