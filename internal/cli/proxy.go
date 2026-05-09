package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

// resolveProxyEnv returns HTTP_PROXY/HTTPS_PROXY/NO_PROXY env values
// derived from the first environment that defines ocpInstall.proxy.
// Credentials referenced by credentialsRef are read from the resolved
// secret path and url-encoded into the proxy URLs.
// Returns nil when no environment declares a proxy.
func resolveProxyEnv(state v1alpha1.State, secretsDir string) (map[string]string, error) {
	for i := range state.Environments {
		env := state.Environments[i]
		proxy := v1alpha1.OCPInstallProxyOf(env)
		if proxy == nil || (proxy.HTTPProxy == "" && proxy.HTTPSProxy == "" && len(proxy.NoProxy) == 0) {
			continue
		}
		authority := ""
		if proxy.CredentialsRef.Name != "" {
			path := resolvedSecretPath(proxy.CredentialsRef.Name, &env, secretsDir)
			user, pass, err := readProxyCredentials(path)
			if err != nil {
				return nil, fmt.Errorf("read proxy credentials %q: %w", path, err)
			}
			authority = url.QueryEscape(user) + ":" + url.QueryEscape(pass) + "@"
		}
		httpURL := injectProxyAuthority(proxy.HTTPProxy, authority)
		httpsURL := injectProxyAuthority(proxy.HTTPSProxy, authority)
		noProxy := strings.Join(proxy.NoProxy, ",")
		out := map[string]string{}
		if httpURL != "" {
			out["HTTP_PROXY"] = httpURL
			out["http_proxy"] = httpURL
		}
		if httpsURL != "" {
			out["HTTPS_PROXY"] = httpsURL
			out["https_proxy"] = httpsURL
		}
		if noProxy != "" {
			out["NO_PROXY"] = noProxy
			out["no_proxy"] = noProxy
		}
		return out, nil
	}
	return nil, nil
}

func readProxyCredentials(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	user, pass, err := parseBMCCredentials(data)
	if err != nil {
		return "", "", err
	}
	if user == "" || pass == "" {
		return "", "", errors.New("username and password must both be non-empty")
	}
	return user, pass, nil
}

var proxySchemeAuthorityRE = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.\-]*://)([^@/]+@)?`)

func injectProxyAuthority(rawURL, authority string) string {
	if rawURL == "" || authority == "" {
		return rawURL
	}
	return proxySchemeAuthorityRE.ReplaceAllString(rawURL, "${1}"+authority)
}

// proxySummary renders a one-line, credential-redacted view of the
// effective proxy env for human-facing output.
func proxySummary(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	if v := env["HTTPS_PROXY"]; v != "" {
		parts = append(parts, "https="+redactProxyURL(v))
	}
	if v := env["HTTP_PROXY"]; v != "" {
		parts = append(parts, "http="+redactProxyURL(v))
	}
	if v := env["NO_PROXY"]; v != "" {
		parts = append(parts, "no_proxy="+v)
	}
	return strings.Join(parts, " ")
}

func redactProxyURL(u string) string {
	return proxySchemeAuthorityRE.ReplaceAllStringFunc(u, func(match string) string {
		idx := strings.Index(match, "://")
		if idx < 0 {
			return match
		}
		scheme := match[:idx+3]
		if strings.Contains(match[idx+3:], "@") {
			return scheme + "***@"
		}
		return scheme
	})
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extra))
	override := make(map[string]struct{}, len(extra))
	for k := range extra {
		override[k] = struct{}{}
	}
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			out = append(out, kv)
			continue
		}
		if _, ok := override[kv[:eq]]; ok {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}
