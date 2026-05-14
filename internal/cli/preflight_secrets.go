package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

type secretRefRequirement struct {
	refName   string
	label     string
	phases    []string
	generated bool
	publicKey bool
}

func secretRefChecks(state v1alpha1.State, secretsDir string, selected []Phase, deps preflightDeps) []preflightCheck {
	requirements := collectSecretRefRequirements(state)
	var inScope []secretRefRequirement
	needsSecretsDir := false
	for _, req := range requirements {
		if !anyPhaseInScope(req.phases, selected) {
			continue
		}
		if req.generated {
			needsSecretsDir = true
		}
		inScope = append(inScope, req)
	}
	if len(inScope) == 0 {
		return nil
	}
	env := environmentForChecks(state)
	var checks []preflightCheck
	if needsSecretsDir {
		checks = append(checks, secretsDirCheck(secretsDir, deps))
	}
	for _, req := range inScope {
		if req.generated {
			path := filepath.Join(secretsDir, req.refName)
			checks = append(checks, generatedSecretCheck(path, req.label, deps))
			continue
		}
		path := resolvedSecretPath(req.refName, env, secretsDir)
		checks = append(checks, secretFileCheck(req.refName, path, req.label, req.publicKey, deps))
	}
	return checks
}

func collectSecretRefRequirements(state v1alpha1.State) []secretRefRequirement {
	generated := allGeneratedSecretNames(state)
	var out []secretRefRequirement

	if env := environmentForChecks(state); env != nil {
		if env.Spec.Proxy != nil && env.Spec.Proxy.Auth != nil && env.Spec.Proxy.Auth.ProxyAuthRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: env.Spec.Proxy.Auth.ProxyAuthRef.Name,
				label:   "proxy proxyAuthRef",
				phases:  []string{"provider", "cluster"},
			})
		}
		if registries := env.Spec.Registries; registries != nil && registries.Mirror != nil && registries.Mirror.CredentialsRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: registries.Mirror.CredentialsRef.Name,
				label:   "registry mirror credentialsRef",
				phases:  []string{"provider", "clusters"},
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
					phases:  []string{"provider", "clusters"},
				})
			}
		}
		if p.Spec.Machine != nil && p.Spec.Machine.VSphere != nil && p.Spec.Machine.VSphere.VCenterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.VSphere.VCenterRef.Name,
				label:   fmt.Sprintf("provider %s vsphere vCenterRef", p.Metadata.Name),
				phases:  []string{"provider"},
			})
		}
		if p.Spec.Machine != nil && p.Spec.Machine.KubeVirt != nil && p.Spec.Machine.KubeVirt.ClusterRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: p.Spec.Machine.KubeVirt.ClusterRef.Name,
				label:   fmt.Sprintf("provider %s kubevirt clusterRef", p.Metadata.Name),
				phases:  []string{"provider"},
			})
		}
	}

	for _, ci := range state.ClusterInfrastructures {
		for _, mname := range sortedMapKeys(ci.Spec.Machines) {
			m := ci.Spec.Machines[mname]
			if m.BareMetal == nil || m.BareMetal.BMC == nil || m.BareMetal.BMC.CredentialRef.Name == "" {
				continue
			}
			out = append(out, secretRefRequirement{
				refName: m.BareMetal.BMC.CredentialRef.Name,
				label:   fmt.Sprintf("infra %s machine %s baremetal bmc credentialRef", ci.Metadata.Name, mname),
				phases:  []string{"provider", "clusters"},
			})
		}
	}

	for _, cluster := range state.OCPClusters {
		install := cluster.Spec.Install
		if install.PullSecretRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName: install.PullSecretRef.Name,
				label:   cluster.Metadata.Name + " pullSecretRef",
				phases:  []string{"clusters"},
			})
		}
		if install.SSHKeyRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName:   install.SSHKeyRef.Name,
				label:     cluster.Metadata.Name + " sshKeyRef",
				phases:    []string{"clusters"},
				publicKey: true,
			})
		}
		if install.AdditionalTrustBundleRef.Name != "" {
			out = append(out, secretRefRequirement{
				refName:   install.AdditionalTrustBundleRef.Name,
				label:     cluster.Metadata.Name + " additionalTrustBundleRef",
				phases:    []string{"clusters"},
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
	if env := primaryEnvironmentForSync(state); env != nil {
		for name, key := range env.Spec.Secrets {
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

func clustersNeedOpenSSL(state v1alpha1.State, secretsDir string, deps preflightDeps) bool {
	env := primaryEnvironmentForSync(state)
	if env == nil {
		return false
	}
	for name, key := range env.Spec.Secrets {
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
				detail: fmt.Sprintf("%v — remove %s and %s, then re-run `bootwright secret generate`", err, certPath, keyPath),
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
