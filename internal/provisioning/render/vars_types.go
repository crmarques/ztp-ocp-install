package render

type VarsFile struct {
	GitupsOCPInstall       EnvironmentOCPInstallVars `yaml:"gitups_ocp_install" json:"gitups_ocp_install"`
	GitupsProviders        []ProviderComponentVars   `yaml:"gitups_providers" json:"gitups_providers"`
	GitupsLoadBalancers    []SharedLoadBalancerVars  `yaml:"gitups_load_balancers" json:"gitups_load_balancers"`
	GitupsMirrorRegistries []MirrorRegistryRunVars   `yaml:"gitups_mirror_registries" json:"gitups_mirror_registries"`
	GitupsForwardProxies   []ForwardProxyRunVars     `yaml:"gitups_forward_proxies,omitempty" json:"gitups_forward_proxies,omitempty"`
	GitupsClusters         []ClusterVars             `yaml:"gitups_clusters" json:"gitups_clusters"`
	GitupsComponentPins    []ComponentPin            `yaml:"gitups_component_pins" json:"gitups_component_pins"`
}

type EnvironmentOCPInstallVars struct {
	Mode         string              `yaml:"mode" json:"mode"`
	Disconnected bool                `yaml:"disconnected" json:"disconnected"`
	Registry     *MirrorRegistryVars `yaml:"registry,omitempty" json:"registry,omitempty"`
	Proxy        *ProxyVars          `yaml:"proxy,omitempty" json:"proxy,omitempty"`
}

type ProxyVars struct {
	HTTP         string   `yaml:"http,omitempty" json:"http,omitempty"`
	HTTPS        string   `yaml:"https,omitempty" json:"https,omitempty"`
	VMHTTP       string   `yaml:"vmHttp,omitempty" json:"vmHttp,omitempty"`
	VMHTTPS      string   `yaml:"vmHttps,omitempty" json:"vmHttps,omitempty"`
	NoProxy      []string `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
	ProxyAuthRef string   `yaml:"proxyAuthRef,omitempty" json:"proxyAuthRef,omitempty"`
}

type ClusterVars struct {
	Name     string             `yaml:"name" json:"name"`
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
	Path                  string                     `yaml:"path" json:"path"`
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
	RelativeWorkDir           string `yaml:"relativeWorkDir" json:"relativeWorkDir"`
}

type OCPClusterNodeVars struct {
	Name       string          `yaml:"name" json:"name"`
	MachineRef string          `yaml:"machineRef" json:"machineRef"`
	Role       string          `yaml:"role" json:"role"`
	HostRef    string          `yaml:"hostRef,omitempty" json:"hostRef,omitempty"`
	IPAddress  string          `yaml:"ipAddress,omitempty" json:"ipAddress,omitempty"`
	MACAddress string          `yaml:"macAddress,omitempty" json:"macAddress,omitempty"`
	BareMetal  *MachineBMCVars `yaml:"bareMetal,omitempty" json:"bareMetal,omitempty"`
}

type MachineBMCVars struct {
	Address                        string `yaml:"address" json:"address"`
	Port                           int    `yaml:"port,omitempty" json:"port,omitempty"`
	Protocol                       string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	CredentialRef                  string `yaml:"credentialRef,omitempty" json:"credentialRef,omitempty"`
	DisableCertificateVerification bool   `yaml:"disableCertificateVerification,omitempty" json:"disableCertificateVerification,omitempty"`
	BootMACAddress                 string `yaml:"bootMACAddress,omitempty" json:"bootMACAddress,omitempty"`
}

type ProviderVars struct {
	Kind                string                        `yaml:"kind" json:"kind"`
	SubstrateRole       string                        `yaml:"substrateRole" json:"substrateRole"`
	BMCRole             string                        `yaml:"bmcRole" json:"bmcRole"`
	BootArtifactsHttp   ProviderBootArtifactsHTTPVars `yaml:"bootArtifactsHttp" json:"bootArtifactsHttp"`
	InfrastructureHosts []ProviderHostVars            `yaml:"infrastructureHosts,omitempty" json:"infrastructureHosts,omitempty"`
	Virtualization      *ProviderVirtualizationVars   `yaml:"virtualization,omitempty" json:"virtualization,omitempty"`
	BMC                 *ProviderBMCVars              `yaml:"bmc,omitempty" json:"bmc,omitempty"`
	Nodes               []ProviderNodeVars            `yaml:"nodes,omitempty" json:"nodes,omitempty"`
}

type ProviderComponentVars struct {
	Name                string                        `yaml:"name" json:"name"`
	Kind                string                        `yaml:"kind" json:"kind"`
	SubstrateRole       string                        `yaml:"substrateRole" json:"substrateRole"`
	BMCRole             string                        `yaml:"bmcRole" json:"bmcRole"`
	BootArtifactsHttp   ProviderBootArtifactsHTTPVars `yaml:"bootArtifactsHttp" json:"bootArtifactsHttp"`
	InfrastructureHosts []ProviderHostVars            `yaml:"infrastructureHosts,omitempty" json:"infrastructureHosts,omitempty"`
	BMC                 *ProviderBMCVars              `yaml:"bmc,omitempty" json:"bmc,omitempty"`
}

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
	Network                 string           `yaml:"network" json:"network"`
	Bridge                  string           `yaml:"bridge" json:"bridge"`
	EgressRestrictedToProxy bool             `yaml:"egressRestrictedToProxy,omitempty" json:"egressRestrictedToProxy,omitempty"`
	ProxyURL                string           `yaml:"proxyURL,omitempty" json:"proxyURL,omitempty"`
	ProxyPort               int              `yaml:"proxyPort,omitempty" json:"proxyPort,omitempty"`
	DNSHosts                []LibvirtDNSHost `yaml:"dnsHosts,omitempty" json:"dnsHosts,omitempty"`
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
	VSphere    *MachineNetworkVSphereVars `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
}

type MachineNetworkLibvirtVars struct {
	LibvirtNetwork string `yaml:"libvirtNetwork" json:"libvirtNetwork"`
	Bridge         string `yaml:"bridge" json:"bridge"`
}

type MachineNetworkVSphereVars struct {
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

type MirrorRegistryRunVars struct {
	Name                      string             `yaml:"name" json:"name"`
	ProviderRef               string             `yaml:"providerRef" json:"providerRef"`
	ProviderHostRef           string             `yaml:"providerHostRef" json:"providerHostRef"`
	URL                       string             `yaml:"url" json:"url"`
	Host                      string             `yaml:"host" json:"host"`
	Port                      int                `yaml:"port" json:"port"`
	DataDir                   string             `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
	Runtime                   string             `yaml:"runtime" json:"runtime"`
	CredentialsSecretName     string             `yaml:"credentialsSecretName,omitempty" json:"credentialsSecretName,omitempty"`
	TrustBundleCertSecretName string             `yaml:"trustBundleCertSecretName,omitempty" json:"trustBundleCertSecretName,omitempty"`
	TrustBundleKeySecretName  string             `yaml:"trustBundleKeySecretName,omitempty" json:"trustBundleKeySecretName,omitempty"`
	Image                     ComponentImageURLs `yaml:"image" json:"image"`
	MirrorSet                 []MirrorImageRef   `yaml:"mirrorSet" json:"mirrorSet"`
}

type ForwardProxyRunVars struct {
	Name                  string             `yaml:"name" json:"name"`
	ProviderRef           string             `yaml:"providerRef" json:"providerRef"`
	ProviderHostRef       string             `yaml:"providerHostRef" json:"providerHostRef"`
	URL                   string             `yaml:"url" json:"url"`
	Host                  string             `yaml:"host" json:"host"`
	Port                  int                `yaml:"port" json:"port"`
	DataDir               string             `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
	Runtime               string             `yaml:"runtime" json:"runtime"`
	CredentialsSecretName string             `yaml:"credentialsSecretName" json:"credentialsSecretName"`
	Image                 ComponentImageURLs `yaml:"image" json:"image"`
}

type MirrorImageRef struct {
	Kind   string `yaml:"kind" json:"kind"`
	Public string `yaml:"public" json:"public"`
	Local  string `yaml:"local" json:"local"`
}

const (
	MirrorImageKindReleasePayload = "releasePayload"
	MirrorImageKindComponent      = "componentImage"
	MirrorImageKindRegistryServer = "registryServer"
)

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
	ClusterName string                    `yaml:"clusterName" json:"clusterName"`
	OCPName     string                    `yaml:"ocpName" json:"ocpName"`
	BindAddress string                    `yaml:"bindAddress" json:"bindAddress"`
	Backends    []LoadBalancerBackendNode `yaml:"backends" json:"backends"`
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
