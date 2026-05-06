package v1alpha1

import "fmt"

const (
	APIVersion = "gitups.io/v1alpha1"

	KindEnvironment            = "Environment"
	KindInfrastructureProvider = "InfrastructureProvider"
	KindClusterInfrastructure  = "ClusterInfrastructure"
	KindOCPCluster             = "OCPCluster"
	KindInfrastructureState    = "InfrastructureState"
	KindGitupsLock             = "GitupsLock"

	MachineFlavorLibvirt   = "libvirt"
	MachineFlavorBaremetal = "baremetal"
	MachineFlavorVsphere   = "vsphere"
	MachineFlavorKubevirt  = "kubevirt"

	OCPInstallKindConnected    = "connected"
	OCPInstallKindRestricted   = "restricted"
	OCPInstallKindDisconnected = "disconnected"

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
	CapabilityMirrorRegistry      = "mirror-registry"
	ComponentCategoryLoadBalancer = "load-balancer"
	ComponentCategoryRegistry     = "registry"
	ComponentTypeHAProxy          = "haproxy"
	ComponentTypeMirrorRegistry   = "mirror-registry"
	ContainerRuntimePodman        = "podman"
	DefaultHAProxyImageRef        = "docker.io/library/haproxy:3.2.15"
	DefaultMirrorRegistryImageRef = "docker.io/library/registry:2"
	DefaultMirrorRegistryPort     = 5000

	EndpointAPI     = "api"
	EndpointAPIInt  = "apiInt"
	EndpointIngress = "ingress"
)

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

type LocalObjectReference struct {
	Name string `yaml:"name" json:"name"`
}

type SecretRef struct {
	Name string `yaml:"name" json:"name"`
}

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
	Keys            map[string]EnvironmentKeySpec            `yaml:"keys,omitempty" json:"keys,omitempty"`
	OpenShift       EnvironmentOpenShiftSpec                 `yaml:"openshift,omitempty" json:"openshift,omitempty"`
	ComponentImages map[string]map[string]ComponentImageSpec `yaml:"componentImages,omitempty" json:"componentImages,omitempty"`
}

type EnvironmentKeySpec struct {
	File      string                   `yaml:"file,omitempty" json:"file,omitempty"`
	Generated *EnvironmentKeyGenerated `yaml:"generated,omitempty" json:"generated,omitempty"`
}

type EnvironmentKeyGenerated struct {
	Credentials           *GeneratedCredentialsSpec  `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	SelfSignedCertificate *SelfSignedCertificateSpec `yaml:"selfSignedCertificate,omitempty" json:"selfSignedCertificate,omitempty"`
}

type GeneratedCredentialsSpec struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
}

type EnvironmentOCPInstallSpec struct {
	Connected    *ConnectedSpec    `yaml:"connected,omitempty" json:"connected,omitempty"`
	Restricted   *RestrictedSpec   `yaml:"restricted,omitempty" json:"restricted,omitempty"`
	Disconnected *DisconnectedSpec `yaml:"disconnected,omitempty" json:"disconnected,omitempty"`
}

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
	HTTPProxy      string    `yaml:"httpProxy,omitempty" json:"httpProxy,omitempty"`
	HTTPSProxy     string    `yaml:"httpsProxy,omitempty" json:"httpsProxy,omitempty"`
	NoProxy        []string  `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
	CredentialsRef SecretRef `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
}

type OCPInstallRegistries struct {
	Mirror             *OCPInstallRegistryMirror `yaml:"mirror,omitempty" json:"mirror,omitempty"`
	ImageDigestSources []ImageDigestSource       `yaml:"imageDigestSources,omitempty" json:"imageDigestSources,omitempty"`
}

type OCPInstallRegistryMirror struct {
	URL            string    `yaml:"url" json:"url"`
	CredentialsRef SecretRef `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
	TrustBundleRef SecretRef `yaml:"trustBundleRef,omitempty" json:"trustBundleRef,omitempty"`
}

type EnvironmentSecretsSpec struct {
	PullSecretRef    SecretRef `yaml:"pullSecretRef,omitempty" json:"pullSecretRef,omitempty"`
	ClusterSSHKeyRef SecretRef `yaml:"clusterSSHKeyRef,omitempty" json:"clusterSSHKeyRef,omitempty"`
}

type EnvironmentOpenShiftSpec struct {
	Release    *OCPReleaseSpec    `yaml:"release,omitempty" json:"release,omitempty"`
	Networking *OCPNetworkingSpec `yaml:"networking,omitempty" json:"networking,omitempty"`
}

type ComponentImageSpec struct {
	Local  string `yaml:"local,omitempty" json:"local,omitempty"`
	Public string `yaml:"public,omitempty" json:"public,omitempty"`
}

type InfrastructureProvider struct {
	APIVersion string                     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                     `yaml:"kind" json:"kind"`
	Metadata   Metadata                   `yaml:"metadata" json:"metadata"`
	Spec       InfrastructureProviderSpec `yaml:"spec" json:"spec"`
	SourcePath string                     `yaml:"-" json:"-"`
}

type InfrastructureProviderSpec struct {
	Hosts          map[string]ProviderHostSpec   `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	Machine        *MachineCapabilitySpec        `yaml:"machine,omitempty" json:"machine,omitempty"`
	LoadBalancer   *LoadBalancerCapabilitySpec   `yaml:"loadBalancer,omitempty" json:"loadBalancer,omitempty"`
	NameResolution *NameResolutionCapabilitySpec `yaml:"nameResolution,omitempty" json:"nameResolution,omitempty"`
	Registry       *RegistryCapabilitySpec       `yaml:"registry,omitempty" json:"registry,omitempty"`
}

type RegistryCapabilitySpec struct {
	MirrorRegistry *RegistryMirrorSpec `yaml:"mirrorRegistry,omitempty" json:"mirrorRegistry,omitempty"`
}

type RegistryMirrorSpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
	Port    int                  `yaml:"port,omitempty" json:"port,omitempty"`
	DataDir string               `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
	Runtime string               `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

type NameResolutionCapabilitySpec struct {
	HostsFile *NameResolutionHostsFileSpec `yaml:"hostsFile,omitempty" json:"hostsFile,omitempty"`
}

type NameResolutionHostsFileSpec struct {
	HostRefs               []LocalObjectReference `yaml:"hostRefs,omitempty" json:"hostRefs,omitempty"`
	AdditionalIngressHosts []string               `yaml:"additionalIngressHosts,omitempty" json:"additionalIngressHosts,omitempty"`
}

type LoadBalancerCapabilitySpec struct {
	HAProxy *LoadBalancerHAProxySpec `yaml:"haProxy,omitempty" json:"haProxy,omitempty"`
}

type LoadBalancerHAProxySpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
	Runtime string               `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

type ProviderHostSpec struct {
	SSH          *ProviderHostSSHSpec `yaml:"ssh,omitempty" json:"ssh,omitempty"`
	LibvirtURI   string               `yaml:"libvirtURI,omitempty" json:"libvirtURI,omitempty"`
	Capabilities []string             `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

type ProviderHostSSHSpec struct {
	Address string    `yaml:"address" json:"address"`
	User    string    `yaml:"user,omitempty" json:"user,omitempty"`
	KeyRef  SecretRef `yaml:"keyRef" json:"keyRef"`
}

type MachineCapabilitySpec struct {
	Libvirt   *MachineProviderLibvirtSpec   `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	Baremetal *MachineProviderBaremetalSpec `yaml:"baremetal,omitempty" json:"baremetal,omitempty"`
	Vsphere   *MachineProviderVsphereSpec   `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
	Kubevirt  *MachineProviderKubevirtSpec  `yaml:"kubevirt,omitempty" json:"kubevirt,omitempty"`
}

type MachineProviderLibvirtSpec struct {
	HostRefs        []LocalObjectReference        `yaml:"hostRefs,omitempty" json:"hostRefs,omitempty"`
	BMCEmulation    *BMCEmulationSpec             `yaml:"bmcEmulation,omitempty" json:"bmcEmulation,omitempty"`
	MachineProfiles map[string]MachineProfileSpec `yaml:"machineProfiles,omitempty" json:"machineProfiles,omitempty"`
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

type MachineProviderBaremetalSpec struct {
	BMCProtocol string `yaml:"bmcProtocol,omitempty" json:"bmcProtocol,omitempty"`
}

type MachineProviderVsphereSpec struct {
	VCenterRef SecretRef `yaml:"vCenterRef" json:"vCenterRef"`
	Datacenter string    `yaml:"datacenter" json:"datacenter"`
	Cluster    string    `yaml:"cluster" json:"cluster"`
}

type MachineProviderKubevirtSpec struct {
	ClusterRef      SecretRef             `yaml:"clusterRef" json:"clusterRef"`
	Namespace       string                `yaml:"namespace" json:"namespace"`
	StorageClassRef *LocalObjectReference `yaml:"storageClassRef,omitempty" json:"storageClassRef,omitempty"`
}

type ClusterInfrastructure struct {
	APIVersion string                    `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                    `yaml:"kind" json:"kind"`
	Metadata   Metadata                  `yaml:"metadata" json:"metadata"`
	Spec       ClusterInfrastructureSpec `yaml:"spec" json:"spec"`
	SourcePath string                    `yaml:"-" json:"-"`
}

type ClusterInfrastructureSpec struct {
	ProviderRefs  []LocalObjectReference        `yaml:"providerRefs" json:"providerRefs"`
	Networks      map[string]MachineNetworkSpec `yaml:"networks,omitempty" json:"networks,omitempty"`
	Machines      map[string]MachineSpec        `yaml:"machines,omitempty" json:"machines,omitempty"`
	Endpoints     ClusterEndpointsSpec          `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	LoadBalancers map[string]LoadBalancerSpec   `yaml:"loadBalancers,omitempty" json:"loadBalancers,omitempty"`
}

type ProviderClosure struct {
	Hosts                      map[string]ProviderHostSpec
	Machine                    *MachineCapabilitySpec
	LoadBalancer               *LoadBalancerCapabilitySpec
	NameResolution             *NameResolutionCapabilitySpec
	Registry                   *RegistryCapabilitySpec
	MachineProviderName        string
	LoadBalancerProviderName   string
	NameResolutionProviderName string
	RegistryProviderName       string
	ProviderRefNames           []string
}

func (c ProviderClosure) MachineFlavor() string {
	if c.Machine == nil {
		return ""
	}
	switch {
	case c.Machine.Libvirt != nil:
		return MachineFlavorLibvirt
	case c.Machine.Baremetal != nil:
		return MachineFlavorBaremetal
	case c.Machine.Vsphere != nil:
		return MachineFlavorVsphere
	case c.Machine.Kubevirt != nil:
		return MachineFlavorKubevirt
	default:
		return ""
	}
}

func BuildProviderClosure(ci ClusterInfrastructure, providers map[string]InfrastructureProvider) (ProviderClosure, []string) {
	closure := ProviderClosure{Hosts: map[string]ProviderHostSpec{}}
	var errs []string
	for _, ref := range ci.Spec.ProviderRefs {
		closure.ProviderRefNames = append(closure.ProviderRefNames, ref.Name)
		p, ok := providers[ref.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs %q does not match any InfrastructureProvider", ci.Metadata.Name, ref.Name))
			continue
		}
		for hostName, host := range p.Spec.Hosts {
			if _, dup := closure.Hosts[hostName]; dup {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union contains duplicate host %q", ci.Metadata.Name, hostName))
				continue
			}
			closure.Hosts[hostName] = host
		}
		if p.Spec.Machine != nil {
			if closure.Machine != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union has multiple suppliers for machine capability (%s, %s)", ci.Metadata.Name, closure.MachineProviderName, ref.Name))
			} else {
				closure.Machine = p.Spec.Machine
				closure.MachineProviderName = ref.Name
			}
		}
		if p.Spec.LoadBalancer != nil {
			if closure.LoadBalancer != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union has multiple suppliers for loadBalancer capability (%s, %s)", ci.Metadata.Name, closure.LoadBalancerProviderName, ref.Name))
			} else {
				closure.LoadBalancer = p.Spec.LoadBalancer
				closure.LoadBalancerProviderName = ref.Name
			}
		}
		if p.Spec.NameResolution != nil {
			if closure.NameResolution != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union has multiple suppliers for nameResolution capability (%s, %s)", ci.Metadata.Name, closure.NameResolutionProviderName, ref.Name))
			} else {
				closure.NameResolution = p.Spec.NameResolution
				closure.NameResolutionProviderName = ref.Name
			}
		}
		if p.Spec.Registry != nil {
			if closure.Registry != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union has multiple suppliers for registry capability (%s, %s)", ci.Metadata.Name, closure.RegistryProviderName, ref.Name))
			} else {
				closure.Registry = p.Spec.Registry
				closure.RegistryProviderName = ref.Name
			}
		}
	}
	return closure, errs
}

type MachineNetworkSpec struct {
	CIDR       string                     `yaml:"cidr" json:"cidr"`
	Gateway    string                     `yaml:"gateway,omitempty" json:"gateway,omitempty"`
	DNSServers []string                   `yaml:"dnsServers,omitempty" json:"dnsServers,omitempty"`
	Libvirt    *MachineNetworkLibvirtSpec `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	Vsphere    *MachineNetworkVsphereSpec `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
}

type MachineNetworkLibvirtSpec struct {
	Bridge string `yaml:"bridge" json:"bridge"`
}

type MachineNetworkVsphereSpec struct {
	Portgroup string `yaml:"portgroup,omitempty" json:"portgroup,omitempty"`
}

type MachineSpec struct {
	ProfileRef      *LocalObjectReference           `yaml:"profileRef,omitempty" json:"profileRef,omitempty"`
	Resources       *MachineResourcesSpec           `yaml:"resources,omitempty" json:"resources,omitempty"`
	Interfaces      map[string]MachineInterfaceSpec `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	RootDeviceHints *RootDeviceHintsSpec            `yaml:"rootDeviceHints,omitempty" json:"rootDeviceHints,omitempty"`
	Libvirt         *MachineLibvirtSpec             `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	Baremetal       *MachineBaremetalSpec           `yaml:"baremetal,omitempty" json:"baremetal,omitempty"`
	Vsphere         *MachineVsphereSpec             `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
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

type MachineVsphereSpec struct {
	Datastore string `yaml:"datastore,omitempty" json:"datastore,omitempty"`
	Folder    string `yaml:"folder,omitempty" json:"folder,omitempty"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
}

type ClusterEndpointsSpec struct {
	API     *EndpointSpec `yaml:"api,omitempty" json:"api,omitempty"`
	APIInt  *EndpointSpec `yaml:"apiInt,omitempty" json:"apiInt,omitempty"`
	Ingress *EndpointSpec `yaml:"ingress,omitempty" json:"ingress,omitempty"`
}

type EndpointSpec struct {
	Hostname string `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Address  string `yaml:"address" json:"address"`
}

type LoadBalancerSpec struct {
	Endpoints []string `yaml:"endpoints" json:"endpoints"`
}

type OCPCluster struct {
	APIVersion string         `yaml:"apiVersion" json:"apiVersion"`
	Kind       string         `yaml:"kind" json:"kind"`
	Metadata   Metadata       `yaml:"metadata" json:"metadata"`
	Spec       OCPClusterSpec `yaml:"spec" json:"spec"`
	SourcePath string         `yaml:"-" json:"-"`
}

type OCPClusterSpec struct {
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
	Method                   string              `yaml:"method,omitempty" json:"method,omitempty"`
	Release                  *OCPReleaseSpec     `yaml:"release,omitempty" json:"release,omitempty"`
	BaseDomain               string              `yaml:"baseDomain,omitempty" json:"baseDomain,omitempty"`
	PullSecretRef            SecretRef           `yaml:"pullSecretRef,omitempty" json:"pullSecretRef,omitempty"`
	SSHKeyRef                SecretRef           `yaml:"sshKeyRef,omitempty" json:"sshKeyRef,omitempty"`
	AdditionalTrustBundleRef SecretRef           `yaml:"additionalTrustBundleRef,omitempty" json:"additionalTrustBundleRef,omitempty"`
	ImageDigestSources       []ImageDigestSource `yaml:"imageDigestSources,omitempty" json:"imageDigestSources,omitempty"`
	InstallConfigOverrides   map[string]any      `yaml:"installConfigOverrides,omitempty" json:"installConfigOverrides,omitempty"`
	AgentConfigOverrides     map[string]any      `yaml:"agentConfigOverrides,omitempty" json:"agentConfigOverrides,omitempty"`
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

func MachineFlavor(provider InfrastructureProvider) string {
	if provider.Spec.Machine == nil {
		return ""
	}
	switch {
	case provider.Spec.Machine.Libvirt != nil:
		return MachineFlavorLibvirt
	case provider.Spec.Machine.Baremetal != nil:
		return MachineFlavorBaremetal
	case provider.Spec.Machine.Vsphere != nil:
		return MachineFlavorVsphere
	case provider.Spec.Machine.Kubevirt != nil:
		return MachineFlavorKubevirt
	default:
		return ""
	}
}

func MachineKind(machine MachineSpec) string {
	switch {
	case machine.Libvirt != nil:
		return MachineFlavorLibvirt
	case machine.Baremetal != nil:
		return MachineFlavorBaremetal
	case machine.Vsphere != nil:
		return MachineFlavorVsphere
	default:
		return ""
	}
}

func ProviderMachineLibvirt(provider InfrastructureProvider) *MachineProviderLibvirtSpec {
	if provider.Spec.Machine == nil {
		return nil
	}
	return provider.Spec.Machine.Libvirt
}

func ProviderMirrorRegistry(provider InfrastructureProvider) *RegistryMirrorSpec {
	if provider.Spec.Registry == nil {
		return nil
	}
	return provider.Spec.Registry.MirrorRegistry
}

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
