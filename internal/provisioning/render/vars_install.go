package render

import (
	"net/netip"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/proxy"
)

func ocpInstallEnvVars(state v1alpha1.State, env *v1alpha1.Environment, secretsDir string) EnvironmentOCPInstallVars {
	kind := v1alpha1.OCPInstallKindConnected
	if env != nil {
		if k := v1alpha1.OCPInstallKind(*env); k != "" {
			kind = k
		}
	}
	result := EnvironmentOCPInstallVars{
		Mode:         kind,
		Disconnected: kind == v1alpha1.OCPInstallKindDisconnected,
	}
	if env == nil {
		return result
	}
	if eff := proxy.Resolve(state, env); eff != nil {
		hostFallback := managedProxyClientHostURLForState(state, env)
		vmFallback := managedProxyClientURLForState(state, env)
		httpProxy, httpsProxy := effectiveProxyURLs(eff, hostFallback)
		vmHTTP, vmHTTPS := effectiveProxyURLs(eff, vmFallback)
		if httpProxy != "" || httpsProxy != "" || vmHTTP != "" || vmHTTPS != "" || len(eff.NoProxy) > 0 || eff.Auth.Name != "" {
			result.Proxy = &ProxyVars{
				HTTP:         httpProxy,
				HTTPS:        httpsProxy,
				VMHTTP:       vmHTTP,
				VMHTTPS:      vmHTTPS,
				NoProxy:      append([]string(nil), eff.NoProxy...),
				ProxyAuthRef: resolvedSecretPath(eff.Auth.Name, secretsDir, env),
			}
		}
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return result
	}
	result.Registry = &MirrorRegistryVars{
		URL:            registries.Mirror.URL,
		Host:           mirrorRegistryHostname(registries.Mirror.URL),
		CredentialsRef: resolvedSecretPath(registries.Mirror.CredentialsRef.Name, secretsDir, env),
	}
	return result
}

func ocpReleaseVars(ocp v1alpha1.OCPCluster) OCPReleaseVars {
	if ocp.Spec.Install.Release == nil {
		return OCPReleaseVars{}
	}
	return OCPReleaseVars{
		Channel: ocp.Spec.Install.Release.Channel,
		Version: ocp.Spec.Install.Release.Version,
	}
}

func ocpInstallVars(ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) OCPInstallVars {
	return OCPInstallVars{
		Method:                   ocp.Spec.Install.Method,
		BaseDomain:               ocp.Spec.Install.BaseDomain,
		PullSecretRef:            resolvedSecretPath(ocp.Spec.Install.PullSecretRef.Name, secretsDir, env),
		SSHKeyRef:                resolvedSecretPath(ocp.Spec.Install.SSHKeyRef.Name, secretsDir, env),
		ReleaseImageOverride:     releaseImageOverride(ocp),
		AdditionalTrustBundleRef: resolvedSecretPath(ocp.Spec.Install.AdditionalTrustBundleRef.Name, secretsDir, env),
		GeneratedSecrets:         generatedSecretVarsFromEnv(env, secretsDir),
		LocalRegistry:            localRegistryVars(env, ocp, secretsDir),
	}
}

func releaseImageOverride(ocp v1alpha1.OCPCluster) string {
	if ocp.Spec.Install.Release == nil || ocp.Spec.Install.Release.Version == "" {
		return ""
	}
	for _, source := range ocp.Spec.Install.ImageDigestSources {
		if source.Source != v1alpha1.OCPReleaseSourceQuayOCPRelease || len(source.Mirrors) == 0 {
			continue
		}
		return strings.TrimRight(source.Mirrors[0], "/") + ":" + ocp.Spec.Install.Release.Version + "-x86_64"
	}
	return ""
}

func localRegistryVars(env *v1alpha1.Environment, ocp v1alpha1.OCPCluster, secretsDir string) *LocalRegistryVars {
	if env == nil {
		return nil
	}
	kind := v1alpha1.OCPInstallKind(*env)
	if kind == "" || kind == v1alpha1.OCPInstallKindConnected {
		return nil
	}
	registries := env.Spec.Registries
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	return &LocalRegistryVars{
		Registry: MirrorRegistryVars{
			URL:            registries.Mirror.URL,
			Host:           mirrorRegistryHostname(registries.Mirror.URL),
			CredentialsRef: resolvedSecretPath(registries.Mirror.CredentialsRef.Name, secretsDir, env),
			TrustBundleRef: resolvedSecretPath(ocp.Spec.Install.AdditionalTrustBundleRef.Name, secretsDir, env),
		},
	}
}

func generatedSecretVarsFromEnv(env *v1alpha1.Environment, secretsDir string) []GeneratedSecretVars {
	if env == nil {
		return nil
	}
	names := make([]string, 0, len(env.Spec.Secrets))
	for name, secret := range env.Spec.Secrets {
		if secret.Generated == nil || secret.Generated.SelfSignedCertificate == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]GeneratedSecretVars, 0, len(names))
	for _, name := range names {
		cert := env.Spec.Secrets[name].Generated.SelfSignedCertificate
		result = append(result, GeneratedSecretVars{
			Name:                  name,
			Path:                  filepath.Join(secretsDir, name),
			Type:                  v1alpha1.GeneratedSecretSelfSigned,
			SelfSignedCertificate: selfSignedCertificateVars(*cert),
		})
	}
	return result
}

func selfSignedCertificateVars(item v1alpha1.SelfSignedCertificateSpec) *SelfSignedCertificateVars {
	dnsNames := append([]string(nil), item.DNSNames...)
	ipAddresses := append([]string(nil), item.IPAddresses...)
	if len(dnsNames) == 0 && len(ipAddresses) == 0 {
		if _, err := netip.ParseAddr(item.CommonName); err == nil {
			ipAddresses = append(ipAddresses, item.CommonName)
		} else if item.CommonName != "" {
			dnsNames = append(dnsNames, item.CommonName)
		}
	}
	return &SelfSignedCertificateVars{
		CommonName:     item.CommonName,
		DNSNames:       dnsNames,
		IPAddresses:    ipAddresses,
		ValidityDays:   item.ValidityDays,
		SubjectAltName: subjectAltName(dnsNames, ipAddresses),
	}
}

func subjectAltName(dnsNames, ipAddresses []string) string {
	entries := make([]string, 0, len(dnsNames)+len(ipAddresses))
	for _, name := range dnsNames {
		entries = append(entries, "DNS:"+name)
	}
	for _, address := range ipAddresses {
		entries = append(entries, "IP:"+address)
	}
	return strings.Join(entries, ",")
}

func ocpInstallerVars(clusterName string) OCPInstallerVars {
	dir := installerRelativeDir(clusterName)
	workDir := installerRelativeWorkDir(clusterName)
	return OCPInstallerVars{
		RelativeDir:               dir,
		RelativeInstallConfigPath: dir + "/install-config.yaml",
		RelativeAgentConfigPath:   dir + "/agent-config.yaml",
		RelativeWorkDir:           workDir,
	}
}

func installerRelativeDir(clusterName string) string {
	return BootstrapRepoRelativeDir + "/" + clusterName + "/openshift"
}

// installerRelativeWorkDir is the local-only runtime tree consumed by
// openshift-install. Kept outside the bootstrap repo so the repo only
// contains declarative config safe to push to a Git provider.
func installerRelativeWorkDir(clusterName string) string {
	return RuntimeRelativeDir + "/" + clusterName + "/installer"
}

func ocpClusterNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment, secretsDir string) []OCPClusterNodeVars {
	nodeNames := sortedKeys(ocp.Spec.Nodes)
	result := make([]OCPClusterNodeVars, 0, len(nodeNames))
	for _, name := range nodeNames {
		node := ocp.Spec.Nodes[name]
		machineRef := name
		if node.MachineRef != nil && node.MachineRef.Name != "" {
			machineRef = node.MachineRef.Name
		}
		entry := OCPClusterNodeVars{
			Name:       name,
			MachineRef: machineRef,
			Role:       node.Role,
		}
		if machine, ok := item.Spec.Machines[machineRef]; ok {
			if machine.Libvirt != nil {
				entry.HostRef = machine.Libvirt.HostRef.Name
			}
			if machine.BareMetal != nil && machine.BareMetal.BMC != nil {
				entry.BareMetal = &MachineBMCVars{
					Address:                        machine.BareMetal.BMC.Address,
					Port:                           machine.BareMetal.BMC.Port,
					Protocol:                       machine.BareMetal.BMC.Protocol,
					CredentialRef:                  resolvedSecretPath(machine.BareMetal.BMC.CredentialRef.Name, secretsDir, env),
					DisableCertificateVerification: machine.BareMetal.BMC.DisableCertificateVerification,
					BootMACAddress:                 machine.BareMetal.BootMACAddress,
				}
			}
			primary := primaryInterface(machine)
			entry.IPAddress = primary.IPAddress
			entry.MACAddress = primary.MACAddress
		}
		result = append(result, entry)
	}
	return result
}
