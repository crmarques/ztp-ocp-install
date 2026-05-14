// Package load decodes and validates on-disk bootwright YAML documents.
package load

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/safepath"
)

func DetectKind(path string) (v1.TypeMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return v1.TypeMeta{}, fmt.Errorf("read %s: %w", path, err)
	}
	var tm v1.TypeMeta
	if err := yaml.Unmarshal(raw, &tm); err != nil {
		return v1.TypeMeta{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return tm, nil
}

// PackageSet loads a GitOpsPackageSet without resolving spec.extends.
func PackageSet(path string) (*v1.GitOpsPackageSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := rejectOldGitOpsPackageSetFields(raw); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	var p v1.GitOpsPackageSet
	if err := decodeKnown(raw, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ValidatePackageSet(&p); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &p, nil
}

// PackageSetResolved loads a GitOpsPackageSet and merges it with the base
// declared via spec.extends. Only one level of extends is supported. Sources
// merge by name (duplicates error); repositories merge by name with env
// entries replacing base. The returned set never carries spec.extends.
func PackageSetResolved(path string) (*v1.GitOpsPackageSet, *v1.ExtendedFrom, error) {
	env, err := PackageSet(path)
	if err != nil {
		return nil, nil, err
	}
	if env.Spec.Extends == nil {
		return env, nil, nil
	}
	ext := env.Spec.Extends
	if ext.Source == "" {
		return nil, nil, fmt.Errorf("%s: spec.extends.source is required", path)
	}
	if strings.HasPrefix(ext.Source, "git+") {
		return nil, nil, fmt.Errorf("%s: spec.extends.source %q: git+ sources are not yet supported; use a filesystem path",
			path, ext.Source)
	}
	if strings.Contains(ext.Source, "://") {
		return nil, nil, fmt.Errorf("%s: spec.extends.source %q: only filesystem paths are supported today",
			path, ext.Source)
	}
	basePath := ext.Source
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(filepath.Dir(path), basePath)
	}
	base, err := PackageSet(basePath)
	if err != nil {
		return nil, nil, fmt.Errorf("load base package set %s: %w", basePath, err)
	}
	if base.Spec.Extends != nil {
		return nil, nil, fmt.Errorf("%s: base package set %s also declares spec.extends; only one level of extends is supported",
			path, basePath)
	}
	merged, err := mergeGitOpsPackageSets(base, env)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: merge with base %s: %w", path, basePath, err)
	}
	return merged, &v1.ExtendedFrom{Source: ext.Source, Ref: ext.Ref}, nil
}

func mergeGitOpsPackageSets(base, env *v1.GitOpsPackageSet) (*v1.GitOpsPackageSet, error) {
	out := &v1.GitOpsPackageSet{
		APIVersion: env.APIVersion,
		Kind:       env.Kind,
		Metadata:   env.Metadata,
		Spec: v1.GitOpsPackageSetSpec{
			EnvKey: env.Spec.EnvKey,
		},
	}

	seenSource := map[string]bool{}
	for _, s := range base.Spec.Sources {
		out.Spec.Sources = append(out.Spec.Sources, s)
		seenSource[s.Name] = true
	}
	for _, s := range env.Spec.Sources {
		if seenSource[s.Name] {
			return nil, fmt.Errorf("source %q: conflicting definitions in base and env provisions", s.Name)
		}
		seenSource[s.Name] = true
		out.Spec.Sources = append(out.Spec.Sources, s)
	}

	repoIdxByName := map[string]int{}
	for i, r := range base.Spec.Repositories {
		repoIdxByName[r.Name] = i
	}
	repos := append([]v1.RepositoryDecl(nil), base.Spec.Repositories...)
	for _, envR := range env.Spec.Repositories {
		if idx, ok := repoIdxByName[envR.Name]; ok {
			repos[idx] = envR
			continue
		}
		repos = append(repos, envR)
	}
	out.Spec.Repositories = repos
	return out, nil
}

func rejectOldGitOpsPackageSetFields(raw []byte) error {
	doc, err := decodeMap(raw)
	if err != nil {
		return err
	}
	spec := childMap(doc, "spec")
	if spec == nil {
		return nil
	}
	if _, ok := spec["packages"]; ok {
		return fmt.Errorf("spec.packages is no longer supported; declare packages under spec.repositories[].packages")
	}
	repos, _ := spec["repositories"].([]any)
	for i, rv := range repos {
		repo, _ := rv.(map[string]any)
		if repo == nil {
			continue
		}
		if _, ok := repo["members"]; ok {
			return fmt.Errorf("spec.repositories[%d].members is no longer supported; use packages/resources", i)
		}
		pkgs, _ := repo["packages"].([]any)
		for j, pv := range pkgs {
			pkg, _ := pv.(map[string]any)
			if pkg == nil {
				continue
			}
			for _, old := range []string{"method", "components", "component", "componentValues"} {
				if _, ok := pkg[old]; ok {
					return fmt.Errorf("spec.repositories[%d].packages[%d].%s is no longer supported", i, j, old)
				}
			}
		}
	}
	return nil
}

func rejectOldPackageFields(raw []byte) error {
	doc, err := decodeMap(raw)
	if err != nil {
		return err
	}
	spec := childMap(doc, "spec")
	for _, old := range []string{
		"inputs", "dependsOn", "installation", "components", "defaultComponents",
		"methods", "targetRepo", "renderer", "olm", "helm", "kustomize",
	} {
		if _, ok := spec[old]; ok {
			return fmt.Errorf("spec.%s belongs in install/resources descriptor.yaml or is no longer supported", old)
		}
	}
	return nil
}

func rejectOldDescriptorFields(raw []byte) error {
	doc, err := decodeMap(raw)
	if err != nil {
		return err
	}
	spec := childMap(doc, "spec")
	for _, old := range []string{
		"targetRepo", "installation", "components", "defaultComponents",
		"defaultInstall", "defaultResources", "methods",
	} {
		if _, ok := spec[old]; ok {
			return fmt.Errorf("spec.%s is not valid in descriptor.yaml", old)
		}
	}
	return nil
}

func decodeMap(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func childMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	child, _ := m[key].(map[string]any)
	if child == nil {
		return map[string]any{}
	}
	return child
}

func ExpandedPackageSet(path string) (*v1.GitOpsPackageSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var fp v1.GitOpsPackageSet
	if err := decodeKnown(raw, &fp); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ValidateExpandedPackageSet(&fp); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &fp, nil
}

func PackageDefinition(path string) (*v1.PackageDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := rejectOldPackageFields(raw); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	var pd v1.PackageDefinition
	if err := decodeKnown(raw, &pd); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ValidatePackageDefinition(&pd); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &pd, nil
}

func PackageDescriptor(path string) (*v1.PackageDescriptor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := rejectOldDescriptorFields(raw); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	var d v1.PackageDescriptor
	if err := decodeKnown(raw, &d); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ValidatePackageDescriptor(&d); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &d, nil
}

func decodeKnown(raw []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	return decoder.Decode(out)
}

func ValidatePackageSet(p *v1.GitOpsPackageSet) error {
	if p.APIVersion != v1.APIVersion {
		return fmt.Errorf("unsupported apiVersion %q; want %q", p.APIVersion, v1.APIVersion)
	}
	if p.Kind != v1.KindGitOpsPackageSet {
		return fmt.Errorf("kind %q; want %q", p.Kind, v1.KindGitOpsPackageSet)
	}
	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	names := map[string]bool{}
	for i, s := range p.Spec.Sources {
		if s.Name == "" {
			return fmt.Errorf("spec.sources[%d].name is required", i)
		}
		if err := safepath.Name(fmt.Sprintf("spec.sources[%d].name", i), s.Name); err != nil {
			return err
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate source name %q", s.Name)
		}
		names[s.Name] = true
		set := 0
		if s.Filesystem != nil {
			set++
			if s.Filesystem.Path == "" {
				return fmt.Errorf("spec.sources[%d].filesystem.path is required", i)
			}
		}
		if s.OCI != nil {
			set++
			if s.OCI.Registry == "" {
				return fmt.Errorf("spec.sources[%d].oci.registry is required", i)
			}
			if s.OCI.Digest == "" {
				return fmt.Errorf("spec.sources[%d].oci.digest is required", i)
			}
		}
		if s.Git != nil {
			set++
			if s.Git.URL == "" {
				return fmt.Errorf("spec.sources[%d].git.url is required", i)
			}
			if s.Git.Ref == "" {
				return fmt.Errorf("spec.sources[%d].git.ref is required (tag or commit SHA; branches rejected)", i)
			}
			if s.Git.Path != "" {
				if _, err := safepath.Relative(fmt.Sprintf("spec.sources[%d].git.path", i), s.Git.Path); err != nil {
					return err
				}
			}
		}
		if set != 1 {
			return fmt.Errorf("spec.sources[%d] must set exactly one of filesystem, oci, or git", i)
		}
	}
	repoNames := map[string]bool{}
	for i, r := range p.Spec.Repositories {
		if r.Name == "" {
			return fmt.Errorf("spec.repositories[%d].name is required", i)
		}
		if repoNames[r.Name] {
			return fmt.Errorf("duplicate repository name %q", r.Name)
		}
		repoNames[r.Name] = true
		if err := validateRepoName(r.Name); err != nil {
			return fmt.Errorf("spec.repositories[%d].name %q: %w", i, r.Name, err)
		}
		switch r.Type {
		case v1.RepoTypeKubernetesResources:
			envRepo := r.RepoRef != nil
			if envRepo {
				if r.RepoRef.Name == "" {
					return fmt.Errorf("spec.repositories[%d] (%s): repoRef.name is required when repoRef is set", i, r.Name)
				}
			} else {
				if len(r.Packages) == 0 {
					return fmt.Errorf("spec.repositories[%d] (%s): packages is required on a generic %s repository", i, r.Name, v1.RepoTypeKubernetesResources)
				}
			}
			if r.ManagedServiceRef != nil {
				return fmt.Errorf("spec.repositories[%d] (%s): managedServiceRef is only valid on %s repositories", i, r.Name, v1.RepoTypeServiceResources)
			}
			if err := validateRepositoryPackages(fmt.Sprintf("spec.repositories[%d].packages", i), r.Packages, envRepo); err != nil {
				return err
			}
		case v1.RepoTypeServiceResources:
			if r.RepoRef != nil {
				return fmt.Errorf("spec.repositories[%d] (%s): repoRef is not valid on %s repositories", i, r.Name, v1.RepoTypeServiceResources)
			}
			if len(r.Packages) > 0 {
				return fmt.Errorf("spec.repositories[%d] (%s): %s repositories do not carry packages[] (content is user-authored declarest payloads)", i, r.Name, v1.RepoTypeServiceResources)
			}
			if r.ManagedServiceRef == nil {
				return fmt.Errorf("spec.repositories[%d] (%s): managedServiceRef is required on %s repositories", i, r.Name, v1.RepoTypeServiceResources)
			}
			if r.ManagedServiceRef.Repo == "" {
				return fmt.Errorf("spec.repositories[%d] (%s): managedServiceRef.repo is required", i, r.Name)
			}
			if r.ManagedServiceRef.Instance == "" {
				return fmt.Errorf("spec.repositories[%d] (%s): managedServiceRef.instance is required", i, r.Name)
			}
		default:
			return fmt.Errorf("spec.repositories[%d].type %q invalid (supported: %q, %q)", i, r.Type, v1.RepoTypeKubernetesResources, v1.RepoTypeServiceResources)
		}
	}
	if err := validateControllers(p.Spec.Controllers, repoNames); err != nil {
		return err
	}
	for i, r := range p.Spec.Repositories {
		if r.Type != v1.RepoTypeServiceResources || r.ManagedServiceRef == nil {
			continue
		}
		if !repoNames[r.ManagedServiceRef.Repo] {
			return fmt.Errorf("spec.repositories[%d] (%s): managedServiceRef.repo %q is not declared in spec.repositories", i, r.Name, r.ManagedServiceRef.Repo)
		}
	}
	return nil
}

func validateControllers(c *v1.Controllers, repoNames map[string]bool) error {
	if c == nil {
		return nil
	}
	check := func(field string, a *v1.ControllerAssignment) error {
		if a == nil {
			return nil
		}
		if a.Repo == "" {
			return fmt.Errorf("spec.controllers.%s.repo is required", field)
		}
		if a.Instance == "" {
			return fmt.Errorf("spec.controllers.%s.instance is required", field)
		}
		if !repoNames[a.Repo] {
			return fmt.Errorf("spec.controllers.%s.repo %q is not declared in spec.repositories", field, a.Repo)
		}
		return nil
	}
	if err := check("kubernetesResources", c.KubernetesResources); err != nil {
		return err
	}
	if err := check("serviceResources", c.ServiceResources); err != nil {
		return err
	}
	return nil
}

func validateRepositoryPackages(prefix string, packages []v1.PackageRef, envRepo bool) error {
	for i, pr := range packages {
		pp := fmt.Sprintf("%s[%d]", prefix, i)
		if pr.Template == "" {
			return fmt.Errorf("%s.template is required", pp)
		}
		if _, err := safepath.Relative(pp+".template", pr.Template); err != nil {
			return err
		}
		if pr.InstallMethod != "" && !validRenderer(pr.InstallMethod) {
			return fmt.Errorf("%s.installMethod %q invalid (olm | kustomize | helm | raw)", pp, pr.InstallMethod)
		}
		if envRepo && pr.InstallMethod != "" {
			return fmt.Errorf("%s.installMethod is only valid on generic %s repositories (no repoRef)", pp, v1.RepoTypeKubernetesResources)
		}
		if !envRepo && len(pr.Resources) > 0 {
			return fmt.Errorf("%s.resources is only valid on env %s repositories (with repoRef)", pp, v1.RepoTypeKubernetesResources)
		}
		for j, r := range pr.Resources {
			rp := fmt.Sprintf("%s.resources[%d]", pp, j)
			if r.Template == "" {
				return fmt.Errorf("%s.template is required", rp)
			}
			if _, err := safepath.Relative(rp+".template", r.Template); err != nil {
				return err
			}
			if r.Name == "" {
				return fmt.Errorf("%s.name is required", rp)
			}
			if err := safepath.Name(rp+".name", r.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validRenderer(r string) bool {
	switch r {
	case v1.RendererOLM, v1.RendererKustomize, v1.RendererHelm, v1.RendererRaw:
		return true
	}
	return false
}

// validateRepoName ensures a repo name (post-{{.Env}} substitution) is safe
// to embed as a Kubernetes resource name and filesystem path.
func validateRepoName(name string) error {
	const maxLen = 253
	stripped := strings.ReplaceAll(name, "{{.Env}}", "x")
	if stripped == "" {
		return fmt.Errorf("name must not be empty after {{.Env}} substitution")
	}
	if len(stripped) > maxLen {
		return fmt.Errorf("name exceeds %d characters after {{.Env}} substitution", maxLen)
	}
	for i, r := range stripped {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return fmt.Errorf("illegal character %q at position %d (lowercase alphanumeric, '-', '.', and the literal '{{.Env}}' token only)", r, i)
		}
	}
	if stripped[0] == '-' || stripped[0] == '.' {
		return fmt.Errorf("must not start with '-' or '.'")
	}
	last := stripped[len(stripped)-1]
	if last == '-' || last == '.' {
		return fmt.Errorf("must not end with '-' or '.'")
	}
	return nil
}

func validateCompatibility(pd *v1.PackageDefinition) error {
	c := pd.Spec.Compatibility
	if c == nil {
		return nil
	}
	for i, v := range c.Kubernetes {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("spec.compatibility.kubernetes[%d]: entry must not be empty", i)
		}
	}
	return nil
}

// IsScaffold reports whether a GitOpsPackageSet still carries the init-time
// empty sources/repositories.
func IsScaffold(p *v1.GitOpsPackageSet) bool {
	return len(p.Spec.Sources) == 0 && len(p.Spec.Repositories) == 0
}

func ValidateExpandedPackageSet(fp *v1.GitOpsPackageSet) error {
	if fp.APIVersion != v1.APIVersion {
		return fmt.Errorf("unsupported apiVersion %q; want %q", fp.APIVersion, v1.APIVersion)
	}
	if fp.Kind != v1.KindGitOpsPackageSet {
		return fmt.Errorf("kind %q; want %q", fp.Kind, v1.KindGitOpsPackageSet)
	}
	if fp.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if fp.Spec.Resolved == nil {
		return fmt.Errorf("spec.resolved is required")
	}
	if fp.Spec.Resolved.Repository.Layout == "" {
		return fmt.Errorf("spec.resolved.repository.layout is required")
	}
	if fp.Spec.Resolved.Repository.Layout != "split" {
		return fmt.Errorf("spec.resolved.repository.layout %q unsupported (v0.1: split only)", fp.Spec.Resolved.Repository.Layout)
	}
	if len(fp.Spec.Resolved.Packages) == 0 {
		return fmt.Errorf("spec.resolved.packages must contain at least one package")
	}
	seen := map[string]bool{}
	for i, rp := range fp.Spec.Resolved.Packages {
		prefix := fmt.Sprintf("spec.resolved.packages[%d]", i)
		if rp.Instance == "" {
			return fmt.Errorf("%s.instance is required", prefix)
		}
		if seen[rp.Instance] {
			return fmt.Errorf("duplicate instance %q", rp.Instance)
		}
		seen[rp.Instance] = true
		switch rp.UnitType {
		case v1.UnitTypeInstall:
			if rp.InstallMethod == "" {
				return fmt.Errorf("%s.installMethod is required for unitType=install", prefix)
			}
		case v1.UnitTypeResource:
			if rp.ResourceTemplate == "" {
				return fmt.Errorf("%s.resourceTemplate is required for unitType=resource", prefix)
			}
			if rp.ResourceName == "" {
				return fmt.Errorf("%s.resourceName is required for unitType=resource", prefix)
			}
		default:
			return fmt.Errorf("%s.unitType %q invalid (install | resource)", prefix, rp.UnitType)
		}
		if rp.Repository == "" {
			return fmt.Errorf("%s.repository is required", prefix)
		}
		if rp.Renderer == "" {
			return fmt.Errorf("%s.renderer is required", prefix)
		}
		if rp.RenderedPaths.Repo == "" || rp.RenderedPaths.Dir == "" {
			return fmt.Errorf("%s.renderedPaths.repo and renderedPaths.dir are required", prefix)
		}
		if err := safepath.Name(prefix+".renderedPaths.repo", rp.RenderedPaths.Repo); err != nil {
			return err
		}
		if _, err := safepath.Relative(prefix+".renderedPaths.dir", rp.RenderedPaths.Dir); err != nil {
			return err
		}
	}
	return nil
}

func ValidatePackageDefinition(pd *v1.PackageDefinition) error {
	if pd.APIVersion != v1.APIVersion {
		return fmt.Errorf("unsupported apiVersion %q; want %q", pd.APIVersion, v1.APIVersion)
	}
	if pd.Kind != v1.KindPackageDefinition {
		return fmt.Errorf("kind %q; want %q", pd.Kind, v1.KindPackageDefinition)
	}
	if pd.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	switch pd.Spec.Role {
	case v1.RoleKRC, v1.RoleSRC, v1.RoleWorkload:
	default:
		return fmt.Errorf("spec.role %q invalid (%s | %s | %s)",
			pd.Spec.Role, v1.RoleKRC, v1.RoleSRC, v1.RoleWorkload)
	}
	if pd.Spec.DefaultInstall == "" {
		return fmt.Errorf("spec.defaultInstall is required")
	}
	if !validRenderer(pd.Spec.DefaultInstall) {
		return fmt.Errorf("spec.defaultInstall %q invalid (olm | kustomize | helm | raw)", pd.Spec.DefaultInstall)
	}
	if err := validateControllerCLI(pd); err != nil {
		return err
	}
	if err := validateReadinessShape(pd); err != nil {
		return err
	}
	if err := validateDeclarestBundle(pd); err != nil {
		return err
	}
	if err := validateCompatibility(pd); err != nil {
		return err
	}
	seenResource := map[string]bool{}
	for i, r := range pd.Spec.DefaultResources {
		if r.Template == "" {
			return fmt.Errorf("spec.defaultResources[%d].template is required", i)
		}
		if r.Name == "" {
			return fmt.Errorf("spec.defaultResources[%d].name is required", i)
		}
		key := r.Template + "/" + r.Name
		if seenResource[key] {
			return fmt.Errorf("duplicate default resource %q", key)
		}
		seenResource[key] = true
	}
	return nil
}

func validateControllerCLI(pd *v1.PackageDefinition) error {
	cli := pd.Spec.CLI
	switch pd.Spec.Role {
	case v1.RoleSRC:
		if cli == nil || cli.Binary == "" {
			return fmt.Errorf("spec.cli.binary is required for role %q", pd.Spec.Role)
		}
	case v1.RoleWorkload:
		if cli != nil {
			return fmt.Errorf("spec.cli is only valid for controller packages (role %q or %q)",
				v1.RoleKRC, v1.RoleSRC)
		}
	}
	return nil
}

func validateReadinessShape(pd *v1.PackageDefinition) error {
	if pd.Spec.Role == v1.RoleKRC || pd.Spec.Role == v1.RoleSRC {
		if len(pd.Spec.Readiness) == 0 {
			return fmt.Errorf("spec.readiness[] is required for role %q", pd.Spec.Role)
		}
	}
	for i, c := range pd.Spec.Readiness {
		if c.Kind == "" || c.Name == "" || c.Condition == "" {
			return fmt.Errorf("spec.readiness[%d]: kind, name, and condition are all required", i)
		}
	}
	return nil
}

func validateDeclarestBundle(pd *v1.PackageDefinition) error {
	db := pd.Spec.DeclarestBundle
	if db == nil {
		return nil
	}
	if db.Name == "" {
		return fmt.Errorf("spec.declarestBundle.name is required")
	}
	if db.Version == "" {
		return fmt.Errorf("spec.declarestBundle.version is required")
	}
	return nil
}

func ValidatePackageDescriptor(d *v1.PackageDescriptor) error {
	if d.APIVersion != v1.APIVersion {
		return fmt.Errorf("unsupported apiVersion %q; want %q", d.APIVersion, v1.APIVersion)
	}
	if d.Kind != v1.KindPackageDescriptor {
		return fmt.Errorf("kind %q; want %q", d.Kind, v1.KindPackageDescriptor)
	}
	inputNames := map[string]bool{}
	for i, in := range d.Spec.Inputs {
		if in.Name == "" {
			return fmt.Errorf("spec.inputs[%d].name is required", i)
		}
		if in.Type == "" {
			return fmt.Errorf("spec.inputs[%d].type is required", i)
		}
		if inputNames[in.Name] {
			return fmt.Errorf("duplicate input %q", in.Name)
		}
		inputNames[in.Name] = true
		if in.Generator != nil {
			if err := validateGenerator(fmt.Sprintf("spec.inputs[%d] (%s).generator", i, in.Name), *in.Generator); err != nil {
				return err
			}
			if !in.Placeholder && !in.Required {
				return fmt.Errorf("spec.inputs[%d] (%s): generator requires placeholder=true or required=true", i, in.Name)
			}
		}
	}
	return validateRenderable("spec", d.Spec.Renderer, d.Spec.OLM, d.Spec.Helm, d.Spec.Kustomize)
}

func validateGenerator(prefix string, g v1.Generator) error {
	switch g.Kind {
	case v1.GeneratorRandomHex:
		if g.Length < 0 || g.Length > 256 {
			return fmt.Errorf("%s: length %d out of range (0-256)", prefix, g.Length)
		}
		if g.Length%2 != 0 {
			return fmt.Errorf("%s: length %d must be even for kind %q", prefix, g.Length, g.Kind)
		}
	case v1.GeneratorRandomBase64:
		if g.Length < 0 || g.Length > 256 {
			return fmt.Errorf("%s: length %d out of range (0-256)", prefix, g.Length)
		}
	case v1.GeneratorUUID:
		if g.Length != 0 {
			return fmt.Errorf("%s: length is not configurable for kind %q", prefix, g.Kind)
		}
		if g.Charset != "" {
			return fmt.Errorf("%s: charset is not configurable for kind %q", prefix, g.Kind)
		}
	case "":
		return fmt.Errorf("%s: kind is required", prefix)
	default:
		return fmt.Errorf("%s: unknown kind %q (want %q | %q | %q)", prefix, g.Kind, v1.GeneratorRandomHex, v1.GeneratorRandomBase64, v1.GeneratorUUID)
	}
	return nil
}

func validateRenderable(prefix, renderer string, olm *v1.OLMSpec, helm *v1.HelmSpec, kustomize *v1.KustomizeSpec) error {
	switch renderer {
	case v1.RendererOLM:
		if olm == nil {
			return fmt.Errorf("%s.olm is required when renderer=olm", prefix)
		}
		if olm.Package == "" || olm.Channel == "" || olm.Source == "" || olm.SourceNamespace == "" || olm.StartingCSV == "" {
			return fmt.Errorf("%s.olm: package, channel, source, sourceNamespace, and startingCSV are all required", prefix)
		}
		if olm.InstallPlanApproval != "" && olm.InstallPlanApproval != "Automatic" && olm.InstallPlanApproval != "Manual" {
			return fmt.Errorf("%s.olm.installPlanApproval %q invalid (Automatic | Manual)", prefix, olm.InstallPlanApproval)
		}
		switch olm.OperatorGroupScope {
		case "", "AllNamespaces", "OwnNamespace", "SingleNamespace":
		default:
			return fmt.Errorf("%s.olm.operatorGroupScope %q invalid (AllNamespaces | OwnNamespace | SingleNamespace)", prefix, olm.OperatorGroupScope)
		}
	case v1.RendererHelm:
		if helm == nil {
			return fmt.Errorf("%s.helm is required when renderer=helm", prefix)
		}
		if helm.Chart == "" || helm.Version == "" || helm.Repo == "" {
			return fmt.Errorf("%s.helm: repo, chart, and version are all required", prefix)
		}
	case v1.RendererKustomize:
		if kustomize == nil {
			return fmt.Errorf("%s.kustomize is required when renderer=kustomize", prefix)
		}
		if kustomize.Base == "" {
			return fmt.Errorf("%s.kustomize.base is required", prefix)
		}
	case v1.RendererRaw:
	default:
		return fmt.Errorf("%s.renderer %q invalid (olm | kustomize | helm | raw)", prefix, renderer)
	}
	return nil
}
