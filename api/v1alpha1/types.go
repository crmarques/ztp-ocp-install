// Package v1alpha1 defines the user-authored desired-state API. The schema is
// the four-domain-layer model declared in ADR 0001: Environment,
// InfrastructureProvider, ClusterInfrastructure, OCPCluster.
package v1alpha1

const (
	APIVersion = "gitups.io/v1alpha1"

	KindEnvironment            = "Environment"
	KindInfrastructureProvider = "InfrastructureProvider"
	KindClusterInfrastructure  = "ClusterInfrastructure"
	KindOCPCluster             = "OCPCluster"
	KindInfrastructureState    = "InfrastructureState"
	KindGitupsLock             = "GitupsLock"

	ProviderKindQemuKVM       = "qemu-kvm"
	ProviderKindBareMetal     = "baremetal"
	ProviderKindVMware        = "vmware"
	ProviderKindOpenShiftVirt = "openshift-virtualization"

	OCPInstallKindConnected    = "connected"
	OCPInstallKindRestricted   = "restricted"
	OCPInstallKindDisconnected = "disconnected"

	NameResolutionKindManaged  = "managed"
	NameResolutionKindExternal = "external"

	OCPRoleHub                = "hub"
	OCPRoleManaged            = "managed"
	OCPTopologySingleNode     = "single-node"
	OCPTopologyMultiNode      = "multi-node"
	OCPInstallMethodAgent     = "agent"
	NodeRoleControlPlane      = "control-plane"
	NodeRoleWorker            = "worker"
	GeneratedSecretSelfSigned = "self-signed-certificate"
	DefaultCertificateDays    = 3650
	ImageSourcePolicyNever    = "NeverContactSource"
	ImageSourcePolicyAllow    = "AllowContactingSource"

	OCPReleaseSourceQuayOCPRelease = "quay.io/openshift-release-dev/ocp-release"
	OCPReleaseSourceQuayARTDev     = "quay.io/openshift-release-dev/ocp-v4.0-art-dev"
	DefaultMirroredReleasePath     = "openshift/release-images"
	DefaultMirroredARTDevPath      = "openshift/release"

	DefaultHostUser               = "root"
	DefaultLibvirtBridge          = "virbr0"
	VirtualizationTypeLibvirt     = "libvirt"
	DefaultNodeCPU                = 8
	DefaultNodeMemoryMiB          = 22528
	DefaultNodeDiskGiB            = 120
	DefaultBMCEnabled             = true
	DefaultBMCProtocol            = "redfish"
	DefaultBMCEmulator            = "sushy-tools"
	DefaultBMCBindAddress         = "0.0.0.0"
	DefaultBMCPort                = 8000
	CapabilityLibvirt             = "libvirt"
	CapabilityContainerRuntime    = "container-runtime"
	CapabilityHostsFile           = "hosts-file"
	ComponentCategoryLoadBalancer = "load-balancer"
	ComponentTypeHAProxy          = "haproxy"
	ContainerRuntimePodman        = "podman"
	DefaultHAProxyImageRef        = "docker.io/library/haproxy:3.2.15"

	EndpointAPI     = "api"
	EndpointAPIInt  = "apiInt"
	EndpointIngress = "ingress"
)

// State is the in-memory container of all loaded resources.
type State struct {
	Environments            []Environment            `yaml:"environments,omitempty" json:"environments,omitempty"`
	InfrastructureProviders []InfrastructureProvider `yaml:"infrastructureProviders" json:"infrastructureProviders"`
	ClusterInfrastructures  []ClusterInfrastructure  `yaml:"clusterInfrastructures" json:"clusterInfrastructures"`
	OCPClusters             []OCPCluster             `yaml:"ocpClusters" json:"ocpClusters"`
}

type TypeMeta struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
}

type Metadata struct {
	Name string `yaml:"name" json:"name"`
}

// LocalObjectReference points at another v1alpha1 object by name.
type LocalObjectReference struct {
	Name string `yaml:"name" json:"name"`
}

// SecretRef points at install-time secret material by name.
type SecretRef struct {
	Name string `yaml:"name" json:"name"`
}

// ----- Environment -----

type Environment struct {
	APIVersion string          `yaml:"apiVersion" json:"apiVersion"`
	Kind       string          `yaml:"kind" json:"kind"`
	Metadata   Metadata        `yaml:"metadata" json:"metadata"`
	Spec       EnvironmentSpec `yaml:"spec" json:"spec"`
	SourcePath string          `yaml:"-" json:"-"`
}

type EnvironmentSpec struct {
	BaseDomain      string                                   `yaml:"baseDomain,omitempty" json:"baseDomain,omitempty"`
	OCPInstall      EnvironmentOCPInstallSpec                `yaml:"ocpInstall,omitempty" json:"ocpInstall,omitempty"`
	Secrets         EnvironmentSecretsSpec                   `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	OpenShift       EnvironmentOpenShiftSpec                 `yaml:"openshift,omitempty" json:"openshift,omitempty"`
	ComponentImages map[string]map[string]ComponentImageSpec `yaml:"componentImages,omitempty" json:"componentImages,omitempty"`
}

// EnvironmentOCPInstallSpec selects how the OpenShift install reaches its
// release content and supporting registries. It is structural: exactly one of
// Connected, Restricted, or Disconnected is set; the presence of the
// sub-block is the discriminator (state-model.md R3). The selection scopes
// only OpenShift install material — release payload, mirror registry, and
// trust bundles — and does not describe the lab host's substrate
// connectivity.
type EnvironmentOCPInstallSpec struct {
	Connected    *ConnectedSpec    `yaml:"connected,omitempty" json:"connected,omitempty"`
	Restricted   *RestrictedSpec   `yaml:"restricted,omitempty" json:"restricted,omitempty"`
	Disconnected *DisconnectedSpec `yaml:"disconnected,omitempty" json:"disconnected,omitempty"`
}

// ConnectedSpec is intentionally empty: no proxy, mirror, or trust material is
// permitted under `connected` (security.md).
type ConnectedSpec struct{}

type RestrictedSpec struct {
	Proxy      *OCPInstallProxy      `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Registries *OCPInstallRegistries `yaml:"registries,omitempty" json:"registries,omitempty"`
}

type DisconnectedSpec struct {
	Proxy      *OCPInstallProxy      `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Registries *OCPInstallRegistries `yaml:"registries,omitempty" json:"registries,omitempty"`
}

type OCPInstallProxy struct {
	HTTPProxy  string   `yaml:"httpProxy,omitempty" json:"httpProxy,omitempty"`
	HTTPSProxy string   `yaml:"httpsProxy,omitempty" json:"httpsProxy,omitempty"`
	NoProxy    []string `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
	// CredentialsRef points at a single-line `username:password` secret used
	// to authenticate against an upstream proxy. The URL fields above must be
	// supplied without inline credentials when CredentialsRef is set; gitups
	// merges the resolved credentials into the URL at apply time so they
	// never appear in committed YAML or rendered install-config artifacts.
	CredentialsRef SecretRef `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
}

type OCPInstallRegistries struct {
	Mirror             *OCPInstallRegistryMirror `yaml:"mirror,omitempty" json:"mirror,omitempty"`
	ImageDigestSources []ImageDigestSource       `yaml:"imageDigestSources,omitempty" json:"imageDigestSources,omitempty"`
}

type OCPInstallRegistryMirror struct {
	URL            string                     `yaml:"url" json:"url"`
	CredentialsRef SecretRef                  `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
	TrustBundle    *OCPInstallRegistryTrustCA `yaml:"trustBundle,omitempty" json:"trustBundle,omitempty"`
}

type OCPInstallRegistryTrustCA struct {
	BundleRef           *SecretRef                 `yaml:"bundleRef,omitempty" json:"bundleRef,omitempty"`
	GeneratedSelfSigned *GeneratedSelfSignedCASpec `yaml:"generatedSelfSigned,omitempty" json:"generatedSelfSigned,omitempty"`
}

type GeneratedSelfSignedCASpec struct {
	SecretRef    SecretRef `yaml:"secretRef" json:"secretRef"`
	CommonName   string    `yaml:"commonName,omitempty" json:"commonName,omitempty"`
	DNSNames     []string  `yaml:"dnsNames,omitempty" json:"dnsNames,omitempty"`
	IPAddresses  []string  `yaml:"ipAddresses,omitempty" json:"ipAddresses,omitempty"`
	ValidityDays int       `yaml:"validityDays,omitempty" json:"validityDays,omitempty"`
}

type EnvironmentSecretsSpec struct {
	PullSecretRef    SecretRef `yaml:"pullSecretRef,omitempty" json:"pullSecretRef,omitempty"`
	ClusterSSHKeyRef SecretRef `yaml:"clusterSSHKeyRef,omitempty" json:"clusterSSHKeyRef,omitempty"`
}

type EnvironmentOpenShiftSpec struct {
	Release    *OCPReleaseSpec    `yaml:"release,omitempty" json:"release,omitempty"`
	Networking *OCPNetworkingSpec `yaml:"networking,omitempty" json:"networking,omitempty"`
}

// ComponentImageSpec carries both image refs Gitups may use to pull a
// component. `local` is tried first when set; `public` is the fallback. At
// least one of the two must be set, but the schema permits either to be
// omitted (leaving the other as the only acceptable source).
type ComponentImageSpec struct {
	Local  string `yaml:"local,omitempty" json:"local,omitempty"`
	Public string `yaml:"public,omitempty" json:"public,omitempty"`
}

// ----- InfrastructureProvider -----

type InfrastructureProvider struct {
	APIVersion string                     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                     `yaml:"kind" json:"kind"`
	Metadata   Metadata                   `yaml:"metadata" json:"metadata"`
	Spec       InfrastructureProviderSpec `yaml:"spec" json:"spec"`
	SourcePath string                     `yaml:"-" json:"-"`
}

type InfrastructureProviderSpec struct {
	QemuKVM                 *QemuKVMProviderSpec         `yaml:"qemuKVM,omitempty" json:"qemuKVM,omitempty"`
	BareMetal               *BareMetalProviderSpec       `yaml:"bareMetal,omitempty" json:"bareMetal,omitempty"`
	VMware                  *VMwareProviderSpec          `yaml:"vmware,omitempty" json:"vmware,omitempty"`
	OpenShiftVirtualization *OpenShiftVirtualizationSpec `yaml:"openShiftVirtualization,omitempty" json:"openShiftVirtualization,omitempty"`
}

type QemuKVMProviderSpec struct {
	Hosts           map[string]QemuKVMHostSpec    `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	BMCEmulation    *BMCEmulationSpec             `yaml:"bmcEmulation,omitempty" json:"bmcEmulation,omitempty"`
	MachineProfiles map[string]MachineProfileSpec `yaml:"machineProfiles,omitempty" json:"machineProfiles,omitempty"`
}

type QemuKVMHostSpec struct {
	Address      string    `yaml:"address" json:"address"`
	User         string    `yaml:"user,omitempty" json:"user,omitempty"`
	SSHKeyRef    SecretRef `yaml:"sshKeyRef" json:"sshKeyRef"`
	LibvirtURI   string    `yaml:"libvirtURI,omitempty" json:"libvirtURI,omitempty"`
	Capabilities []string  `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

type BMCEmulationSpec struct {
	Enabled     *bool        `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Protocol    string       `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Emulator    string       `yaml:"emulator,omitempty" json:"emulator,omitempty"`
	BindAddress string       `yaml:"bindAddress,omitempty" json:"bindAddress,omitempty"`
	Port        int          `yaml:"port,omitempty" json:"port,omitempty"`
	Auth        *BMCAuthSpec `yaml:"auth,omitempty" json:"auth,omitempty"`
}

type BMCAuthSpec struct {
	CredentialRef SecretRef `yaml:"credentialRef" json:"credentialRef"`
}

type MachineProfileSpec struct {
	CPU       int `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	MemoryMiB int `yaml:"memoryMiB,omitempty" json:"memoryMiB,omitempty"`
	DiskGiB   int `yaml:"diskGiB,omitempty" json:"diskGiB,omitempty"`
}

type BareMetalProviderSpec struct {
	BMCProtocol string `yaml:"bmcProtocol,omitempty" json:"bmcProtocol,omitempty"`
}

type VMwareProviderSpec struct {
	VCenterRef SecretRef `yaml:"vCenterRef" json:"vCenterRef"`
	Datacenter string    `yaml:"datacenter" json:"datacenter"`
	Cluster    string    `yaml:"cluster" json:"cluster"`
}

type OpenShiftVirtualizationSpec struct {
	ClusterRef      SecretRef             `yaml:"clusterRef" json:"clusterRef"`
	Namespace       string                `yaml:"namespace" json:"namespace"`
	StorageClassRef *LocalObjectReference `yaml:"storageClassRef,omitempty" json:"storageClassRef,omitempty"`
}

// ----- ClusterInfrastructure -----

type ClusterInfrastructure struct {
	APIVersion string                    `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                    `yaml:"kind" json:"kind"`
	Metadata   Metadata                  `yaml:"metadata" json:"metadata"`
	Spec       ClusterInfrastructureSpec `yaml:"spec" json:"spec"`
	SourcePath string                    `yaml:"-" json:"-"`
}

type ClusterInfrastructureSpec struct {
	ProviderRef    LocalObjectReference          `yaml:"providerRef" json:"providerRef"`
	Networks       map[string]MachineNetworkSpec `yaml:"networks,omitempty" json:"networks,omitempty"`
	Machines       map[string]MachineSpec        `yaml:"machines,omitempty" json:"machines,omitempty"`
	Endpoints      ClusterEndpointsSpec          `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	LoadBalancers  map[string]LoadBalancerSpec   `yaml:"loadBalancers,omitempty" json:"loadBalancers,omitempty"`
	NameResolution NameResolutionSpec            `yaml:"nameResolution,omitempty" json:"nameResolution,omitempty"`
}

// MachineNetworkSpec describes a network instance the cluster needs on the
// referenced provider. Provider-typed sub-blocks (qemuKVM, vmware) carry the
// provider-specific realisation of that network and must match the provider's
// structural sub-block on InfrastructureProvider.spec.
type MachineNetworkSpec struct {
	CIDR       string                     `yaml:"cidr" json:"cidr"`
	Gateway    string                     `yaml:"gateway,omitempty" json:"gateway,omitempty"`
	DNSServers []string                   `yaml:"dnsServers,omitempty" json:"dnsServers,omitempty"`
	Libvirt    *MachineNetworkLibvirtSpec `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	VMware     *MachineNetworkVMwareSpec  `yaml:"vmware,omitempty" json:"vmware,omitempty"`
}

type MachineNetworkLibvirtSpec struct {
	Bridge string `yaml:"bridge" json:"bridge"`
}

type MachineNetworkVMwareSpec struct {
	Portgroup string `yaml:"portgroup,omitempty" json:"portgroup,omitempty"`
}

type MachineSpec struct {
	ProfileRef      *LocalObjectReference           `yaml:"profileRef,omitempty" json:"profileRef,omitempty"`
	Resources       *MachineResourcesSpec           `yaml:"resources,omitempty" json:"resources,omitempty"`
	Interfaces      map[string]MachineInterfaceSpec `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	RootDeviceHints *RootDeviceHintsSpec            `yaml:"rootDeviceHints,omitempty" json:"rootDeviceHints,omitempty"`
	Libvirt         *MachineLibvirtSpec             `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	Baremetal       *MachineBaremetalSpec           `yaml:"baremetal,omitempty" json:"baremetal,omitempty"`
	VMware          *MachineVMwareSpec              `yaml:"vmware,omitempty" json:"vmware,omitempty"`
}

type MachineResourcesSpec struct {
	CPU       int `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	MemoryMiB int `yaml:"memoryMiB,omitempty" json:"memoryMiB,omitempty"`
	DiskGiB   int `yaml:"diskGiB,omitempty" json:"diskGiB,omitempty"`
}

type MachineInterfaceSpec struct {
	NetworkRef LocalObjectReference `yaml:"networkRef" json:"networkRef"`
	IPAddress  string               `yaml:"ipAddress,omitempty" json:"ipAddress,omitempty"`
	MACAddress string               `yaml:"macAddress,omitempty" json:"macAddress,omitempty"`
	Primary    *bool                `yaml:"primary,omitempty" json:"primary,omitempty"`
}

type RootDeviceHintsSpec struct {
	DeviceName       string `yaml:"deviceName,omitempty" json:"deviceName,omitempty"`
	HCTL             string `yaml:"hctl,omitempty" json:"hctl,omitempty"`
	Model            string `yaml:"model,omitempty" json:"model,omitempty"`
	Vendor           string `yaml:"vendor,omitempty" json:"vendor,omitempty"`
	SerialNumber     string `yaml:"serialNumber,omitempty" json:"serialNumber,omitempty"`
	MinSizeGigabytes int    `yaml:"minSizeGigabytes,omitempty" json:"minSizeGigabytes,omitempty"`
	WWN              string `yaml:"wwn,omitempty" json:"wwn,omitempty"`
	Rotational       *bool  `yaml:"rotational,omitempty" json:"rotational,omitempty"`
}

type MachineLibvirtSpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
}

type MachineBaremetalSpec struct {
	BootMACAddress string          `yaml:"bootMACAddress,omitempty" json:"bootMACAddress,omitempty"`
	BMC            *MachineBMCSpec `yaml:"bmc,omitempty" json:"bmc,omitempty"`
}

type MachineBMCSpec struct {
	Address                        string    `yaml:"address" json:"address"`
	Port                           int       `yaml:"port,omitempty" json:"port,omitempty"`
	Protocol                       string    `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	CredentialRef                  SecretRef `yaml:"credentialRef,omitempty" json:"credentialRef,omitempty"`
	DisableCertificateVerification bool      `yaml:"disableCertificateVerification,omitempty" json:"disableCertificateVerification,omitempty"`
}

type MachineVMwareSpec struct {
	Datastore string `yaml:"datastore,omitempty" json:"datastore,omitempty"`
	Folder    string `yaml:"folder,omitempty" json:"folder,omitempty"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
}

// ClusterEndpointsSpec holds api / api-int / ingress endpoints with VIPs.
type ClusterEndpointsSpec struct {
	API     *EndpointSpec `yaml:"api,omitempty" json:"api,omitempty"`
	APIInt  *EndpointSpec `yaml:"apiInt,omitempty" json:"apiInt,omitempty"`
	Ingress *EndpointSpec `yaml:"ingress,omitempty" json:"ingress,omitempty"`
}

type EndpointSpec struct {
	Hostname string `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Address  string `yaml:"address" json:"address"`
}

// LoadBalancerSpec binds a list of cluster endpoints (by name) to a managed
// HAProxy instance placed on a provider host. Standard OpenShift LB ports are
// implied by endpoint names: api → 6443, apiInt → 22623, ingress → 80+443.
type LoadBalancerSpec struct {
	Placement LoadBalancerPlacementSpec `yaml:"placement" json:"placement"`
	Endpoints []string                  `yaml:"endpoints" json:"endpoints"`
}

type LoadBalancerPlacementSpec struct {
	ProviderHostRef LocalObjectReference `yaml:"providerHostRef" json:"providerHostRef"`
}

// NameResolutionSpec is structural: exactly one of Managed or External is set.
// External is the empty selection (the operator owns DNS); Managed delegates
// /etc/hosts placement on listed provider hosts (state-model.md R3).
type NameResolutionSpec struct {
	Managed  *ManagedNameResolutionSpec  `yaml:"managed,omitempty" json:"managed,omitempty"`
	External *ExternalNameResolutionSpec `yaml:"external,omitempty" json:"external,omitempty"`
}

type ExternalNameResolutionSpec struct{}

type ManagedNameResolutionSpec struct {
	ProviderHostRefs       []LocalObjectReference `yaml:"providerHostRefs,omitempty" json:"providerHostRefs,omitempty"`
	AdditionalIngressHosts []string               `yaml:"additionalIngressHosts,omitempty" json:"additionalIngressHosts,omitempty"`
}

// ----- OCPCluster -----

type OCPCluster struct {
	APIVersion string         `yaml:"apiVersion" json:"apiVersion"`
	Kind       string         `yaml:"kind" json:"kind"`
	Metadata   Metadata       `yaml:"metadata" json:"metadata"`
	Spec       OCPClusterSpec `yaml:"spec" json:"spec"`
	SourcePath string         `yaml:"-" json:"-"`
}

type OCPClusterSpec struct {
	Role              string                 `yaml:"role" json:"role"`
	Topology          string                 `yaml:"topology,omitempty" json:"topology,omitempty"`
	InfrastructureRef LocalObjectReference   `yaml:"infrastructureRef" json:"infrastructureRef"`
	Install           OCPInstallSpec         `yaml:"install,omitempty" json:"install,omitempty"`
	Networking        *OCPNetworkingSpec     `yaml:"networking,omitempty" json:"networking,omitempty"`
	Nodes             map[string]OCPNodeSpec `yaml:"nodes,omitempty" json:"nodes,omitempty"`
}

type OCPNodeSpec struct {
	Role       string                `yaml:"role" json:"role"`
	MachineRef *LocalObjectReference `yaml:"machineRef,omitempty" json:"machineRef,omitempty"`
}

type OCPReleaseSpec struct {
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

type OCPInstallSpec struct {
	Method                   string                `yaml:"method,omitempty" json:"method,omitempty"`
	Release                  *OCPReleaseSpec       `yaml:"release,omitempty" json:"release,omitempty"`
	BaseDomain               string                `yaml:"baseDomain,omitempty" json:"baseDomain,omitempty"`
	PullSecretRef            SecretRef             `yaml:"pullSecretRef,omitempty" json:"pullSecretRef,omitempty"`
	SSHKeyRef                SecretRef             `yaml:"sshKeyRef,omitempty" json:"sshKeyRef,omitempty"`
	AdditionalTrustBundleRef SecretRef             `yaml:"additionalTrustBundleRef,omitempty" json:"additionalTrustBundleRef,omitempty"`
	GeneratedSecrets         []GeneratedSecretSpec `yaml:"generatedSecrets,omitempty" json:"generatedSecrets,omitempty"`
	ImageDigestSources       []ImageDigestSource   `yaml:"imageDigestSources,omitempty" json:"imageDigestSources,omitempty"`
	InstallConfigOverrides   map[string]any        `yaml:"installConfigOverrides,omitempty" json:"installConfigOverrides,omitempty"`
	AgentConfigOverrides     map[string]any        `yaml:"agentConfigOverrides,omitempty" json:"agentConfigOverrides,omitempty"`
}

type GeneratedSecretSpec struct {
	Name                  string                     `yaml:"name" json:"name"`
	Type                  string                     `yaml:"type,omitempty" json:"type,omitempty"`
	SelfSignedCertificate *SelfSignedCertificateSpec `yaml:"selfSignedCertificate,omitempty" json:"selfSignedCertificate,omitempty"`
}

type SelfSignedCertificateSpec struct {
	CommonName   string   `yaml:"commonName" json:"commonName"`
	DNSNames     []string `yaml:"dnsNames,omitempty" json:"dnsNames,omitempty"`
	IPAddresses  []string `yaml:"ipAddresses,omitempty" json:"ipAddresses,omitempty"`
	ValidityDays int      `yaml:"validityDays,omitempty" json:"validityDays,omitempty"`
}

type ImageDigestSource struct {
	Source       string   `yaml:"source" json:"source"`
	Mirrors      []string `yaml:"mirrors" json:"mirrors"`
	SourcePolicy string   `yaml:"sourcePolicy,omitempty" json:"sourcePolicy,omitempty"`
}

type OCPNetworkingSpec struct {
	NetworkType    string                  `yaml:"networkType,omitempty" json:"networkType,omitempty"`
	ClusterNetwork []OCPClusterNetworkCIDR `yaml:"clusterNetwork,omitempty" json:"clusterNetwork,omitempty"`
	ServiceNetwork []string                `yaml:"serviceNetwork,omitempty" json:"serviceNetwork,omitempty"`
}

type OCPClusterNetworkCIDR struct {
	CIDR       string `yaml:"cidr" json:"cidr"`
	HostPrefix int    `yaml:"hostPrefix,omitempty" json:"hostPrefix,omitempty"`
}

// ----- Discriminator helpers -----

// ProviderKind reports the structural discriminator of an
// InfrastructureProvider. Returns the empty string when no recognised
// sub-block is set; validation rejects that case.
func ProviderKind(provider InfrastructureProvider) string {
	switch {
	case provider.Spec.QemuKVM != nil:
		return ProviderKindQemuKVM
	case provider.Spec.BareMetal != nil:
		return ProviderKindBareMetal
	case provider.Spec.VMware != nil:
		return ProviderKindVMware
	case provider.Spec.OpenShiftVirtualization != nil:
		return ProviderKindOpenShiftVirt
	default:
		return ""
	}
}

// MachineKind reports the structural discriminator of a MachineSpec, mapped
// onto the provider-kind constants so cross-layer comparisons against
// ProviderKind() continue to work. The YAML field names use substrate-flavor
// vocabulary (libvirt / baremetal / vmware) rather than product names.
func MachineKind(machine MachineSpec) string {
	switch {
	case machine.Libvirt != nil:
		return ProviderKindQemuKVM
	case machine.Baremetal != nil:
		return ProviderKindBareMetal
	case machine.VMware != nil:
		return ProviderKindVMware
	default:
		return ""
	}
}

// OCPInstallKind reports the structural discriminator of an Environment's
// ocpInstall block. Returns the empty string when no sub-block is set;
// validation rejects that case.
func OCPInstallKind(env Environment) string {
	switch {
	case env.Spec.OCPInstall.Connected != nil:
		return OCPInstallKindConnected
	case env.Spec.OCPInstall.Restricted != nil:
		return OCPInstallKindRestricted
	case env.Spec.OCPInstall.Disconnected != nil:
		return OCPInstallKindDisconnected
	default:
		return ""
	}
}

// OCPInstallRegistriesOf returns the registries pointer for restricted or
// disconnected modes, and nil otherwise (connected, unset).
func OCPInstallRegistriesOf(env Environment) *OCPInstallRegistries {
	switch {
	case env.Spec.OCPInstall.Disconnected != nil:
		return env.Spec.OCPInstall.Disconnected.Registries
	case env.Spec.OCPInstall.Restricted != nil:
		return env.Spec.OCPInstall.Restricted.Registries
	default:
		return nil
	}
}

// OCPInstallProxyOf returns the proxy pointer for restricted or
// disconnected modes, and nil otherwise.
func OCPInstallProxyOf(env Environment) *OCPInstallProxy {
	switch {
	case env.Spec.OCPInstall.Disconnected != nil:
		return env.Spec.OCPInstall.Disconnected.Proxy
	case env.Spec.OCPInstall.Restricted != nil:
		return env.Spec.OCPInstall.Restricted.Proxy
	default:
		return nil
	}
}

// NameResolutionKind reports the structural discriminator of a
// NameResolutionSpec. Returns "" when neither sub-block is set; validation
// rejects that case.
func NameResolutionKind(spec NameResolutionSpec) string {
	switch {
	case spec.Managed != nil:
		return NameResolutionKindManaged
	case spec.External != nil:
		return NameResolutionKindExternal
	default:
		return ""
	}
}

// StandardLoadBalancerPorts returns the implied (listen, target) port pairs
// for a standard OpenShift endpoint name.
func StandardLoadBalancerPorts(endpoint string) [][2]int {
	switch endpoint {
	case EndpointAPI:
		return [][2]int{{6443, 6443}}
	case EndpointAPIInt:
		return [][2]int{{22623, 22623}}
	case EndpointIngress:
		return [][2]int{{80, 80}, {443, 443}}
	default:
		return nil
	}
}

// StandardEndpointBackendRole returns the recommended backend node role for an
// endpoint. ingress goes to workers when present, otherwise control-plane;
// callers handle the worker fallback themselves. api/apiInt always go to
// control-plane.
func StandardEndpointBackendRole(endpoint string) string {
	switch endpoint {
	case EndpointAPI, EndpointAPIInt:
		return NodeRoleControlPlane
	default:
		return ""
	}
}

func BoolPtr(v bool) *bool {
	return &v
}
