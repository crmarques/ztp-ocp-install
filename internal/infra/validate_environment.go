package infra

import (
	"fmt"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func validateEnvironments(envs []v1alpha1.Environment) []string {
	var errs []string
	if len(envs) == 0 {
		errs = append(errs, "at least one Environment is required")
	}
	if len(envs) > 1 {
		errs = append(errs, "exactly one Environment is supported in this release")
	}
	seen := map[string]bool{}
	for _, env := range envs {
		if e := validateName("Environment", env.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[env.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate Environment %q", env.Metadata.Name))
		}
		seen[env.Metadata.Name] = true
		if env.Spec.BaseDomain == "" {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.baseDomain is required", env.Metadata.Name))
		}
		errs = append(errs, validateOCPInstall(env)...)
		errs = append(errs, validateEnvironmentSecrets(env)...)
		errs = append(errs, validateComponentImages(env)...)
	}
	return errs
}

func validateEnvironmentSecrets(env v1alpha1.Environment) []string {
	var errs []string
	for name, secret := range env.Spec.Secrets {
		if !dnsLabel.MatchString(name) {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets entry %q is not a DNS label", env.Metadata.Name, name))
			continue
		}
		hasFile := secret.File != ""
		hasGenerated := secret.Generated != nil
		switch {
		case hasFile && hasGenerated:
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s] sets both file and generated; pick exactly one source", env.Metadata.Name, name))
		case !hasFile && !hasGenerated:
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s] requires a source (file or generated)", env.Metadata.Name, name))
		case hasGenerated:
			errs = append(errs, validateGeneratedSecret(env.Metadata.Name, name, secret.Generated)...)
		}
	}
	return errs
}

func validateGeneratedSecret(envName, secretName string, gen *v1alpha1.EnvironmentSecretGenerated) []string {
	var errs []string
	hasCreds := gen.Credentials != nil
	hasCert := gen.SelfSignedCertificate != nil
	switch {
	case hasCreds && hasCert:
		errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated sets both credentials and selfSignedCertificate; pick exactly one", envName, secretName))
	case !hasCreds && !hasCert:
		errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated requires one of {credentials, selfSignedCertificate}", envName, secretName))
	case hasCert:
		if gen.SelfSignedCertificate.CommonName == "" {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated.selfSignedCertificate.commonName is required", envName, secretName))
		}
		if gen.SelfSignedCertificate.ValidityDays < 0 {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated.selfSignedCertificate.validityDays must not be negative", envName, secretName))
		}
	case hasCreds:
		username := gen.Credentials.Username
		if username != "" && strings.TrimSpace(username) != username {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated.credentials.username must not contain leading or trailing whitespace", envName, secretName))
		}
		if strings.ContainsAny(username, ":\r\n\t ") {
			errs = append(errs, fmt.Sprintf("Environment/%s spec.secrets[%s].generated.credentials.username must not contain whitespace, colon, or newlines", envName, secretName))
		}
	}
	return errs
}

func validateComponentImages(env v1alpha1.Environment) []string {
	var errs []string
	for category, types := range env.Spec.ComponentImages {
		if category == "" || !dnsLabel.MatchString(category) {
			errs = append(errs, fmt.Sprintf("Environment/%s componentImages key %q is not a DNS label", env.Metadata.Name, category))
			continue
		}
		for typ, image := range types {
			if typ == "" || !dnsLabel.MatchString(typ) {
				errs = append(errs, fmt.Sprintf("Environment/%s componentImages[%s] key %q is not a DNS label", env.Metadata.Name, category, typ))
				continue
			}
			if image.Local == "" && image.Public == "" {
				errs = append(errs, fmt.Sprintf("Environment/%s componentImages[%s][%s] requires at least one of local or public", env.Metadata.Name, category, typ))
			}
		}
	}
	return errs
}

func validateOCPInstall(env v1alpha1.Environment) []string {
	var errs []string
	switch env.Spec.OCPInstallType {
	case "", v1alpha1.OCPInstallKindConnected, v1alpha1.OCPInstallKindDisconnected:
	default:
		return []string{fmt.Sprintf("Environment/%s spec.ocpInstallType %q must be one of {%s, %s}",
			env.Metadata.Name, env.Spec.OCPInstallType,
			v1alpha1.OCPInstallKindConnected, v1alpha1.OCPInstallKindDisconnected)}
	}
	requireMirror := v1alpha1.OCPInstallKind(env) == v1alpha1.OCPInstallKindDisconnected
	errs = append(errs, validateRegistriesBlock(env, env.Spec.Registries, requireMirror)...)
	errs = append(errs, validateEnvironmentProxy(env)...)
	return errs
}

func validateEnvironmentProxy(env v1alpha1.Environment) []string {
	p := env.Spec.Proxy
	if p == nil {
		return nil
	}
	var errs []string
	owner := fmt.Sprintf("Environment/%s spec.proxy", env.Metadata.Name)
	if p.Auth != nil && p.Auth.ProxyAuthRef.Name != "" && !dnsLabel.MatchString(p.Auth.ProxyAuthRef.Name) {
		errs = append(errs, fmt.Sprintf("%s.auth.proxyAuthRef.name %q is not a DNS label", owner, p.Auth.ProxyAuthRef.Name))
	}
	for _, field := range []struct{ name, value string }{
		{"http", p.HTTP},
		{"https", p.HTTPS},
	} {
		if field.value == "" {
			continue
		}
		if proxyURLHasInlineCredentials(field.value) {
			errs = append(errs, fmt.Sprintf("%s.%s must not embed credentials; use auth.proxyAuthRef and supply the bare URL", owner, field.name))
		}
	}
	return errs
}

func proxyURLHasInlineCredentials(url string) bool {
	idx := strings.Index(url, "://")
	if idx < 0 {
		return false
	}
	authority := url[idx+3:]
	if at := strings.Index(authority, "@"); at >= 0 {
		if slash := strings.Index(authority, "/"); slash < 0 || at < slash {
			return true
		}
	}
	return false
}

func validateRegistriesBlock(env v1alpha1.Environment, registries *v1alpha1.EnvironmentRegistriesSpec, requireMirror bool) []string {
	var errs []string
	owner := fmt.Sprintf("Environment/%s spec.registries", env.Metadata.Name)
	if registries == nil {
		if requireMirror {
			errs = append(errs, fmt.Sprintf("%s.mirror and trust material are required when ocpInstallType=%s", owner, v1alpha1.OCPInstallKindDisconnected))
		}
		return errs
	}
	if requireMirror && registries.Mirror == nil {
		errs = append(errs, fmt.Sprintf("%s.mirror is required when ocpInstallType=%s", owner, v1alpha1.OCPInstallKindDisconnected))
	}
	if registries.Mirror != nil {
		if registries.Mirror.URL == "" {
			errs = append(errs, fmt.Sprintf("%s.mirror.url is required", owner))
		}
		if requireMirror && registries.Mirror.TrustBundleRef.Name == "" {
			errs = append(errs, fmt.Sprintf("%s.mirror.trustBundleRef is required", owner))
		}
	}
	if requireMirror {
		errs = append(errs, validateDisconnectedRegistrySources(env, registries)...)
	}
	return errs
}

func validateDisconnectedRegistrySources(env v1alpha1.Environment, registries *v1alpha1.EnvironmentRegistriesSpec) []string {
	var errs []string
	if registries == nil || registries.Mirror == nil {
		return errs
	}
	owner := fmt.Sprintf("Environment/%s spec.registries", env.Metadata.Name)
	for _, src := range registries.ImageDigestSources {
		errs = append(errs, validateImageDigestSource(owner, src)...)
		errs = append(errs, validateMirrorRefs(fmt.Sprintf("%s.imageDigestSources[%s]", owner, src.Source), registries.Mirror.URL, src.Mirrors)...)
		if src.SourcePolicy == v1alpha1.ImageSourcePolicyAllow {
			errs = append(errs, fmt.Sprintf("%s.imageDigestSources[%s].sourcePolicy must not allow contacting the source when disconnected", owner, src.Source))
		}
	}
	return errs
}
