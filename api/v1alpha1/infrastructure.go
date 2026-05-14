package v1alpha1

import "fmt"

const (
	APIVersion = "bootwright.io/v1alpha1"

	KindEnvironment            = "Environment"
	KindInfrastructureProvider = "InfrastructureProvider"
	KindClusterInfrastructure  = "ClusterInfrastructure"
	KindOCPCluster             = "OCPCluster"
	KindInfrastructureState    = "InfrastructureState"
	KindBootwrightLock         = "BootwrightLock"

	MachineFlavorLibvirt   = "libvirt"
	MachineFlavorBareMetal = "baremetal"
	MachineFlavorVSphere   = "vsphere"
	MachineFlavorKubeVirt  = "kubevirt"

	OCPInstallKindConnected    = "connected"
	OCPInstallKindDisconnected = "disconnected"

	OCPTopologySingleNode     = "single-node"
	OCPTopologyMultiNode      = "multi-node"
	OCPClusterRoleHub         = "hub"
	OCPClusterRoleManaged     = "managed"
	OCPInstallMethodAgent     = "agent"
	NodeRoleControlPlane      = "control-plane"
	NodeRoleWorker            = "worker"
	GeneratedSecretSelfSigned = "self-signed-certificate"
	DefaultPullSecretName     = "openshift-pull-secret"
	DefaultClusterSSHKeyName  = "cluster-admin-key"
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
	CapabilityProxy               = "proxy"
	ComponentCategoryLoadBalancer = "load-balancer"
	ComponentCategoryRegistry     = "registry"
	ComponentCategoryProxy        = "proxy"
	ComponentTypeHAProxy          = "haproxy"
	ComponentTypeMirrorRegistry   = "mirror-registry"
	ComponentTypeSquid            = "squid"
	ContainerRuntimePodman        = "podman"
	DefaultHAProxyImageRef        = "docker.io/library/haproxy:3.3.8@sha256:f14a1788b56894e7ec7b5cb0ca09dbb959b674cf3c980f92139ec008167d4a91"
	DefaultMirrorRegistryImageRef = "docker.io/library/registry:3.1.1@sha256:85347ed2ecde64161c7a4788a4d7d3dcc9d6f86f7be95834022e3c6a423a945a"
	DefaultSquidImageRef          = "docker.io/openeuler/squid:7.5-oe2403sp3@sha256:8e16e4439a7c0d4e0e71092a1611bb89cea9929c30642c18ba991ba7a7524d87"
	DefaultMirrorRegistryPort     = 5000
	DefaultSquidPort              = 3128

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
	OCPInstallType  string                                   `yaml:"ocpInstallType,omitempty" json:"ocpInstallType,omitempty"`
	Proxy           *EnvironmentProxySpec                    `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Registries      *EnvironmentRegistriesSpec               `yaml:"registries,omitempty" json:"registries,omitempty"`
	Secrets         map[string]EnvironmentSecretSpec         `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	OpenShift       EnvironmentOpenShiftSpec                 `yaml:"openshift,omitempty" json:"openshift,omitempty"`
	ComponentImages map[string]map[string]ComponentImageSpec `yaml:"componentImages,omitempty" json:"componentImages,omitempty"`
}

type EnvironmentSecretSpec struct {
	File      string                      `yaml:"file,omitempty" json:"file,omitempty"`
	Generated *EnvironmentSecretGenerated `yaml:"generated,omitempty" json:"generated,omitempty"`
}

type EnvironmentSecretGenerated struct {
	Credentials           *GeneratedCredentialsSpec  `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	SelfSignedCertificate *SelfSignedCertificateSpec `yaml:"selfSignedCertificate,omitempty" json:"selfSignedCertificate,omitempty"`
}

type GeneratedCredentialsSpec struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
}

type EnvironmentProxySpec struct {
	HTTP    string                    `yaml:"http,omitempty"    json:"http,omitempty"`
	HTTPS   string                    `yaml:"https,omitempty"   json:"https,omitempty"`
	NoProxy []string                  `yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
	Auth    *EnvironmentProxyAuthSpec `yaml:"auth,omitempty"    json:"auth,omitempty"`
}

type EnvironmentProxyAuthSpec struct {
	ProxyAuthRef SecretRef `yaml:"proxyAuthRef" json:"proxyAuthRef"`
}

type EnvironmentRegistriesSpec struct {
	Mirror             *EnvironmentRegistryMirrorSpec `yaml:"mirror,omitempty" json:"mirror,omitempty"`
	ImageDigestSources []ImageDigestSource            `yaml:"imageDigestSources,omitempty" json:"imageDigestSources,omitempty"`
}

type EnvironmentRegistryMirrorSpec struct {
	URL            string    `yaml:"url" json:"url"`
	CredentialsRef SecretRef `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
	TrustBundleRef SecretRef `yaml:"trustBundleRef,omitempty" json:"trustBundleRef,omitempty"`
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
	Proxy          *ProxyCapabilitySpec          `yaml:"proxy,omitempty" json:"proxy,omitempty"`
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

type ProxyCapabilitySpec struct {
	Squid *ProxySquidSpec `yaml:"squid,omitempty" json:"squid,omitempty"`
}

type ProxySquidSpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
	Port    int                  `yaml:"port,omitempty" json:"port,omitempty"`
	Runtime string               `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	DataDir string               `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
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
	BareMetal *MachineProviderBareMetalSpec `yaml:"baremetal,omitempty" json:"baremetal,omitempty"`
	VSphere   *MachineProviderVSphereSpec   `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
	KubeVirt  *MachineProviderKubeVirtSpec  `yaml:"kubevirt,omitempty" json:"kubevirt,omitempty"`
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

type MachineProviderBareMetalSpec struct {
	BMCProtocol string `yaml:"bmcProtocol,omitempty" json:"bmcProtocol,omitempty"`
}

type MachineProviderVSphereSpec struct {
	VCenterRef SecretRef `yaml:"vCenterRef" json:"vCenterRef"`
	Datacenter string    `yaml:"datacenter" json:"datacenter"`
	Cluster    string    `yaml:"cluster" json:"cluster"`
}

type MachineProviderKubeVirtSpec struct {
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
	Proxy                      *ProxyCapabilitySpec
	MachineProviderName        string
	LoadBalancerProviderName   string
	NameResolutionProviderName string
	RegistryProviderName       string
	ProxyProviderName          string
	ProviderRefNames           []string
}

func (c ProviderClosure) MachineFlavor() string {
	if c.Machine == nil {
		return ""
	}
	switch {
	case c.Machine.Libvirt != nil:
		return MachineFlavorLibvirt
	case c.Machine.BareMetal != nil:
		return MachineFlavorBareMetal
	case c.Machine.VSphere != nil:
		return MachineFlavorVSphere
	case c.Machine.KubeVirt != nil:
		return MachineFlavorKubeVirt
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
		if p.Spec.Proxy != nil {
			if closure.Proxy != nil {
				errs = append(errs, fmt.Sprintf("ClusterInfrastructure/%s providerRefs union has multiple suppliers for proxy capability (%s, %s)", ci.Metadata.Name, closure.ProxyProviderName, ref.Name))
			} else {
				closure.Proxy = p.Spec.Proxy
				closure.ProxyProviderName = ref.Name
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
	VSphere    *MachineNetworkVSphereSpec `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
}

type MachineNetworkLibvirtSpec struct {
	Bridge string `yaml:"bridge" json:"bridge"`
}

type MachineNetworkVSphereSpec struct {
	Portgroup string `yaml:"portgroup,omitempty" json:"portgroup,omitempty"`
}

type MachineSpec struct {
	ProfileRef      *LocalObjectReference           `yaml:"profileRef,omitempty" json:"profileRef,omitempty"`
	Resources       *MachineResourcesSpec           `yaml:"resources,omitempty" json:"resources,omitempty"`
	Interfaces      map[string]MachineInterfaceSpec `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	RootDeviceHints *RootDeviceHintsSpec            `yaml:"rootDeviceHints,omitempty" json:"rootDeviceHints,omitempty"`
	Libvirt         *MachineLibvirtSpec             `yaml:"libvirt,omitempty" json:"libvirt,omitempty"`
	BareMetal       *MachineBareMetalSpec           `yaml:"baremetal,omitempty" json:"baremetal,omitempty"`
	VSphere         *MachineVSphereSpec             `yaml:"vsphere,omitempty" json:"vsphere,omitempty"`
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

type MachineBareMetalSpec struct {
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

type MachineVSphereSpec struct {
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
	Role              string                 `yaml:"role,omitempty" json:"role,omitempty"`
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
	case provider.Spec.Machine.BareMetal != nil:
		return MachineFlavorBareMetal
	case provider.Spec.Machine.VSphere != nil:
		return MachineFlavorVSphere
	case provider.Spec.Machine.KubeVirt != nil:
		return MachineFlavorKubeVirt
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

func ProviderProxySquid(provider InfrastructureProvider) *ProxySquidSpec {
	if provider.Spec.Proxy == nil {
		return nil
	}
	return provider.Spec.Proxy.Squid
}

func OCPInstallKind(env Environment) string {
	if env.Spec.OCPInstallType == "" {
		return OCPInstallKindConnected
	}
	return env.Spec.OCPInstallType
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
