package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
)

// registryProbeTimeout caps each HTTP request the verify command issues.
// Disconnected lab registries should respond fast; a long timeout here just
// turns a typo or wrong URL into a slow failure.
const registryProbeTimeout = 15 * time.Second

func newRegistryCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Inspect the local mirror registry that backs disconnected installs",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newRegistryVerifyCmd(stdout, stderr))
	return cmd
}

func newRegistryVerifyCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var (
		files      []string
		secretsDir string
		insecure   bool
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the local mirror registry has the OCP images required by a disconnected install",
		Long: `verify reads Environment.spec.ocpInstall.disconnected.registries from the
loaded state, resolves the mirror URL, mirror credentials, and trust bundle
from the local secrets directory, and queries the OCI v2 manifest API for the
release image referenced by Environment.openshift.release.version under each
configured mirror path. Per-source results are printed; exit code is 1 when
any required image is missing.`,
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "Gitups YAML file or directory; may be repeated")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory containing mirror credentials and CA")
	cmd.Flags().BoolVar(&insecure, "insecure-skip-tls-verify", false, "skip TLS verification (use only when probing self-signed registries without --secrets-dir/<trust>)")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(files)
		if err != nil {
			return failErr(1, err)
		}
		if len(state.Environments) == 0 {
			return failf(1, "no Environment found in -f input")
		}
		env := state.Environments[0]
		if v1alpha1.OCPInstallKind(env) != v1alpha1.OCPInstallKindDisconnected {
			return failf(1, "Environment/%s ocpInstall is %q; registry verify only applies to disconnected installs", env.Metadata.Name, v1alpha1.OCPInstallKind(env))
		}
		registries := v1alpha1.OCPInstallRegistriesOf(env)
		if registries == nil || registries.Mirror == nil || registries.Mirror.URL == "" {
			return failf(1, "Environment/%s ocpInstall.disconnected.registries.mirror.url is required", env.Metadata.Name)
		}
		creds, err := loadRegistryCredentials(secretsDir, registries.Mirror.CredentialsRef.Name)
		if err != nil {
			return failErr(1, err)
		}
		client, err := buildRegistryHTTPClient(secretsDir, registries.Mirror, insecure)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Registry Verify")
		fmt.Fprintf(stdout, "registry: %s\n", registries.Mirror.URL)
		fmt.Fprintf(stdout, "credentialsRef: %s\n", registries.Mirror.CredentialsRef.Name)
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		failed := 0
		probes := buildRegistryProbes(state, registries)
		if len(probes) == 0 {
			fmt.Fprintln(stdout, "no required images derived from desired state")
			return nil
		}
		for _, p := range probes {
			ok, detail, perr := probeRegistryImage(ctx, client, p, creds)
			if perr != nil {
				printFail(stdout, p.label, perr.Error())
				failed++
				continue
			}
			if ok {
				printOK(stdout, p.label, detail)
				continue
			}
			printFail(stdout, p.label, detail)
			failed++
		}
		if failed > 0 {
			fmt.Fprintf(stderr, "registry verify: %d image(s) missing or unreachable\n", failed)
			return silentExit(1)
		}
		fmt.Fprintf(stdout, "registry verify: all %d image(s) reachable\n", len(probes))
		return nil
	}
	return cmd
}

type registryProbe struct {
	label     string
	scheme    string
	host      string
	repo      string
	reference string
}

// buildRegistryProbes produces one probe per imageDigestSources mirror entry,
// using the matching OCP release version as the reference (since disconnected
// installs require the release-images mirror path to carry the release tag).
// Each probe queries the v2 manifest API at <scheme>://<host>/v2/<repo>/manifests/<reference>.
func buildRegistryProbes(state v1alpha1.State, registries *v1alpha1.OCPInstallRegistries) []registryProbe {
	var version string
	if len(state.Environments) > 0 && state.Environments[0].Spec.OpenShift.Release != nil {
		version = state.Environments[0].Spec.OpenShift.Release.Version
	}
	references := map[string]string{
		v1alpha1.OCPReleaseSourceQuayOCPRelease: version + "-x86_64",
		v1alpha1.OCPReleaseSourceQuayARTDev:     "",
	}
	scheme := registryScheme(registries.Mirror)
	var probes []registryProbe
	seen := map[string]bool{}
	for _, src := range registries.ImageDigestSources {
		ref, hasRef := references[src.Source]
		for _, mirror := range src.Mirrors {
			host, repo := splitMirrorRef(mirror)
			if host == "" || repo == "" {
				continue
			}
			reference := ref
			if !hasRef {
				continue
			}
			if reference == "" {
				// art-dev images are referenced by digest only; skip when the
				// mirror catalog is what we're checking, since there is no
				// stable tag the release version maps to. We still want to
				// confirm the repository exists.
				key := fmt.Sprintf("%s|%s|tags", host, repo)
				if !seen[key] {
					seen[key] = true
					probes = append(probes, registryProbe{
						label:     fmt.Sprintf("%s tag list", mirror),
						scheme:    scheme,
						host:      host,
						repo:      repo,
						reference: "",
					})
				}
				continue
			}
			key := fmt.Sprintf("%s|%s|%s", host, repo, reference)
			if seen[key] {
				continue
			}
			seen[key] = true
			probes = append(probes, registryProbe{
				label:     fmt.Sprintf("%s:%s", mirror, reference),
				scheme:    scheme,
				host:      host,
				repo:      repo,
				reference: reference,
			})
		}
	}
	sort.Slice(probes, func(i, j int) bool { return probes[i].label < probes[j].label })
	return probes
}

func splitMirrorRef(ref string) (host string, repo string) {
	idx := strings.Index(ref, "/")
	if idx < 0 {
		return ref, ""
	}
	return ref[:idx], ref[idx+1:]
}

// registryScheme picks https for any mirror URL that includes a port (assumed
// TLS-terminated) or starts with "https://". Bare-name registries fall back to
// https as the most secure default; users can flip to http via a non-standard
// mirror URL containing "http://" prefix.
func registryScheme(mirror *v1alpha1.OCPInstallRegistryMirror) string {
	if mirror == nil {
		return "https"
	}
	if strings.HasPrefix(mirror.URL, "http://") {
		return "http"
	}
	return "https"
}

type registryCredentials struct {
	username string
	password string
}

func (c registryCredentials) basicAuth() string {
	if c.username == "" && c.password == "" {
		return ""
	}
	token := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	return "Basic " + token
}

func loadRegistryCredentials(secretsDir, name string) (registryCredentials, error) {
	if name == "" {
		return registryCredentials{}, nil
	}
	path := filepath.Join(secretsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return registryCredentials{}, fmt.Errorf("read mirror credentials %s: %w", path, err)
	}
	user, pass, err := parseBMCCredentials(data)
	if err != nil {
		return registryCredentials{}, fmt.Errorf("parse mirror credentials %s: %w", path, err)
	}
	return registryCredentials{username: user, password: pass}, nil
}

func buildRegistryHTTPClient(secretsDir string, mirror *v1alpha1.OCPInstallRegistryMirror, insecure bool) (*http.Client, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: insecure}
	if !insecure && mirror != nil && mirror.TrustBundle != nil {
		ref := registryTrustRefName(mirror.TrustBundle)
		if ref != "" {
			path := filepath.Join(secretsDir, ref)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read mirror trust bundle %s: %w", path, err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(data) {
				return nil, fmt.Errorf("mirror trust bundle %s: no PEM certificates parsed", path)
			}
			tlsConfig.RootCAs = pool
		}
	}
	return &http.Client{
		Timeout: registryProbeTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

func registryTrustRefName(tb *v1alpha1.OCPInstallRegistryTrustCA) string {
	if tb == nil {
		return ""
	}
	if tb.BundleRef != nil {
		return tb.BundleRef.Name
	}
	if tb.GeneratedSelfSigned != nil {
		return tb.GeneratedSelfSigned.SecretRef.Name
	}
	return ""
}

// probeRegistryImage queries the registry v2 manifest API for either a
// specific reference or, when reference is empty, the tag list of the
// repository. Returns ok=true on HTTP 200, ok=false on 404, and err on
// transport failure or any other status.
func probeRegistryImage(ctx context.Context, client *http.Client, p registryProbe, creds registryCredentials) (bool, string, error) {
	endpoint := p.scheme + "://" + p.host + "/v2/" + p.repo + "/manifests/" + p.reference
	if p.reference == "" {
		endpoint = p.scheme + "://" + p.host + "/v2/" + p.repo + "/tags/list"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json")
	if auth := creds.basicAuth(); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return true, fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)), nil
	case http.StatusNotFound:
		return false, fmt.Sprintf("%d %s — image not present in mirror", resp.StatusCode, http.StatusText(resp.StatusCode)), nil
	case http.StatusUnauthorized:
		return false, "401 Unauthorized — check `gitups secrets bmc set --name <credentialsRef>` material", errors.New("unauthorized")
	default:
		return false, fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)), nil
	}
}
