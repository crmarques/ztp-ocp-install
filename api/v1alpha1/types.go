// Package v1alpha1 defines the user-authored desired-state API. The schema is
// the four-domain-layer model declared in ADR 0001: Environment,
// InfrastructureProvider, ClusterInfrastructure, OCPCluster.
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
	Keys            map[string]EnvironmentKeySpec            `yaml:"keys,omitempty" json:"keys,omitempty"`
	OpenShift       EnvironmentOpenShiftSpec                 `yaml:"openshift,omitempty" json:"openshift,omitempty"`
	ComponentImages map[string]map[string]ComponentImageSpec `yaml:"componentImages,omitempty" json:"componentImages,omitempty"`
}

// EnvironmentKeySpec declares the source for a named key whose name is
// referenced by one or more SecretRefs in the desired state. Exactly one
// source sub-block is set: `file` points at operator-supplied material on
// disk; `generated` declares material gitups will materialize itself.
// `gitups secrets generate` walks every declared key and produces or links
// `<secretsDir>/<name>` accordingly (symlink for SSH file refs, copy for
// other file refs, fresh material for generated entries).
type EnvironmentKeySpec struct {
	File      string                   `yaml:"file,omitempty" json:"file,omitempty"`
	Generated *EnvironmentKeyGenerated `yaml:"generated,omitempty" json:"generated,omitempty"`
}

// EnvironmentKeyGenerated is a structural-discriminator union: exactly one
// of Credentials or SelfSignedCertificate is set. Adding a new generated
// kind means adding a new sub-block here, never a new top-level command.
type EnvironmentKeyGenerated struct {
	Credentials           *GeneratedCredentialsSpec  `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	SelfSignedCertificate *SelfSignedCertificateSpec `yaml:"selfSignedCertificate,omitempty" json:"selfSignedCertificate,omitempty"`
}

// GeneratedCredentialsSpec declares a `username:password\n` secret. The
// password is generated on first materialization and is preserved on
// subsequent runs to keep the BMC/registry/proxy/etc. callers stable.
type GeneratedCredentialsSpec struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
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
	URL            string    `yaml:"url" json:"url"`
	CredentialsRef SecretRef `yaml:"credentialsRef,omitempty" json:"credentialsRef,omitempty"`
	// TrustBundleRef names the secret that carries the registry's CA. The
	// secret may be operator-supplied (declare it in Environment.spec.keys
	// with a `file:` source) or gitups-generated (declare it with a
	// `generated.selfSignedCertificate` source). Either way, this field
	// just points at the name; how it is sourced lives in `keys`.
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

// InfrastructureProviderSpec is capability-oriented: each top-level field is
// an independent capability the provider may supply. v1 ships only the
// `machine` capability; later rounds add `loadBalancer`, `nameResolution`, …
// At least one capability sub-block must be set.
//
// Hosts is a shared provider host pool, optional. Capabilities that need an
// SSH-reachable Linux host (libvirt substrate, future haProxy / hostsFile)
// reference an entry by name. Capabilities that talk to an appliance over an
// API embed their endpoint in the capability block — the host pool stays
// optional so appliance-style providers need not declare it.
type InfrastructureProviderSpec struct {
	Hosts          map[string]ProviderHostSpec   `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	Machine        *MachineCapabilitySpec        `yaml:"machine,omitempty" json:"machine,omitempty"`
	LoadBalancer   *LoadBalancerCapabilitySpec   `yaml:"loadBalancer,omitempty" json:"loadBalancer,omitempty"`
	NameResolution *NameResolutionCapabilitySpec `yaml:"nameResolution,omitempty" json:"nameResolution,omitempty"`
	Registry       *RegistryCapabilitySpec       `yaml:"registry,omitempty" json:"registry,omitempty"`
}

// RegistryCapabilitySpec is the structural-discriminator union for the
// container-image registry capability. v1 ships only mirrorRegistry — a
// docker/distribution server colocated with a provider host. Omission of the
// capability block on a provider means external — the operator runs the
// mirror themselves and disconnected installs only consume it.
type RegistryCapabilitySpec struct {
	MirrorRegistry *RegistryMirrorSpec `yaml:"mirrorRegistry,omitempty" json:"mirrorRegistry,omitempty"`
}

// RegistryMirrorSpec describes a docker/distribution mirror server colocated
// on a provider host. The server's URL, credentials, and trust material are
// owned by Environment.spec.ocpInstall.{disconnected,restricted}.registries.mirror;
// this block contributes only the placement (which provider host runs it)
// and tunables (port, runtime, data path).
type RegistryMirrorSpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
	Port    int                  `yaml:"port,omitempty" json:"port,omitempty"`
	DataDir string               `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
	Runtime string               `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

// NameResolutionCapabilitySpec is the structural-discriminator union for the
// name-resolution capability. v1 ships only hostsFile (managed /etc/hosts on
// listed provider hosts). Omission of the capability block on a provider
// means external — the operator owns DNS for clusters bound to that provider.
type NameResolutionCapabilitySpec struct {
	HostsFile *NameResolutionHostsFileSpec `yaml:"hostsFile,omitempty" json:"hostsFile,omitempty"`
}

type NameResolutionHostsFileSpec struct {
	HostRefs               []LocalObjectReference `yaml:"hostRefs,omitempty" json:"hostRefs,omitempty"`
	AdditionalIngressHosts []string               `yaml:"additionalIngressHosts,omitempty" json:"additionalIngressHosts,omitempty"`
}

// LoadBalancerCapabilitySpec is the structural-discriminator union for the
// load-balancer capability. v1 ships only haProxy; future appliance flavors
// (BigIP, NSX-LB) slot in here without schema rework.
type LoadBalancerCapabilitySpec struct {
	HAProxy *LoadBalancerHAProxySpec `yaml:"haProxy,omitempty" json:"haProxy,omitempty"`
}

type LoadBalancerHAProxySpec struct {
	HostRef LocalObjectReference `yaml:"hostRef" json:"hostRef"`
	Runtime string               `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

// ProviderHostSpec describes one host in the provider's pool. The connection
// sub-block (ssh in v1) is the structural discriminator; future appliance
// providers may add httpsApi etc. LibvirtURI is a libvirt-only hint that
// happens to live on the host so the substrate can connect remotely.
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

// MachineCapabilitySpec is the substrate-flavor union for the machine
// capability. Exactly one flavor sub-block is set (state-model R3); the
// presence of the sub-block is the discriminator.
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

// ----- ClusterInfrastructure -----

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

// ProviderClosure is the merged view of all providers a ClusterInfrastructure
// references via spec.providerRefs. Each capability sub-pointer is set by at
// most one supplying provider in the closure; the validator rejects multiple
// suppliers for the same capability. Hosts is the union of all referenced
// providers' host pools (host names must be unique across the closure).
//
// MachineProviderName / LoadBalancerProviderName / NameResolutionProviderName
// record which provider supplied that capability — used for renderer
// dispatch and error messages.
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
	// ProviderRefNames lists the provider names in the order declared on the
	// ClusterInfrastructure; renderer entry points use this to keep deterministic
	// output across multi-provider closures.
	ProviderRefNames []string
}

// MachineFlavor reports the machine-flavor discriminator on a closure.
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

// BuildProviderClosure resolves a cluster's providerRefs against the loaded
// provider set and merges their capabilities. Errors are returned as a slice;
// validation reports them. The closure is best-effort — even when errors
// exist, the returned closure carries whatever could be merged so renderer
// callers can produce partial output for diagnostics.
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

// MachineNetworkSpec describes a network instance the cluster needs on the
// referenced provider. Provider-typed sub-blocks carry the provider-specific
// realisation of that network and must match the provider's structural
// sub-block on InfrastructureProvider.spec.
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

// LoadBalancerSpec binds a list of cluster endpoints (by name) to the
// provider's load-balancer capability. Standard OpenShift LB ports are
// implied by endpoint names: api → 6443, apiInt → 22623, ingress → 80+443.
// Placement now lives on the provider (spec.loadBalancer.<flavor>.hostRef).
type LoadBalancerSpec struct {
	Endpoints []string `yaml:"endpoints" json:"endpoints"`
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

// ----- Discriminator helpers -----

// MachineFlavor reports the structural discriminator of an
// InfrastructureProvider's machine capability. Returns the empty string when
// no recognised flavor sub-block is set; validation rejects that case.
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

// MachineKind reports the structural discriminator of a MachineSpec, returning
// the substrate-flavor name that aligns with MachineFlavor() on the provider.
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

// ProviderMachineLibvirt returns the libvirt machine-capability spec on
// provider, or nil. Convenience for the common consumer pattern.
func ProviderMachineLibvirt(provider InfrastructureProvider) *MachineProviderLibvirtSpec {
	if provider.Spec.Machine == nil {
		return nil
	}
	return provider.Spec.Machine.Libvirt
}

// ProviderMirrorRegistry returns the mirror-registry capability spec on
// provider, or nil. Convenience for the common consumer pattern.
func ProviderMirrorRegistry(provider InfrastructureProvider) *RegistryMirrorSpec {
	if provider.Spec.Registry == nil {
		return nil
	}
	return provider.Spec.Registry.MirrorRegistry
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
