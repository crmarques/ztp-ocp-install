package v1alpha1

const (
	KindGitOpsPackageSet  = "GitOpsPackageSet"
	KindPackageDefinition = "PackageDefinition"
	KindPackageDescriptor = "PackageDescriptor"

	PlaceholderSentinel = "__GITUPS_PLACEHOLDER__"

	UnitTypeInstall  = "install"
	UnitTypeResource = "resource"

	DomainInstall   = "install"
	DomainResources = "resources"
	DomainKRC       = "kubernetes-resource-controller"
	DomainSRC       = "service-resource-controller"

	RepoTypeKubernetesResources = "kubernetes-resources"
	RepoTypeServiceResources    = "service-resources"

	IntentApply               = "apply"
	IntentApplyDryRun         = "apply-dry-run"
	IntentGetJSON             = "get-json"
	IntentWaitCondition       = "wait-condition"
	IntentWaitCRDsEstablished = "wait-crds-established"
	IntentListCRDs            = "list-crds"
	IntentServerVersion       = "server-version"
	IntentListPodsJSONPath    = "list-pods-jsonpath"
	IntentPodLogs             = "pod-logs"

	RendererOLM       = "olm"
	RendererKustomize = "kustomize"
	RendererHelm      = "helm"
	RendererRaw       = "raw"

	GeneratorRandomHex    = "randomHex"
	GeneratorRandomBase64 = "randomBase64"
	GeneratorUUID         = "uuid"

	ProvisioningStyleCRD    = "crd"
	ProvisioningStyleScript = "script"

	ApplyWaveAnnotation = "gitups.io/apply-wave"
	ReadinessAnnotation = "gitups.io/readiness"
)

type Role string

const (
	RoleKRC      Role = "kubernetes-resource-controller"
	RoleSRC      Role = "service-resource-controller"
	RoleWorkload Role = "workload"
)

type GitOpsPackageSet struct {
	APIVersion string               `yaml:"apiVersion" json:"apiVersion"`
	Kind       string               `yaml:"kind" json:"kind"`
	Metadata   Metadata             `yaml:"metadata" json:"metadata"`
	Spec       GitOpsPackageSetSpec `yaml:"spec" json:"spec"`
}

type GitOpsPackageSetSpec struct {
	EnvKey       string           `yaml:"envKey,omitempty" json:"envKey,omitempty"`
	Extends      *Extends         `yaml:"extends,omitempty" json:"extends,omitempty"`
	Controllers  *Controllers     `yaml:"controllers,omitempty" json:"controllers,omitempty"`
	Sources      []PackageSource  `yaml:"sources" json:"sources"`
	Repositories []RepositoryDecl `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	Resolved     *ResolvedGitOps  `yaml:"resolved,omitempty" json:"resolved,omitempty"`
}

type Controllers struct {
	KubernetesResources *ControllerAssignment `yaml:"kubernetesResources,omitempty" json:"kubernetesResources,omitempty"`
	ServiceResources    *ControllerAssignment `yaml:"serviceResources,omitempty" json:"serviceResources,omitempty"`
}

type ControllerAssignment struct {
	Repo     string `yaml:"repo" json:"repo"`
	Instance string `yaml:"instance" json:"instance"`
}

type Extends struct {
	Source string `yaml:"source" json:"source"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
}

// PackageSource selects exactly one structural sub-block: filesystem, oci, or git.
// Presence of the sub-block is the discriminator (no `type:` string), per the
// authoritative R3 rule on structural-only discriminators.
type PackageSource struct {
	Name       string                   `yaml:"name" json:"name"`
	Filesystem *PackageSourceFilesystem `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	OCI        *PackageSourceOCI        `yaml:"oci,omitempty" json:"oci,omitempty"`
	Git        *PackageSourceGit        `yaml:"git,omitempty" json:"git,omitempty"`
}

type PackageSourceFilesystem struct {
	Path string `yaml:"path" json:"path"`
}

// PackageSourceOCI pulls a per-package OCI artifact from a registry. Either
// Tag or Digest must be set (digest preferred for immutability).
type PackageSourceOCI struct {
	Registry string `yaml:"registry" json:"registry"`
	Tag      string `yaml:"tag,omitempty" json:"tag,omitempty"`
	Digest   string `yaml:"digest,omitempty" json:"digest,omitempty"`
}

// PackageSourceGit clones a git repository at the given Ref (tag or commit
// SHA; branches are rejected). Path locates the catalog inside the clone,
// defaulting to "packages".
type PackageSourceGit struct {
	URL  string `yaml:"url" json:"url"`
	Ref  string `yaml:"ref" json:"ref"`
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

type PackageRef struct {
	Template      string         `yaml:"template" json:"template"`
	Instance      string         `yaml:"instance,omitempty" json:"instance,omitempty"`
	Role          Role           `yaml:"role,omitempty" json:"role,omitempty"`
	InstallMethod string         `yaml:"installMethod,omitempty" json:"installMethod,omitempty"`
	Values        map[string]any `yaml:"values,omitempty" json:"values,omitempty"`
	Resources     []ResourceRef  `yaml:"resources,omitempty" json:"resources,omitempty"`
	Bindings      []BindingRef   `yaml:"bindings,omitempty" json:"bindings,omitempty"`
}

type BindingRef struct {
	Name       string         `yaml:"name" json:"name"`
	Capability string         `yaml:"capability" json:"capability"`
	Provider   ProviderRef    `yaml:"provider" json:"provider"`
	Values     map[string]any `yaml:"values,omitempty" json:"values,omitempty"`
}

type ProviderRef struct {
	Repo     string `yaml:"repo" json:"repo"`
	Instance string `yaml:"instance" json:"instance"`
}

type ResourceRef struct {
	Template string         `yaml:"template" json:"template"`
	Name     string         `yaml:"name" json:"name"`
	Values   map[string]any `yaml:"values,omitempty" json:"values,omitempty"`
}

type RepositoryDecl struct {
	Name              string             `yaml:"name" json:"name"`
	Description       string             `yaml:"description,omitempty" json:"description,omitempty"`
	Type              string             `yaml:"type" json:"type"`
	RepoRef           *RepositoryRef     `yaml:"repoRef,omitempty" json:"repoRef,omitempty"`
	Packages          []PackageRef       `yaml:"packages,omitempty" json:"packages,omitempty"`
	ManagedServiceRef *ManagedServiceRef `yaml:"managedServiceRef,omitempty" json:"managedServiceRef,omitempty"`
}

type ManagedServiceRef struct {
	Repo     string `yaml:"repo" json:"repo"`
	Instance string `yaml:"instance" json:"instance"`
}

type RepositoryRef struct {
	Name   string `yaml:"name" json:"name"`
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"`
}

type ResolvedGitOps struct {
	SourcePackageSetRef Metadata             `yaml:"sourcePackageSetRef" json:"sourcePackageSetRef"`
	ExtendedFrom        *ExtendedFrom        `yaml:"extendedFrom,omitempty" json:"extendedFrom,omitempty"`
	Repository          RepositoryBlock      `yaml:"repository" json:"repository"`
	Repositories        []ResolvedRepository `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	Packages            []ResolvedPackage    `yaml:"packages" json:"packages"`
	Placeholders        []Placeholder        `yaml:"placeholders" json:"placeholders"`
}

type ExtendedFrom struct {
	Source string `yaml:"source" json:"source"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
}

type RepositoryBlock struct {
	Layout     string `yaml:"layout" json:"layout"`
	OutputPath string `yaml:"outputPath" json:"outputPath"`
}

type ResolvedRepository struct {
	Name              string             `yaml:"name" json:"name"`
	Description       string             `yaml:"description,omitempty" json:"description,omitempty"`
	Type              string             `yaml:"type,omitempty" json:"type,omitempty"`
	RepoRef           *RepositoryRef     `yaml:"repoRef,omitempty" json:"repoRef,omitempty"`
	ManagedServiceRef *ManagedServiceRef `yaml:"managedServiceRef,omitempty" json:"managedServiceRef,omitempty"`
}

type ResolvedPackage struct {
	Template         string             `yaml:"template" json:"template"`
	PackageInstance  string             `yaml:"packageInstance,omitempty" json:"packageInstance,omitempty"`
	UnitType         string             `yaml:"unitType" json:"unitType"`
	Domain           string             `yaml:"domain" json:"domain"`
	InstallMethod    string             `yaml:"installMethod,omitempty" json:"installMethod,omitempty"`
	ResourceTemplate string             `yaml:"resourceTemplate,omitempty" json:"resourceTemplate,omitempty"`
	ResourceName     string             `yaml:"resourceName,omitempty" json:"resourceName,omitempty"`
	Repository       string             `yaml:"repository" json:"repository"`
	Instance         string             `yaml:"instance" json:"instance"`
	Role             Role               `yaml:"role" json:"role"`
	Renderer         string             `yaml:"renderer" json:"renderer"`
	DependsOn        []string           `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	ResolvedValues   map[string]any     `yaml:"resolvedValues" json:"resolvedValues"`
	RenderedPaths    RenderedPaths      `yaml:"renderedPaths" json:"renderedPaths"`
	ApplyWave        int                `yaml:"applyWave" json:"applyWave"`
	Binding          *BindingOrigin     `yaml:"binding,omitempty" json:"binding,omitempty"`
	Controller       *ControllerBinding `yaml:"controller,omitempty" json:"controller,omitempty"`
}

type ControllerBinding struct {
	Kind     Role   `yaml:"kind" json:"kind"`
	Instance string `yaml:"instance" json:"instance"`
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
	Intent   string `yaml:"intent" json:"intent"`
}

type BindingOrigin struct {
	Name       string `yaml:"name" json:"name"`
	Capability string `yaml:"capability" json:"capability"`
	Side       string `yaml:"side" json:"side"`
}

type RenderedPaths struct {
	Repo string `yaml:"repo" json:"repo"`
	Dir  string `yaml:"dir" json:"dir"`
}

type Placeholder struct {
	Path      string     `yaml:"path" json:"path"`
	Reason    string     `yaml:"reason" json:"reason"`
	Sensitive bool       `yaml:"sensitive" json:"sensitive"`
	Generator *Generator `yaml:"generator,omitempty" json:"generator,omitempty"`
}

type Generator struct {
	Kind    string `yaml:"kind" json:"kind"`
	Length  int    `yaml:"length,omitempty" json:"length,omitempty"`
	Charset string `yaml:"charset,omitempty" json:"charset,omitempty"`
}

type PackageDefinition struct {
	APIVersion string                `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                `yaml:"kind" json:"kind"`
	Metadata   PackageMeta           `yaml:"metadata" json:"metadata"`
	Spec       PackageDefinitionSpec `yaml:"spec" json:"spec"`
}

type PackageMeta struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

type PackageDefinitionSpec struct {
	Role             Role                `yaml:"role" json:"role"`
	Category         string              `yaml:"category,omitempty" json:"category,omitempty"`
	DefaultInstall   string              `yaml:"defaultInstall,omitempty" json:"defaultInstall,omitempty"`
	DefaultResources []ResourceRef       `yaml:"defaultResources,omitempty" json:"defaultResources,omitempty"`
	Provides         []CapabilityProvide `yaml:"provides,omitempty" json:"provides,omitempty"`
	Requires         []CapabilityRequire `yaml:"requires,omitempty" json:"requires,omitempty"`
	CLI              *ControllerCLI      `yaml:"cli,omitempty" json:"cli,omitempty"`
	Readiness        []ReadinessCheck    `yaml:"readiness,omitempty" json:"readiness,omitempty"`
	DeclarestBundle  *DeclarestBundle    `yaml:"declarestBundle,omitempty" json:"declarestBundle,omitempty"`
	Compatibility    *Compatibility      `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
}

type Compatibility struct {
	Kubernetes []string `yaml:"kubernetes,omitempty" json:"kubernetes,omitempty"`
}

type DeclarestBundle struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	Ref     string `yaml:"ref,omitempty" json:"ref,omitempty"`
}

type ControllerCLI struct {
	Binary  string                         `yaml:"binary" json:"binary"`
	Args    []string                       `yaml:"args,omitempty" json:"args,omitempty"`
	Intents map[string]ControllerCLIIntent `yaml:"intents,omitempty" json:"intents,omitempty"`
}

type ControllerCLIIntent struct {
	Args []string `yaml:"args" json:"args"`
}

type CapabilityProvide struct {
	Capability       string             `yaml:"capability" json:"capability"`
	ResourceTemplate string             `yaml:"resourceTemplate" json:"resourceTemplate"`
	Exports          []CapabilityExport `yaml:"exports,omitempty" json:"exports,omitempty"`
}

type CapabilityExport struct {
	Name   string `yaml:"name" json:"name"`
	From   string `yaml:"from" json:"from"`
	Secret bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}

type CapabilityRequire struct {
	Capability       string              `yaml:"capability" json:"capability"`
	ResourceTemplate string              `yaml:"resourceTemplate" json:"resourceTemplate"`
	Consumes         []CapabilityConsume `yaml:"consumes,omitempty" json:"consumes,omitempty"`
}

type CapabilityConsume struct {
	Input string `yaml:"input" json:"input"`
	From  string `yaml:"from" json:"from"`
}

type PackageDescriptor struct {
	APIVersion string                `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                `yaml:"kind" json:"kind"`
	Metadata   Metadata              `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec       PackageDescriptorSpec `yaml:"spec" json:"spec"`
}

type PackageDescriptorSpec struct {
	Renderer             string           `yaml:"renderer" json:"renderer"`
	Inputs               []InputSpec      `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	DependsOn            []string         `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	Hooks                *HookSpec        `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Overlays             []string         `yaml:"overlays,omitempty" json:"overlays,omitempty"`
	OLM                  *OLMSpec         `yaml:"olm,omitempty" json:"olm,omitempty"`
	Helm                 *HelmSpec        `yaml:"helm,omitempty" json:"helm,omitempty"`
	Kustomize            *KustomizeSpec   `yaml:"kustomize,omitempty" json:"kustomize,omitempty"`
	ProvisioningStyle    string           `yaml:"provisioningStyle,omitempty" json:"provisioningStyle,omitempty"`
	ScriptImage          string           `yaml:"scriptImage,omitempty" json:"scriptImage,omitempty"`
	ScriptServiceAccount string           `yaml:"scriptServiceAccount,omitempty" json:"scriptServiceAccount,omitempty"`
	Readiness            []ReadinessCheck `yaml:"readiness,omitempty" json:"readiness,omitempty"`
}

type ReadinessCheck struct {
	Kind      string `yaml:"kind" json:"kind"`
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Condition string `yaml:"condition" json:"condition"`
}

type OLMSpec struct {
	Package             string `yaml:"package" json:"package"`
	Channel             string `yaml:"channel" json:"channel"`
	Source              string `yaml:"source" json:"source"`
	SourceNamespace     string `yaml:"sourceNamespace" json:"sourceNamespace"`
	StartingCSV         string `yaml:"startingCSV" json:"startingCSV"`
	InstallPlanApproval string `yaml:"installPlanApproval,omitempty" json:"installPlanApproval,omitempty"`
	OperatorGroupScope  string `yaml:"operatorGroupScope,omitempty" json:"operatorGroupScope,omitempty"`
}

type HelmSpec struct {
	Repo           string `yaml:"repo" json:"repo"`
	Chart          string `yaml:"chart" json:"chart"`
	Version        string `yaml:"version" json:"version"`
	ValuesTemplate string `yaml:"valuesTemplate,omitempty" json:"valuesTemplate,omitempty"`
}

type KustomizeSpec struct {
	Base           string `yaml:"base" json:"base"`
	ValuesTemplate string `yaml:"valuesTemplate,omitempty" json:"valuesTemplate,omitempty"`
}

type InputSpec struct {
	Name              string     `yaml:"name" json:"name"`
	Type              string     `yaml:"type" json:"type"`
	Default           any        `yaml:"default,omitempty" json:"default,omitempty"`
	Enum              []any      `yaml:"enum,omitempty" json:"enum,omitempty"`
	Required          bool       `yaml:"required,omitempty" json:"required,omitempty"`
	Sensitive         bool       `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`
	Placeholder       bool       `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	PlaceholderReason string     `yaml:"placeholderReason,omitempty" json:"placeholderReason,omitempty"`
	Generator         *Generator `yaml:"generator,omitempty" json:"generator,omitempty"`
}

type HookSpec struct {
	PreRender  string `yaml:"preRender,omitempty" json:"preRender,omitempty"`
	PostRender string `yaml:"postRender,omitempty" json:"postRender,omitempty"`
}
