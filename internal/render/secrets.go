package render

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/proxy"
	"github.com/crmarques/gitups/internal/secretref"
)

// InstallerSecrets holds the secret material that openshift-install expects
// inlined inside install-config.yaml. Use LoadInstallerSecrets to populate
// from the local secrets directory, or PlaceholderInstallerSecrets for the
// safe-to-inspect rendering.
type InstallerSecrets struct {
	PullSecret  string
	SSHKey      string
	TrustBundle string
	ProxyHTTP   string
	ProxyHTTPS  string
}

// PlaceholderInstallerSecrets returns sentinel strings that mark where secret
// material would land in the rendered install-config. They are safe to write
// to disk and to inspect.
func PlaceholderInstallerSecrets(ocp v1alpha1.OCPCluster) InstallerSecrets {
	out := InstallerSecrets{
		PullSecret: pullSecretPlaceholder(ocp.Spec.Install.PullSecretRef.Name),
		SSHKey:     secretRefPlaceholder("ssh-key", ocp.Spec.Install.SSHKeyRef.Name),
	}
	if ocp.Spec.Install.AdditionalTrustBundleRef.Name != "" {
		out.TrustBundle = secretRefPlaceholder("trust-bundle", ocp.Spec.Install.AdditionalTrustBundleRef.Name)
	}
	return out
}

// LoadInstallerSecrets reads secret material from the local secrets directory
// (and from file-backed Environment keys) for one OCP cluster. It mirrors the
// install_agent Ansible role: pull secret + ssh key + optional trust
// bundle, with mirror-registry auth merged into the pull secret and proxy
// credentials baked into proxy URLs.
func LoadInstallerSecrets(state v1alpha1.State, ocp v1alpha1.OCPCluster, secretsDir string) (InstallerSecrets, error) {
	env := primaryEnvironment(state)
	var out InstallerSecrets

	pullName := ocp.Spec.Install.PullSecretRef.Name
	if pullName == "" {
		return out, fmt.Errorf("%s: pullSecretRef is empty; declare Environment.spec.secrets.%s or set OCPCluster.spec.install.pullSecretRef", ocp.Metadata.Name, v1alpha1.DefaultPullSecretName)
	}
	pullPath := secretref.ResolvePath(pullName, env, secretsDir)
	pullSecret, err := readSecretFile(pullPath, "pull secret")
	if err != nil {
		return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
	}
	if err := validatePullSecret(pullSecret, pullPath); err != nil {
		return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
	}
	out.PullSecret = pullSecret

	sshName := ocp.Spec.Install.SSHKeyRef.Name
	if sshName == "" {
		return out, fmt.Errorf("%s: sshKeyRef is empty; declare Environment.spec.secrets.%s or set OCPCluster.spec.install.sshKeyRef", ocp.Metadata.Name, v1alpha1.DefaultClusterSSHKeyName)
	}
	sshPath := secretref.ResolvePath(sshName, env, secretsDir)
	sshKey, err := readSecretFile(sshPath, "ssh key")
	if err != nil {
		return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
	}
	out.SSHKey = sshKey

	if name := ocp.Spec.Install.AdditionalTrustBundleRef.Name; name != "" {
		tbPath := secretref.ResolvePath(name, env, secretsDir)
		bundle, err := readSecretFile(tbPath, "additional trust bundle")
		if err != nil {
			return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
		}
		out.TrustBundle = bundle
	}

	if env != nil {
		if v1alpha1.OCPInstallKind(*env) == v1alpha1.OCPInstallKindDisconnected {
			if registries := env.Spec.Registries; registries != nil && registries.Mirror != nil && registries.Mirror.CredentialsRef.Name != "" {
				credPath := secretref.ResolvePath(registries.Mirror.CredentialsRef.Name, env, secretsDir)
				creds, err := readUserPassFile(credPath, "mirror registry credentials")
				if err != nil {
					return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
				}
				merged, err := mergeMirrorAuth(out.PullSecret, registries.Mirror.URL, creds)
				if err != nil {
					return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
				}
				out.PullSecret = merged
			}
		}
		if eff := proxy.Resolve(state, env); eff != nil {
			fallbackURL := ""
			if eff.HTTP == "" || eff.HTTPS == "" {
				var err error
				fallbackURL, err = managedProxyClientURLForOCP(state, ocp, env)
				if err != nil {
					return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
				}
			}
			httpURL := eff.HTTP
			if httpURL == "" {
				httpURL = fallbackURL
			}
			httpsURL := eff.HTTPS
			if httpsURL == "" {
				httpsURL = fallbackURL
			}
			if eff.Auth.Name != "" && (httpURL != "" || httpsURL != "") {
				credPath := secretref.ResolvePath(eff.Auth.Name, env, secretsDir)
				creds, err := readUserPassFile(credPath, "proxy credentials")
				if err != nil {
					return out, fmt.Errorf("%s: %w", ocp.Metadata.Name, err)
				}
				if httpURL != "" {
					httpURL, err = bakeProxyCredentials(httpURL, creds)
					if err != nil {
						return out, fmt.Errorf("%s: httpProxy: %w", ocp.Metadata.Name, err)
					}
				}
				if httpsURL != "" {
					httpsURL, err = bakeProxyCredentials(httpsURL, creds)
					if err != nil {
						return out, fmt.Errorf("%s: httpsProxy: %w", ocp.Metadata.Name, err)
					}
				}
			}
			out.ProxyHTTP = httpURL
			out.ProxyHTTPS = httpsURL
		}
	}
	return out, nil
}

func readSecretFile(path, kind string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s path is empty", kind)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s at %s: %w", kind, path, err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func validatePullSecret(content, path string) error {
	var doc struct {
		Auths map[string]any `json:"auths"`
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("pull secret %s is empty", path)
	}
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return fmt.Errorf("pull secret %s is not valid JSON: %w", path, err)
	}
	if doc.Auths == nil {
		return fmt.Errorf("pull secret %s is missing .auths object", path)
	}
	return nil
}

type userPass struct {
	Username string
	Password string
}

func readUserPassFile(path, kind string) (userPass, error) {
	if path == "" {
		return userPass{}, fmt.Errorf("%s path is empty", kind)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return userPass{}, fmt.Errorf("read %s at %s: %w", kind, path, err)
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return userPass{}, fmt.Errorf("%s at %s is empty", kind, path)
	}
	sep := strings.Index(line, ":")
	if sep <= 0 || sep == len(line)-1 {
		return userPass{}, fmt.Errorf("%s at %s must be a single username:password line", kind, path)
	}
	return userPass{Username: line[:sep], Password: line[sep+1:]}, nil
}

func mergeMirrorAuth(pullSecret, registryURL string, creds userPass) (string, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(pullSecret), &doc); err != nil {
		return "", fmt.Errorf("merge mirror auth: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	auths, _ := doc["auths"].(map[string]any)
	if auths == nil {
		auths = map[string]any{}
		doc["auths"] = auths
	}
	auth := base64.StdEncoding.EncodeToString([]byte(creds.Username + ":" + creds.Password))
	auths[registryURL] = map[string]any{"auth": auth}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func bakeProxyCredentials(rawURL string, creds userPass) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("proxy URL must include scheme and host")
	}
	u.User = url.UserPassword(creds.Username, creds.Password)
	return u.String(), nil
}
