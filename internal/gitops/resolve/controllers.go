package resolve

import (
	"fmt"

	v1 "github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/gitops/catalog"
)

const (
	intentManagedRepo   = "managed-repo"
	intentManagedScript = "managed-script"
)

func resolveControllers(
	p *v1.GitOpsPackageSet,
	cat *catalog.Catalog,
	envKey string,
	fp *v1.GitOpsPackageSet,
	priorByInstance map[string]*v1.ResolvedPackage,
	forceMode bool,
) ([]v1.Placeholder, error) {
	if err := rewireManagedScripts(p, cat, fp); err != nil {
		return nil, err
	}
	if err := rewireSRCIntents(p, cat, fp); err != nil {
		return nil, err
	}
	if p.Spec.Controllers == nil || p.Spec.Controllers.KubernetesResources == nil {
		return nil, nil
	}
	assignment := p.Spec.Controllers.KubernetesResources

	entry, template, instance, err := findControllerPackage(p, cat, assignment.Repo, assignment.Instance)
	if err != nil {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources: %w", err)
	}
	if entry.Def.Spec.Role != v1.RoleKRC {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources: package %q has role %q; expected %q",
			entry.Def.Metadata.Name, entry.Def.Spec.Role, v1.RoleKRC)
	}
	unit, ok := entry.LookupDomainUnit(v1.DomainKRC, intentManagedRepo)
	if !ok {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources: package %q does not implement the %q intent (missing %s/%s/)",
			entry.Def.Metadata.Name, intentManagedRepo, v1.DomainKRC, intentManagedRepo)
	}

	krcGenericRepo := assignment.Repo

	var krcEnvRepos []v1.RepositoryDecl
	for _, r := range p.Spec.Repositories {
		if r.Type == v1.RepoTypeKubernetesResources && r.RepoRef != nil && r.RepoRef.Name == krcGenericRepo {
			krcEnvRepos = append(krcEnvRepos, r)
		}
	}
	if len(krcEnvRepos) == 0 {
		return nil, fmt.Errorf("spec.controllers.kubernetesResources: no env repository references generic repo %q; the KRC has no home for managed-repo units",
			krcGenericRepo)
	}

	// Service-resources repos are reconciled by the SRC, not by the KRC.
	var outputRepos []string
	seen := map[string]bool{}
	for _, r := range p.Spec.Repositories {
		if r.Type == v1.RepoTypeServiceResources {
			continue
		}
		name := substituteEnv(r.Name, envKey)
		if seen[name] {
			continue
		}
		seen[name] = true
		outputRepos = append(outputRepos, name)
	}

	var allPhs []v1.Placeholder
	for _, envRepo := range krcEnvRepos {
		krcEnvRepoName := substituteEnv(envRepo.Name, envKey)
		for _, target := range outputRepos {
			if target == krcGenericRepo {
				// KRC cannot reconcile its own install; bootstrap applies it directly.
				continue
			}
			resName := target
			resInstance := resourceInstance(instance, intentManagedRepo, resName)

			r := ref{
				template:         template,
				packageInstance:  instance,
				instance:         resInstance,
				repository:       krcEnvRepoName,
				unitType:         v1.UnitTypeResource,
				domain:           v1.DomainKRC,
				resourceTemplate: intentManagedRepo,
				resourceName:     resName,
				entry:            entry,
				unit:             unit,
				values:           map[string]any{},
			}
			rp, phs, err := resolveOne(r, priorByInstance[resInstance], forceMode)
			if err != nil {
				return nil, fmt.Errorf("spec.controllers.kubernetesResources (%s): %w", target, err)
			}
			rp.Controller = &v1.ControllerBinding{
				Kind:     v1.RoleKRC,
				Instance: instance,
				Template: template,
				Intent:   intentManagedRepo,
			}
			fp.Spec.Resolved.Packages = append(fp.Spec.Resolved.Packages, rp)
			allPhs = append(allPhs, phs...)
		}
	}
	return allPhs, nil
}

// rewireManagedScripts points every script-style resource unit at the SRC's
// managed-script intent without mutating the unit's identity or values, so
// per-field placeholder tracking is preserved.
func rewireManagedScripts(p *v1.GitOpsPackageSet, cat *catalog.Catalog, fp *v1.GitOpsPackageSet) error {
	var scriptIdx []int
	for i := range fp.Spec.Resolved.Packages {
		rp := &fp.Spec.Resolved.Packages[i]
		if rp.UnitType != v1.UnitTypeResource {
			continue
		}
		entry, ok := cat.Lookup(rp.Template)
		if !ok {
			continue
		}
		u, ok := entry.LookupDomainUnit(rp.Domain, rp.ResourceTemplate)
		if !ok {
			continue
		}
		if u.Descriptor.ProvisioningStyle == v1.ProvisioningStyleScript {
			scriptIdx = append(scriptIdx, i)
		}
	}
	if len(scriptIdx) == 0 {
		return nil
	}

	if p.Spec.Controllers == nil || p.Spec.Controllers.ServiceResources == nil {
		return fmt.Errorf("spec.controllers.serviceResources is required: %d script-style resource unit(s) need an SRC to provision them",
			len(scriptIdx))
	}
	srcEntry, srcTemplate, srcInstance, err := findControllerPackage(p, cat, p.Spec.Controllers.ServiceResources.Repo, p.Spec.Controllers.ServiceResources.Instance)
	if err != nil {
		return fmt.Errorf("spec.controllers.serviceResources: %w", err)
	}
	if srcEntry.Def.Spec.Role != v1.RoleSRC {
		return fmt.Errorf("spec.controllers.serviceResources: package %q has role %q; expected %q",
			srcEntry.Def.Metadata.Name, srcEntry.Def.Spec.Role, v1.RoleSRC)
	}
	if _, ok := srcEntry.LookupDomainUnit(v1.DomainSRC, intentManagedScript); !ok {
		return fmt.Errorf("spec.controllers.serviceResources: package %q does not implement the %q intent (missing %s/%s/)",
			srcEntry.Def.Metadata.Name, intentManagedScript, v1.DomainSRC, intentManagedScript)
	}

	for _, i := range scriptIdx {
		rp := &fp.Spec.Resolved.Packages[i]
		rp.Controller = &v1.ControllerBinding{
			Kind:     v1.RoleSRC,
			Instance: srcInstance,
			Template: srcTemplate,
			Intent:   intentManagedScript,
		}
	}
	return nil
}

// rewireSRCIntents tags resource units whose template matches an SRC
// bootstrap-sync intent: bootwright apply also invokes the SRC CLI for these.
func rewireSRCIntents(p *v1.GitOpsPackageSet, cat *catalog.Catalog, fp *v1.GitOpsPackageSet) error {
	if p.Spec.Controllers == nil || p.Spec.Controllers.ServiceResources == nil {
		return nil
	}
	srcEntry, srcTemplate, srcInstance, err := findControllerPackage(p, cat, p.Spec.Controllers.ServiceResources.Repo, p.Spec.Controllers.ServiceResources.Instance)
	if err != nil {
		return fmt.Errorf("spec.controllers.serviceResources: %w", err)
	}
	if srcEntry.Def.Spec.CLI == nil || len(srcEntry.Def.Spec.CLI.Intents) == 0 {
		return nil
	}
	intents := srcEntry.Def.Spec.CLI.Intents
	for i := range fp.Spec.Resolved.Packages {
		rp := &fp.Spec.Resolved.Packages[i]
		if rp.UnitType != v1.UnitTypeResource {
			continue
		}
		if rp.Controller != nil {
			continue
		}
		if _, ok := intents[rp.ResourceTemplate]; !ok {
			continue
		}
		rp.Controller = &v1.ControllerBinding{
			Kind:     v1.RoleSRC,
			Instance: srcInstance,
			Template: srcTemplate,
			Intent:   rp.ResourceTemplate,
		}
	}
	return nil
}

func findControllerPackage(p *v1.GitOpsPackageSet, cat *catalog.Catalog, repoName, instance string) (catalog.Entry, string, string, error) {
	for _, r := range p.Spec.Repositories {
		if r.Type != v1.RepoTypeKubernetesResources || r.RepoRef != nil || r.Name != repoName {
			continue
		}
		for _, pr := range r.Packages {
			entry, resolvedInstance, err := packageEntry(pr, cat)
			if err != nil {
				continue
			}
			if resolvedInstance == instance {
				return entry, pr.Template, resolvedInstance, nil
			}
		}
		return catalog.Entry{}, "", "", fmt.Errorf("instance %q not selected in generic repo %q", instance, repoName)
	}
	return catalog.Entry{}, "", "", fmt.Errorf("generic repo %q not declared in GitOpsPackageSet", repoName)
}
