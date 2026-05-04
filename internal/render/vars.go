package render

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type VarsFile struct {
	GitupsOCPInstall    EnvironmentOCPInstallVars `yaml:"gitups_ocp_install" json:"gitups_ocp_install"`
	GitupsProviders     []ProviderComponentVars   `yaml:"gitups_providers" json:"gitups_providers"`
	GitupsLoadBalancers []SharedLoadBalancerVars  `yaml:"gitups_load_balancers" json:"gitups_load_balancers"`
	GitupsClusters      []ClusterVars             `yaml:"gitups_clusters" json:"gitups_clusters"`
	GitupsComponentPins []ComponentPin            `yaml:"gitups_component_pins" json:"gitups_component_pins"`
}

// EnvironmentOCPInstallVars projects Environment.spec.ocpInstall to Ansible
// vars. Only OpenShift install material (release payload, mirror registry,
// trust bundles) is gated by these values.
type EnvironmentOCPInstallVars struct {
	Mode         string              `yaml:"mode" json:"mode"`
	Disconnected bool                `yaml:"disconnected" json:"disconnected"`
	Registry     *MirrorRegistryVars `yaml:"registry,omitempty" json:"registry,omitempty"`
	Proxy        *ProxyVars          `yaml:"proxy,omitempty" json:"proxy,omitempty"`
}

// ProxyVars projects Environment.spec.ocpInstall.{restricted,disconnected}.proxy
// onto Ansible vars. The host_proxy role and per-task `environment:` blocks
// consume these to drive package, container-pull, and openshift-install traffic
// through the configured proxy while leaving NoProxy ranges direct.
//
// CredentialsRef points at a single-line `username:password` secret. When set,
// the host_proxy role merges the resolved credentials into the URLs at apply
// time (writing credentialed values to /etc/environment, dnf/pip/podman, and
// the systemd default environment), and the hub_install_agent role injects
// credentialed URLs into the effective install-config. The plaintext URLs in
// vars.yaml stay credential-free.
type ProxyVars struct {
	HTTPProxy      string   `yaml:"httpProxy,omitempty" json:"httpProxy,omitempty"`
	HTTPSProxy     string   `yaml:"httpsProxy,omitempty" json:"httpsProxy,omitempty"`
	NoProxy        []string `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
	CredentialsRef string   `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
}

type ClusterVars struct {
	Name     string             `yaml:"name" json:"name"`
	Role     string             `yaml:"role" json:"role"`
	OCP      OCPClusterVars     `yaml:"ocp" json:"ocp"`
	Provider ProviderVars       `yaml:"provider" json:"provider"`
	Network  ClusterNetworkVars `yaml:"network" json:"network"`
}

type OCPClusterVars struct {
	Name      string               `yaml:"name" json:"name"`
	Topology  string               `yaml:"topology" json:"topology"`
	Release   OCPReleaseVars       `yaml:"release" json:"release"`
	Install   OCPInstallVars       `yaml:"install" json:"install"`
	Installer OCPInstallerVars     `yaml:"installer" json:"installer"`
	Nodes     []OCPClusterNodeVars `yaml:"nodes" json:"nodes"`
}

type OCPReleaseVars struct {
	Channel string `yaml:"channel" json:"channel"`
	Version string `yaml:"version" json:"version"`
}

type OCPInstallVars struct {
	Method                   string                `yaml:"method" json:"method"`
	BaseDomain               string                `yaml:"baseDomain" json:"baseDomain"`
	PullSecretRef            string                `yaml:"pullSecretRef" json:"pullSecretRef"`
	SSHKeyRef                string                `yaml:"sshKeyRef" json:"sshKeyRef"`
	ReleaseImageOverride     string                `yaml:"releaseImageOverride,omitempty" json:"releaseImageOverride,omitempty"`
	AdditionalTrustBundleRef string                `yaml:"additionalTrustBundleRef,omitempty" json:"additionalTrustBundleRef,omitempty"`
	GeneratedSecrets         []GeneratedSecretVars `yaml:"generatedSecrets,omitempty" json:"generatedSecrets,omitempty"`
	LocalRegistry            *LocalRegistryVars    `yaml:"localRegistry,omitempty" json:"localRegistry,omitempty"`
}

type LocalRegistryVars struct {
	Registry MirrorRegistryVars `yaml:"registry" json:"registry"`
}

type MirrorRegistryVars struct {
	URL            string `yaml:"url" json:"url"`
	Host           string `yaml:"host" json:"host"`
	CredentialsRef string `yaml:"credentialsRef" json:"credentialsRef"`
	TrustBundleRef string `yaml:"trustBundleRef,omitempty" json:"trustBundleRef,omitempty"`
}

type GeneratedSecretVars struct {
	Name                  string                     `yaml:"name" json:"name"`
	Type                  string                     `yaml:"type" json:"type"`
	SelfSignedCertificate *SelfSignedCertificateVars `yaml:"selfSignedCertificate,omitempty" json:"selfSignedCertificate,omitempty"`
}

type SelfSignedCertificateVars struct {
	CommonName     string   `yaml:"commonName" json:"commonName"`
	DNSNames       []string `yaml:"dnsNames,omitempty" json:"dnsNames,omitempty"`
	IPAddresses    []string `yaml:"ipAddresses,omitempty" json:"ipAddresses,omitempty"`
	ValidityDays   int      `yaml:"validityDays" json:"validityDays"`
	SubjectAltName string   `yaml:"subjectAltName" json:"subjectAltName"`
}

type OCPInstallerVars struct {
	RelativeDir               string `yaml:"relativeDir" json:"relativeDir"`
	RelativeInstallConfigPath string `yaml:"relativeInstallConfigPath" json:"relativeInstallConfigPath"`
	RelativeAgentConfigPath   string `yaml:"relativeAgentConfigPath" json:"relativeAgentConfigPath"`
}

type OCPClusterNodeVars struct {
	Name       string             `yaml:"name" json:"name"`
	MachineRef string             `yaml:"machineRef" json:"machineRef"`
	Role       string             `yaml:"role" json:"role"`
	HostRef    string             `yaml:"hostRef,omitempty" json:"hostRef,omitempty"`
	IPAddress  string             `yaml:"ipAddress,omitempty" json:"ipAddress,omitempty"`
	MACAddress string             `yaml:"macAddress,omitempty" json:"macAddress,omitempty"`
	BareMetal  *MachineBMCVars    `yaml:"bareMetal,omitempty" json:"bareMetal,omitempty"`
}

// MachineBMCVars carries one machine's out-of-band BMC endpoint for the
// bare-metal substrate. Populated only when the provider kind is baremetal;
// consumed by cluster_substrate_baremetal (credential staging) and
// hub_boot_redfish (per-host Redfish endpoints once it grows beyond the
// emulated loopback).
type MachineBMCVars struct {
	Address                        string `yaml:"address" json:"address"`
	Port                           int    `yaml:"port,omitempty" json:"port,omitempty"`
	Protocol                       string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	CredentialRef                  string `yaml:"credentialRef,omitempty" json:"credentialRef,omitempty"`
	DisableCertificateVerification bool   `yaml:"disableCertificateVerification,omitempty" json:"disableCertificateVerification,omitempty"`
	BootMACAddress                 string `yaml:"bootMACAddress,omitempty" json:"bootMACAddress,omitempty"`
}

// ProviderVars projects the provider chosen by a ClusterInfrastructure onto
// the per-cluster Ansible vars surface. Kind is the structural discriminator
// (mirrors v1alpha1.ProviderKind). SubstrateRole, BmcRole, and
// BootArtifactsHttp drive the dynamic role-name dispatch in the playbooks:
// roles are selected as `cluster_substrate_<SubstrateRole>` and
// `provider_bmc_<BmcRole>`, and the boot-artifacts HTTP role is gated on
// BootArtifactsHttp.Enabled.
type ProviderVars struct {
	Kind                string                        `yaml:"kind" json:"kind"`
	SubstrateRole       string                        `yaml:"substrateRole" json:"substrateRole"`
	BmcRole             string                        `yaml:"bmcRole" json:"bmcRole"`
	BootArtifactsHttp   ProviderBootArtifactsHTTPVars `yaml:"bootArtifactsHttp" json:"bootArtifactsHttp"`
	InfrastructureHosts []ProviderHostVars            `yaml:"infrastructureHosts,omitempty" json:"infrastructureHosts,omitempty"`
	Virtualization      *ProviderVirtualizationVars   `yaml:"virtualization,omitempty" json:"virtualization,omitempty"`
	BMC                 *ProviderBMCVars              `yaml:"bmc,omitempty" json:"bmc,omitempty"`
	Nodes               []ProviderNodeVars            `yaml:"nodes,omitempty" json:"nodes,omitempty"`
}

// ProviderComponentVars is the provider-scoped twin of ProviderVars consumed
// by the provider play in infra-prepare.yml. Same dispatch fields, different
// scope (one entry per InfrastructureProvider, not per cluster).
type ProviderComponentVars struct {
	Name                string                        `yaml:"name" json:"name"`
	Kind                string                        `yaml:"kind" json:"kind"`
	SubstrateRole       string                        `yaml:"substrateRole" json:"substrateRole"`
	BmcRole             string                        `yaml:"bmcRole" json:"bmcRole"`
	BootArtifactsHttp   ProviderBootArtifactsHTTPVars `yaml:"bootArtifactsHttp" json:"bootArtifactsHttp"`
	InfrastructureHosts []ProviderHostVars            `yaml:"infrastructureHosts,omitempty" json:"infrastructureHosts,omitempty"`
	BMC                 *ProviderBMCVars              `yaml:"bmc,omitempty" json:"bmc,omitempty"`
}

// ProviderBootArtifactsHTTPVars carries the inputs of the
// provider_boot_artifacts_http role: the substrate-neutral HTTP server that
// publishes RHCOS rootfs and other boot artifacts to nodes that can't fetch
// them out-of-band. Disabled for substrates that mount media via their own
// API (vSphere, KubeVirt).
type ProviderBootArtifactsHTTPVars struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	BindAddress string `yaml:"bindAddress,omitempty" json:"bindAddress,omitempty"`
	Port        int    `yaml:"port,omitempty" json:"port,omitempty"`
}

type ProviderHostVars struct {
	Name         string   `yaml:"name" json:"name"`
	Address      string   `yaml:"address" json:"address"`
	User         string   `yaml:"user,omitempty" json:"user,omitempty"`
	SSHKeyRef    string   `yaml:"sshKeyRef" json:"sshKeyRef"`
	LibvirtURI   string   `yaml:"libvirtURI,omitempty" json:"libvirtURI,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

type ProviderVirtualizationVars struct {
	Type        string                  `yaml:"type" json:"type"`
	Libvirt     *LibvirtVars            `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	DefaultNode VirtualNodeResourceVars `yaml:"defaultNode" json:"defaultNode"`
}

type VirtualNodeResourceVars struct {
	CPU       int `yaml:"cpu" json:"cpu"`
	MemoryMiB int `yaml:"memoryMiB" json:"memoryMiB"`
	DiskGiB   int `yaml:"diskGiB" json:"diskGiB"`
}

type LibvirtVars struct {
	Network  string           `yaml:"network" json:"network"`
	Bridge   string           `yaml:"bridge" json:"bridge"`
	DNSHosts []LibvirtDNSHost `yaml:"dnsHosts,omitempty" json:"dnsHosts,omitempty"`
}

type LibvirtDNSHost struct {
	IP        string   `yaml:"ip" json:"ip"`
	Hostnames []string `yaml:"hostnames" json:"hostnames"`
}

type ProviderBMCVars struct {
	Enabled     bool                  `yaml:"enabled" json:"enabled"`
	Protocol    string                `yaml:"protocol" json:"protocol"`
	Emulator    string                `yaml:"emulator" json:"emulator"`
	BindAddress string                `yaml:"bindAddress" json:"bindAddress"`
	Port        int                   `yaml:"port" json:"port"`
	Auth        *ProviderBMCAuthVars  `yaml:"auth,omitempty" json:"auth,omitempty"`
	Nodes       []ProviderBMCNodeVars `yaml:"nodes,omitempty" json:"nodes,omitempty"`
}

type ProviderBMCAuthVars struct {
	CredentialRef string `yaml:"credentialRef" json:"credentialRef"`
}

type ProviderBMCNodeVars struct {
	ClusterName string `yaml:"clusterName" json:"clusterName"`
	Name        string `yaml:"name" json:"name"`
	HostRef     string `yaml:"hostRef,omitempty" json:"hostRef,omitempty"`
	IPAddress   string `yaml:"ipAddress" json:"ipAddress"`
	MACAddress  string `yaml:"macAddress,omitempty" json:"macAddress,omitempty"`
}

type ProviderNodeVars struct {
	Name       string                  `yaml:"name" json:"name"`
	Role       string                  `yaml:"role" json:"role"`
	HostRef    string                  `yaml:"hostRef,omitempty" json:"hostRef,omitempty"`
	IPAddress  string                  `yaml:"ipAddress" json:"ipAddress"`
	MACAddress string                  `yaml:"macAddress,omitempty" json:"macAddress,omitempty"`
	Resources  VirtualNodeResourceVars `yaml:"resources" json:"resources"`
}

type ClusterNetworkVars struct {
	MachineNetworks []MachineNetworkVars    `yaml:"machineNetworks" json:"machineNetworks"`
	Endpoints       NetworkEndpointVars     `yaml:"endpoints" json:"endpoints"`
	LoadBalancer    ClusterLoadBalancerVars `yaml:"loadBalancer" json:"loadBalancer"`
	NameResolution  NameResolutionVars      `yaml:"nameResolution" json:"nameResolution"`
	APIVIP          string                  `yaml:"apiVIP" json:"apiVIP"`
	IngressVIP      string                  `yaml:"ingressVIP" json:"ingressVIP"`
	Records         DNSRecords              `yaml:"records" json:"records"`
}

type MachineNetworkVars struct {
	Name       string                     `yaml:"name" json:"name"`
	CIDR       string                     `yaml:"cidr" json:"cidr"`
	Gateway    string                     `yaml:"gateway,omitempty" json:"gateway,omitempty"`
	DNSServers []string                   `yaml:"dnsServers,omitempty" json:"dnsServers,omitempty"`
	Libvirt    *MachineNetworkLibvirtVars `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	VMware     *MachineNetworkVMwareVars  `yaml:"vmware,omitempty" json:"vmware,omitempty"`
}

type MachineNetworkLibvirtVars struct {
	LibvirtNetwork string `yaml:"libvirtNetwork" json:"libvirtNetwork"`
	Bridge         string `yaml:"bridge" json:"bridge"`
}

type MachineNetworkVMwareVars struct {
	Portgroup string `yaml:"portgroup,omitempty" json:"portgroup,omitempty"`
}

type NetworkEndpointVars struct {
	API     EndpointVar `yaml:"api,omitempty" json:"api,omitempty"`
	APIInt  EndpointVar `yaml:"apiInt,omitempty" json:"apiInt,omitempty"`
	Ingress EndpointVar `yaml:"ingress,omitempty" json:"ingress,omitempty"`
}

type EndpointVar struct {
	Hostname string `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Address  string `yaml:"address,omitempty" json:"address,omitempty"`
}

type DNSRecords struct {
	API     string `yaml:"api" json:"api"`
	APIInt  string `yaml:"apiInt" json:"apiInt"`
	Ingress string `yaml:"ingress" json:"ingress"`
}

type ClusterLoadBalancerVars struct {
	Mode     string            `yaml:"mode" json:"mode"`
	External bool              `yaml:"external" json:"external"`
	Refs     map[string]string `yaml:"refs,omitempty" json:"refs,omitempty"`
}

type SharedLoadBalancerVars struct {
	Name        string                           `yaml:"name" json:"name"`
	ClusterName string                           `yaml:"clusterName" json:"clusterName"`
	ProviderRef string                           `yaml:"providerRef" json:"providerRef"`
	Image       ComponentImageURLs               `yaml:"image" json:"image"`
	Runtime     string                           `yaml:"runtime" json:"runtime"`
	Placement   LoadBalancerPlacementVars        `yaml:"placement" json:"placement"`
	Frontends   []SharedLoadBalancerFrontendVars `yaml:"frontends" json:"frontends"`
}

// ComponentImageURLs exposes both image refs to Ansible. The role tries
// `local` first when set and falls back to `public` on pull failure.
type ComponentImageURLs struct {
	Local  string `yaml:"local,omitempty" json:"local,omitempty"`
	Public string `yaml:"public,omitempty" json:"public,omitempty"`
}

type LoadBalancerPlacementVars struct {
	ProviderHostRef string `yaml:"providerHostRef" json:"providerHostRef"`
}

type SharedLoadBalancerFrontendVars struct {
	Name        string                    `yaml:"name" json:"name"`
	EndpointRef string                    `yaml:"endpointRef" json:"endpointRef"`
	Ports       []LoadBalancerPortVars    `yaml:"ports" json:"ports"`
	Backend     LoadBalancerBackendVars   `yaml:"backend" json:"backend"`
	Bindings    []LoadBalancerBindingVars `yaml:"bindings" json:"bindings"`
}

type LoadBalancerPortVars struct {
	ListenPort int `yaml:"listenPort" json:"listenPort"`
	TargetPort int `yaml:"targetPort" json:"targetPort"`
}

type LoadBalancerBackendVars struct {
	NodeRole string `yaml:"nodeRole,omitempty" json:"nodeRole,omitempty"`
}

type LoadBalancerBindingVars struct {
	ClusterName      string                           `yaml:"clusterName" json:"clusterName"`
	OCPName          string                           `yaml:"ocpName" json:"ocpName"`
	BindAddress      string                           `yaml:"bindAddress" json:"bindAddress"`
	BridgeAttachment *LoadBalancerVIPBridgeAttachment `yaml:"bridgeAttachment,omitempty" json:"bridgeAttachment,omitempty"`
	Backends         []LoadBalancerBackendNode        `yaml:"backends" json:"backends"`
}

// LoadBalancerVIPBridgeAttachment tells the network_lb_managed Ansible role
// where to plumb the VIP so the address actually answers ARP on the local
// network. Without this, ip_nonlocal_bind lets HAProxy bind the listener but
// no interface owns the IP, so neither the SNO node nor the host can reach
// the API endpoint.
type LoadBalancerVIPBridgeAttachment struct {
	Bridge       string `yaml:"bridge" json:"bridge"`
	PrefixLength int    `yaml:"prefixLength" json:"prefixLength"`
}

type LoadBalancerBackendNode struct {
	Name      string `yaml:"name" json:"name"`
	Address   string `yaml:"address" json:"address"`
	Role      string `yaml:"role" json:"role"`
	TargetRef string `yaml:"targetRef" json:"targetRef"`
}

type NameResolutionVars struct {
	Mode    string                     `yaml:"mode" json:"mode"`
	Managed *ManagedNameResolutionVars `yaml:"managed,omitempty" json:"managed,omitempty"`
}

type ManagedNameResolutionVars struct {
	ProviderHostRefs []string       `yaml:"providerHostRefs" json:"providerHostRefs"`
	HostsFile        *HostsFileVars `yaml:"hostsFile,omitempty" json:"hostsFile,omitempty"`
}

type HostsFileVars struct {
	AdditionalIngressHosts []string         `yaml:"additionalIngressHosts,omitempty" json:"additionalIngressHosts,omitempty"`
	Entries                []HostsFileEntry `yaml:"entries" json:"entries"`
}

type HostsFileEntry struct {
	Address string   `yaml:"address" json:"address"`
	Names   []string `yaml:"names" json:"names"`
}

func Vars(state v1alpha1.State) VarsFile {
	clusters := make([]ClusterVars, 0, len(state.ClusterInfrastructures))
	providers := providerIndex(state.InfrastructureProviders)
	ocpByInfra := ocpByInfrastructure(state.OCPClusters)
	env := primaryEnvironment(state)
	for _, item := range state.ClusterInfrastructures {
		provider := providers[item.Spec.ProviderRef.Name]
		ocp := ocpByInfra[item.Metadata.Name]
		clusters = append(clusters, clusterVars(item, provider, ocp, env))
	}
	return VarsFile{
		GitupsOCPInstall:    ocpInstallEnvVars(env),
		GitupsProviders:     providerComponentVars(state),
		GitupsLoadBalancers: sharedLoadBalancerVars(state, env),
		GitupsClusters:      clusters,
		GitupsComponentPins: ComponentPins(state),
	}
}

func ocpInstallEnvVars(env *v1alpha1.Environment) EnvironmentOCPInstallVars {
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
	if proxy := v1alpha1.OCPInstallProxyOf(*env); proxy != nil && proxyHasValue(proxy) {
		result.Proxy = &ProxyVars{
			HTTPProxy:      proxy.HTTPProxy,
			HTTPSProxy:     proxy.HTTPSProxy,
			NoProxy:        append([]string(nil), proxy.NoProxy...),
			CredentialsRef: proxy.CredentialsRef.Name,
		}
	}
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
	if registries == nil || registries.Mirror == nil {
		return result
	}
	result.Registry = &MirrorRegistryVars{
		URL:            registries.Mirror.URL,
		Host:           mirrorRegistryHostname(registries.Mirror.URL),
		CredentialsRef: registries.Mirror.CredentialsRef.Name,
	}
	return result
}

func proxyHasValue(p *v1alpha1.OCPInstallProxy) bool {
	return p != nil && (p.HTTPProxy != "" || p.HTTPSProxy != "" || len(p.NoProxy) > 0)
}

func clusterVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment) ClusterVars {
	return ClusterVars{
		Name: item.Metadata.Name,
		Role: ocp.Spec.Role,
		OCP: OCPClusterVars{
			Name:      ocp.Metadata.Name,
			Topology:  ocp.Spec.Topology,
			Release:   ocpReleaseVars(ocp),
			Install:   ocpInstallVars(ocp, env),
			Installer: ocpInstallerVars(ocp.Metadata.Name),
			Nodes:     ocpClusterNodes(item, ocp),
		},
		Provider: providerVars(item, provider, ocp, env),
		Network:  networkVars(item),
	}
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

func ocpInstallVars(ocp v1alpha1.OCPCluster, env *v1alpha1.Environment) OCPInstallVars {
	return OCPInstallVars{
		Method:                   ocp.Spec.Install.Method,
		BaseDomain:               ocp.Spec.Install.BaseDomain,
		PullSecretRef:            ocp.Spec.Install.PullSecretRef.Name,
		SSHKeyRef:                ocp.Spec.Install.SSHKeyRef.Name,
		ReleaseImageOverride:     releaseImageOverride(ocp),
		AdditionalTrustBundleRef: ocp.Spec.Install.AdditionalTrustBundleRef.Name,
		GeneratedSecrets:         generatedSecretVars(ocp.Spec.Install.GeneratedSecrets),
		LocalRegistry:            localRegistryVars(env, ocp),
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

// localRegistryVars projects the Environment ocpInstall registry mirror
// onto the per-cluster install vars so the Ansible bundle continues to read
// `gitups_clusters[].ocp.install.localRegistry.*`.
func localRegistryVars(env *v1alpha1.Environment, ocp v1alpha1.OCPCluster) *LocalRegistryVars {
	if env == nil {
		return nil
	}
	kind := v1alpha1.OCPInstallKind(*env)
	if kind == "" || kind == v1alpha1.OCPInstallKindConnected {
		return nil
	}
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	return &LocalRegistryVars{
		Registry: MirrorRegistryVars{
			URL:            registries.Mirror.URL,
			Host:           mirrorRegistryHostname(registries.Mirror.URL),
			CredentialsRef: registries.Mirror.CredentialsRef.Name,
			TrustBundleRef: ocp.Spec.Install.AdditionalTrustBundleRef.Name,
		},
	}
}

func mirrorRegistryHostname(url string) string {
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		return url[:idx]
	}
	return url
}

func generatedSecretVars(items []v1alpha1.GeneratedSecretSpec) []GeneratedSecretVars {
	result := make([]GeneratedSecretVars, 0, len(items))
	for _, item := range items {
		entry := GeneratedSecretVars{
			Name: item.Name,
			Type: item.Type,
		}
		if item.SelfSignedCertificate != nil {
			entry.SelfSignedCertificate = selfSignedCertificateVars(*item.SelfSignedCertificate)
		}
		result = append(result, entry)
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
	return OCPInstallerVars{
		RelativeDir:               dir,
		RelativeInstallConfigPath: dir + "/install-config.yaml",
		RelativeAgentConfigPath:   dir + "/agent-config.yaml",
	}
}

func installerRelativeDir(clusterName string) string {
	return "clusters/" + clusterName + "/installer"
}

func ocpClusterNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) []OCPClusterNodeVars {
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
			if machine.Baremetal != nil && machine.Baremetal.BMC != nil {
				entry.BareMetal = &MachineBMCVars{
					Address:                        machine.Baremetal.BMC.Address,
					Port:                           machine.Baremetal.BMC.Port,
					Protocol:                       machine.Baremetal.BMC.Protocol,
					CredentialRef:                  machine.Baremetal.BMC.CredentialRef.Name,
					DisableCertificateVerification: machine.Baremetal.BMC.DisableCertificateVerification,
					BootMACAddress:                 machine.Baremetal.BootMACAddress,
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

func providerVars(item v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, ocp v1alpha1.OCPCluster, env *v1alpha1.Environment) ProviderVars {
	var result ProviderVars
	result.Kind = v1alpha1.ProviderKind(provider)
	result.SubstrateRole, result.BmcRole, result.BootArtifactsHttp = providerDispatch(provider)
	if provider.Spec.QemuKVM == nil {
		return result
	}
	q := provider.Spec.QemuKVM
	result.InfrastructureHosts = providerHostVars(q)
	result.Virtualization = &ProviderVirtualizationVars{
		Type:        v1alpha1.VirtualizationTypeLibvirt,
		Libvirt:     libvirtVars(item, q, env),
		DefaultNode: defaultNodeVars(q),
	}
	if q.BMCEmulation != nil {
		result.BMC = bmcVars(q.BMCEmulation, nil)
	}
	result.Nodes = providerNodes(item, ocp)
	return result
}

// providerDispatch maps the structural provider discriminator onto the three
// fields the playbooks use to pick roles by name. Sources of truth:
//   - SubstrateRole → cluster_substrate_<role> (per-cluster substrate concerns
//     such as VM lifecycle, hardware credential staging).
//   - BmcRole → provider_bmc_<role> (provider-scoped BMC handling: emulated
//     sushy stack, real Redfish credentials, no-op for substrates with no
//     external power-control surface).
//   - BootArtifactsHttp → enables provider_boot_artifacts_http when the
//     substrate cannot deliver the agent rootfs / boot artifacts out-of-band.
func providerDispatch(provider v1alpha1.InfrastructureProvider) (string, string, ProviderBootArtifactsHTTPVars) {
	switch v1alpha1.ProviderKind(provider) {
	case v1alpha1.ProviderKindQemuKVM:
		http := ProviderBootArtifactsHTTPVars{}
		if provider.Spec.QemuKVM != nil && provider.Spec.QemuKVM.BMCEmulation != nil && provider.Spec.QemuKVM.BMCEmulation.Port > 0 {
			http = ProviderBootArtifactsHTTPVars{
				Enabled:     true,
				BindAddress: "0.0.0.0",
				Port:        provider.Spec.QemuKVM.BMCEmulation.Port + 2,
			}
		}
		return "libvirt", "emulated", http
	case v1alpha1.ProviderKindBareMetal:
		return "baremetal", "redfish", ProviderBootArtifactsHTTPVars{}
	case v1alpha1.ProviderKindVMware:
		return "vsphere", "none", ProviderBootArtifactsHTTPVars{}
	case v1alpha1.ProviderKindOpenShiftVirt:
		return "kubevirt", "none", ProviderBootArtifactsHTTPVars{}
	default:
		return "", "none", ProviderBootArtifactsHTTPVars{}
	}
}

func providerHostVars(q *v1alpha1.QemuKVMProviderSpec) []ProviderHostVars {
	names := sortedKeys(q.Hosts)
	out := make([]ProviderHostVars, 0, len(names))
	for _, name := range names {
		host := q.Hosts[name]
		out = append(out, ProviderHostVars{
			Name:         name,
			Address:      host.Address,
			User:         host.User,
			SSHKeyRef:    host.SSHKeyRef.Name,
			LibvirtURI:   host.LibvirtURI,
			Capabilities: append([]string(nil), host.Capabilities...),
		})
	}
	return out
}

func defaultNodeVars(q *v1alpha1.QemuKVMProviderSpec) VirtualNodeResourceVars {
	if len(q.MachineProfiles) == 0 {
		return VirtualNodeResourceVars{
			CPU:       v1alpha1.DefaultNodeCPU,
			MemoryMiB: v1alpha1.DefaultNodeMemoryMiB,
			DiskGiB:   v1alpha1.DefaultNodeDiskGiB,
		}
	}
	keys := sortedKeys(q.MachineProfiles)
	p := q.MachineProfiles[keys[0]]
	return VirtualNodeResourceVars{
		CPU:       p.CPU,
		MemoryMiB: p.MemoryMiB,
		DiskGiB:   p.DiskGiB,
	}
}

func providerComponentVars(state v1alpha1.State) []ProviderComponentVars {
	result := make([]ProviderComponentVars, 0, len(state.InfrastructureProviders))
	for _, provider := range state.InfrastructureProviders {
		substrateRole, bmcRole, http := providerDispatch(provider)
		item := ProviderComponentVars{
			Name:              provider.Metadata.Name,
			Kind:              v1alpha1.ProviderKind(provider),
			SubstrateRole:     substrateRole,
			BmcRole:           bmcRole,
			BootArtifactsHttp: http,
		}
		if provider.Spec.QemuKVM != nil {
			item.InfrastructureHosts = providerHostVars(provider.Spec.QemuKVM)
			if provider.Spec.QemuKVM.BMCEmulation != nil {
				item.BMC = bmcVars(provider.Spec.QemuKVM.BMCEmulation, providerBMCNodes(provider, state))
			}
		}
		result = append(result, item)
	}
	return result
}

func bmcVars(source *v1alpha1.BMCEmulationSpec, nodes []ProviderBMCNodeVars) *ProviderBMCVars {
	enabled := false
	if source.Enabled != nil {
		enabled = *source.Enabled
	}
	result := &ProviderBMCVars{
		Enabled:     enabled,
		Protocol:    source.Protocol,
		Emulator:    source.Emulator,
		BindAddress: source.BindAddress,
		Port:        source.Port,
		Nodes:       nodes,
	}
	if source.Auth != nil && source.Auth.CredentialRef.Name != "" {
		result.Auth = &ProviderBMCAuthVars{CredentialRef: source.Auth.CredentialRef.Name}
	}
	return result
}

func providerBMCNodes(provider v1alpha1.InfrastructureProvider, state v1alpha1.State) []ProviderBMCNodeVars {
	if provider.Spec.QemuKVM == nil {
		return nil
	}
	var result []ProviderBMCNodeVars
	for _, infra := range state.ClusterInfrastructures {
		if infra.Spec.ProviderRef.Name != provider.Metadata.Name {
			continue
		}
		machineNames := sortedKeys(infra.Spec.Machines)
		for _, mname := range machineNames {
			machine := infra.Spec.Machines[mname]
			primary := primaryInterface(machine)
			hostRef := ""
			if machine.Libvirt != nil {
				hostRef = machine.Libvirt.HostRef.Name
			}
			result = append(result, ProviderBMCNodeVars{
				ClusterName: infra.Metadata.Name,
				Name:        mname,
				HostRef:     hostRef,
				IPAddress:   primary.IPAddress,
				MACAddress:  primary.MACAddress,
			})
		}
	}
	return result
}

func libvirtVars(item v1alpha1.ClusterInfrastructure, q *v1alpha1.QemuKVMProviderSpec, env *v1alpha1.Environment) *LibvirtVars {
	if len(item.Spec.Networks) == 0 || q == nil {
		return nil
	}
	machineNetworks := machineNetworksList(item)
	if len(machineNetworks) == 0 {
		return nil
	}
	machineNetwork := machineNetworks[0]
	if machineNetwork.Libvirt == nil {
		return nil
	}
	return &LibvirtVars{
		Network:  machineNetwork.Libvirt.LibvirtNetwork,
		Bridge:   machineNetwork.Libvirt.Bridge,
		DNSHosts: libvirtDNSHosts(machineNetwork, env),
	}
}

func libvirtDNSHosts(machineNetwork MachineNetworkVars, env *v1alpha1.Environment) []LibvirtDNSHost {
	if env == nil || machineNetwork.Gateway == "" {
		return nil
	}
	kind := v1alpha1.OCPInstallKind(*env)
	if kind == "" || kind == v1alpha1.OCPInstallKindConnected {
		return nil
	}
	registries := v1alpha1.OCPInstallRegistriesOf(*env)
	if registries == nil || registries.Mirror == nil {
		return nil
	}
	host := mirrorRegistryHostname(registries.Mirror.URL)
	if host == "" {
		return nil
	}
	return []LibvirtDNSHost{{
		IP:        machineNetwork.Gateway,
		Hostnames: []string{host},
	}}
}

func providerNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster) []ProviderNodeVars {
	roles := nodeRolesByMachineRef(ocp)
	machineNames := sortedKeys(item.Spec.Machines)
	out := make([]ProviderNodeVars, 0, len(machineNames))
	for _, name := range machineNames {
		machine := item.Spec.Machines[name]
		primary := primaryInterface(machine)
		hostRef := ""
		if machine.Libvirt != nil {
			hostRef = machine.Libvirt.HostRef.Name
		}
		var resources VirtualNodeResourceVars
		if machine.Resources != nil {
			resources = VirtualNodeResourceVars{
				CPU:       machine.Resources.CPU,
				MemoryMiB: machine.Resources.MemoryMiB,
				DiskGiB:   machine.Resources.DiskGiB,
			}
		}
		out = append(out, ProviderNodeVars{
			Name:       name,
			Role:       roles[name],
			HostRef:    hostRef,
			IPAddress:  primary.IPAddress,
			MACAddress: primary.MACAddress,
			Resources:  resources,
		})
	}
	return out
}

func nodeRolesByMachineRef(ocp v1alpha1.OCPCluster) map[string]string {
	out := map[string]string{}
	for nodeName, node := range ocp.Spec.Nodes {
		ref := nodeName
		if node.MachineRef != nil && node.MachineRef.Name != "" {
			ref = node.MachineRef.Name
		}
		out[ref] = node.Role
	}
	return out
}

func networkVars(item v1alpha1.ClusterInfrastructure) ClusterNetworkVars {
	endpoints := endpointVars(item.Spec.Endpoints)
	return ClusterNetworkVars{
		MachineNetworks: machineNetworksList(item),
		Endpoints:       endpoints,
		LoadBalancer:    clusterLoadBalancerVars(item),
		NameResolution:  nameResolutionVars(item),
		APIVIP:          endpoints.API.Address,
		IngressVIP:      endpoints.Ingress.Address,
		Records: DNSRecords{
			API:     endpoints.API.Hostname,
			APIInt:  endpoints.APIInt.Hostname,
			Ingress: endpoints.Ingress.Hostname,
		},
	}
}

func endpointVars(spec v1alpha1.ClusterEndpointsSpec) NetworkEndpointVars {
	out := NetworkEndpointVars{}
	if spec.API != nil {
		out.API = EndpointVar{Hostname: spec.API.Hostname, Address: spec.API.Address}
	}
	if spec.APIInt != nil {
		out.APIInt = EndpointVar{Hostname: spec.APIInt.Hostname, Address: spec.APIInt.Address}
	}
	if spec.Ingress != nil {
		out.Ingress = EndpointVar{Hostname: spec.Ingress.Hostname, Address: spec.Ingress.Address}
	}
	return out
}

func machineNetworksList(item v1alpha1.ClusterInfrastructure) []MachineNetworkVars {
	names := sortedKeys(item.Spec.Networks)
	out := make([]MachineNetworkVars, 0, len(names))
	for _, name := range names {
		n := item.Spec.Networks[name]
		entry := MachineNetworkVars{
			Name:       name,
			CIDR:       n.CIDR,
			Gateway:    n.Gateway,
			DNSServers: append([]string(nil), n.DNSServers...),
		}
		if n.Libvirt != nil {
			entry.Libvirt = &MachineNetworkLibvirtVars{
				LibvirtNetwork: libvirtNetworkName(item.Metadata.Name, name),
				Bridge:         n.Libvirt.Bridge,
			}
		}
		if n.VMware != nil {
			entry.VMware = &MachineNetworkVMwareVars{Portgroup: n.VMware.Portgroup}
		}
		out = append(out, entry)
	}
	return out
}

// libvirtNetworkName builds a deterministic libvirt network identifier from
// the ClusterInfrastructure name and the cluster-local network key. Multiple
// clusters can share a libvirt host, so the name has to be unique per cluster.
func libvirtNetworkName(clusterName, networkKey string) string {
	return clusterName + "-" + networkKey
}

func clusterLoadBalancerVars(item v1alpha1.ClusterInfrastructure) ClusterLoadBalancerVars {
	if len(item.Spec.LoadBalancers) == 0 {
		return ClusterLoadBalancerVars{Mode: "external", External: true}
	}
	refs := map[string]string{}
	for lbName, lb := range item.Spec.LoadBalancers {
		for _, ep := range lb.Endpoints {
			refs[ep] = lbName
		}
	}
	external := false
	for _, ep := range []string{v1alpha1.EndpointAPI, v1alpha1.EndpointAPIInt, v1alpha1.EndpointIngress} {
		if _, ok := refs[ep]; !ok {
			external = true
			break
		}
	}
	return ClusterLoadBalancerVars{
		Mode:     "managed",
		External: external,
		Refs:     refs,
	}
}

func sharedLoadBalancerVars(state v1alpha1.State, env *v1alpha1.Environment) []SharedLoadBalancerVars {
	imageRef := componentImageURLs(env, v1alpha1.ComponentCategoryLoadBalancer, v1alpha1.ComponentTypeHAProxy)
	ocpByInfra := ocpByInfrastructure(state.OCPClusters)
	providers := providerIndex(state.InfrastructureProviders)
	var result []SharedLoadBalancerVars
	for _, infra := range state.ClusterInfrastructures {
		ocp := ocpByInfra[infra.Metadata.Name]
		provider := providers[infra.Spec.ProviderRef.Name]
		lbNames := sortedKeys(infra.Spec.LoadBalancers)
		for _, lbName := range lbNames {
			lb := infra.Spec.LoadBalancers[lbName]
			item := SharedLoadBalancerVars{
				Name:        lbName,
				ClusterName: infra.Metadata.Name,
				ProviderRef: infra.Spec.ProviderRef.Name,
				Image:       imageRef,
				Runtime:     v1alpha1.ContainerRuntimePodman,
				Placement:   LoadBalancerPlacementVars{ProviderHostRef: lb.Placement.ProviderHostRef.Name},
				Frontends:   make([]SharedLoadBalancerFrontendVars, 0, len(lb.Endpoints)),
			}
			for _, ep := range lb.Endpoints {
				item.Frontends = append(item.Frontends, frontendForEndpoint(infra, ocp, provider, lbName, ep))
			}
			result = append(result, item)
		}
	}
	return result
}

func frontendForEndpoint(infra v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, provider v1alpha1.InfrastructureProvider, lbName, ep string) SharedLoadBalancerFrontendVars {
	ports := []LoadBalancerPortVars{}
	for _, p := range v1alpha1.StandardLoadBalancerPorts(ep) {
		ports = append(ports, LoadBalancerPortVars{ListenPort: p[0], TargetPort: p[1]})
	}
	frontend := SharedLoadBalancerFrontendVars{
		Name:        ep,
		EndpointRef: ep,
		Ports:       ports,
		Backend:     LoadBalancerBackendVars{NodeRole: v1alpha1.StandardEndpointBackendRole(ep)},
	}
	bindAddress := endpointAddressFromCI(infra, ep)
	frontend.Bindings = append(frontend.Bindings, LoadBalancerBindingVars{
		ClusterName:      infra.Metadata.Name,
		OCPName:          ocp.Metadata.Name,
		BindAddress:      bindAddress,
		BridgeAttachment: vipBridgeAttachment(infra, provider, bindAddress),
		Backends:         backendNodes(infra, ocp, frontend.Backend.NodeRole),
	})
	_ = lbName
	return frontend
}

// vipBridgeAttachment resolves the local interface that must own a managed VIP
// so the address answers ARP. Returns nil when the cluster's provider does not
// expose a host-local bridge (e.g. baremetal, vSphere) — in those cases the
// upstream network owns address plumbing and the VIP is reached out-of-band.
func vipBridgeAttachment(infra v1alpha1.ClusterInfrastructure, provider v1alpha1.InfrastructureProvider, bindAddress string) *LoadBalancerVIPBridgeAttachment {
	if v1alpha1.ProviderKind(provider) != v1alpha1.ProviderKindQemuKVM {
		return nil
	}
	if bindAddress == "" {
		return nil
	}
	addr, err := netip.ParseAddr(bindAddress)
	if err != nil {
		return nil
	}
	for _, name := range sortedKeys(infra.Spec.Networks) {
		net := infra.Spec.Networks[name]
		if net.Libvirt == nil || net.Libvirt.Bridge == "" || net.CIDR == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(net.CIDR)
		if err != nil {
			continue
		}
		if !prefix.Contains(addr) {
			continue
		}
		return &LoadBalancerVIPBridgeAttachment{
			Bridge:       net.Libvirt.Bridge,
			PrefixLength: prefix.Bits(),
		}
	}
	return nil
}

func endpointAddressFromCI(infra v1alpha1.ClusterInfrastructure, ep string) string {
	switch ep {
	case v1alpha1.EndpointAPI:
		if infra.Spec.Endpoints.API != nil {
			return infra.Spec.Endpoints.API.Address
		}
	case v1alpha1.EndpointAPIInt:
		if infra.Spec.Endpoints.APIInt != nil {
			return infra.Spec.Endpoints.APIInt.Address
		}
	case v1alpha1.EndpointIngress:
		if infra.Spec.Endpoints.Ingress != nil {
			return infra.Spec.Endpoints.Ingress.Address
		}
	}
	return ""
}

func nameResolutionVars(item v1alpha1.ClusterInfrastructure) NameResolutionVars {
	if item.Spec.NameResolution.Managed == nil {
		return NameResolutionVars{Mode: "external"}
	}
	managed := item.Spec.NameResolution.Managed
	refs := make([]string, 0, len(managed.ProviderHostRefs))
	for _, r := range managed.ProviderHostRefs {
		refs = append(refs, r.Name)
	}
	result := NameResolutionVars{
		Mode: "managed",
		Managed: &ManagedNameResolutionVars{
			ProviderHostRefs: refs,
		},
	}
	result.Managed.HostsFile = &HostsFileVars{
		AdditionalIngressHosts: append([]string(nil), managed.AdditionalIngressHosts...),
		Entries:                hostsFileEntries(item, managed.AdditionalIngressHosts),
	}
	return result
}

func hostsFileEntries(item v1alpha1.ClusterInfrastructure, additionalIngress []string) []HostsFileEntry {
	entries := []HostsFileEntry{}
	if item.Spec.Endpoints.API != nil {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.API.Address,
			Names:   []string{item.Spec.Endpoints.API.Hostname},
		})
	}
	if item.Spec.Endpoints.APIInt != nil {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.APIInt.Address,
			Names:   []string{item.Spec.Endpoints.APIInt.Hostname},
		})
	}
	if item.Spec.Endpoints.Ingress != nil && len(additionalIngress) > 0 {
		entries = append(entries, HostsFileEntry{
			Address: item.Spec.Endpoints.Ingress.Address,
			Names:   append([]string(nil), additionalIngress...),
		})
	}
	return entries
}

func backendNodes(item v1alpha1.ClusterInfrastructure, ocp v1alpha1.OCPCluster, role string) []LoadBalancerBackendNode {
	roleByMachineRef := nodeRolesByMachineRef(ocp)
	var workerRefs, controlPlaneRefs []string
	for ref, r := range roleByMachineRef {
		switch r {
		case v1alpha1.NodeRoleWorker:
			workerRefs = append(workerRefs, ref)
		case v1alpha1.NodeRoleControlPlane:
			controlPlaneRefs = append(controlPlaneRefs, ref)
		}
	}
	sort.Strings(workerRefs)
	sort.Strings(controlPlaneRefs)
	var selected []string
	switch role {
	case v1alpha1.NodeRoleWorker:
		selected = workerRefs
	case v1alpha1.NodeRoleControlPlane:
		selected = controlPlaneRefs
	default:
		if len(workerRefs) > 0 {
			selected = workerRefs
		} else {
			selected = controlPlaneRefs
		}
	}
	selectedSet := map[string]bool{}
	for _, ref := range selected {
		selectedSet[ref] = true
	}
	machineNames := sortedKeys(item.Spec.Machines)
	var out []LoadBalancerBackendNode
	for _, name := range machineNames {
		if !selectedSet[name] {
			continue
		}
		machine := item.Spec.Machines[name]
		out = append(out, LoadBalancerBackendNode{
			Name:      name,
			Address:   primaryInterface(machine).IPAddress,
			Role:      roleByMachineRef[name],
			TargetRef: name,
		})
	}
	return out
}

func primaryInterface(machine v1alpha1.MachineSpec) v1alpha1.MachineInterfaceSpec {
	if len(machine.Interfaces) == 0 {
		return v1alpha1.MachineInterfaceSpec{}
	}
	for _, iface := range machine.Interfaces {
		if iface.Primary != nil && *iface.Primary {
			return iface
		}
	}
	keys := sortedKeys(machine.Interfaces)
	return machine.Interfaces[keys[0]]
}

func providerIndex(items []v1alpha1.InfrastructureProvider) map[string]v1alpha1.InfrastructureProvider {
	result := map[string]v1alpha1.InfrastructureProvider{}
	for _, item := range items {
		result[item.Metadata.Name] = item
	}
	return result
}

func ocpByInfrastructure(items []v1alpha1.OCPCluster) map[string]v1alpha1.OCPCluster {
	result := map[string]v1alpha1.OCPCluster{}
	for _, item := range items {
		result[item.Spec.InfrastructureRef.Name] = item
	}
	return result
}

func primaryEnvironment(state v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

// componentImageURLs returns both image refs for a component declared under
// `Environment.spec.componentImages.<category>.<type>`. The Ansible role
// chooses at apply time: it pulls `local` first when set and falls back to
// `public` on failure. When the user declared neither, Gitups supplies a
// built-in default for known components (haproxy → DefaultHAProxyImageRef as
// `public`).
func componentImageURLs(env *v1alpha1.Environment, category, typ string) ComponentImageURLs {
	if env != nil {
		if types, ok := env.Spec.ComponentImages[category]; ok {
			if image, ok := types[typ]; ok {
				return ComponentImageURLs{Local: image.Local, Public: image.Public}
			}
		}
	}
	if category == v1alpha1.ComponentCategoryLoadBalancer && typ == v1alpha1.ComponentTypeHAProxy {
		return ComponentImageURLs{Public: v1alpha1.DefaultHAProxyImageRef}
	}
	return ComponentImageURLs{}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
