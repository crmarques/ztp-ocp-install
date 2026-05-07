package cli

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
	"github.com/crmarques/ztp-ocp-install-lab/internal/secretref"
)

type generatedSelfSignedRequest struct {
	name        string
	certificate v1alpha1.SelfSignedCertificateSpec
}

func newSecretsCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage local install secret material",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newSecretsGenerateCmd(stdout, stderr),
		newSecretsPullSecretCmd(stdout, stderr),
		newSecretsCredentialsCmd(stdout),
	)
	return cmd
}

func resolvedSecretPath(name string, env *v1alpha1.Environment, secretsDir string) string {
	return secretref.ResolvePath(name, env, secretsDir)
}

func primaryEnvironmentForSync(state v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

func newSecretsGenerateCmd(stdout io.Writer, _ io.Writer) *cobra.Command {
	var (
		files      []string
		secretsDir string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate local install secret material requested by desired state",
		Args:  cobra.NoArgs,
	}
	secretsDir = defaultSecretsDir()
	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "Gitups YAML file or directory; may be repeated")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory for generated install secret material")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(files)
		if err != nil {
			return failErr(1, err)
		}
		certRequests, err := generatedSelfSignedRequests(state)
		if err != nil {
			return failErr(1, err)
		}
		credRequests := generatedCredentialsRequestsFor(state)
		printTitle(stdout, "Secrets")
		if len(certRequests) == 0 && len(credRequests) == 0 {
			fmt.Fprintln(stdout, "secrets: no generated secret requests found")
			return nil
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
		}
		if err := os.Chmod(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
		}
		for _, request := range certRequests {
			action, err := materializeSelfSignedCertificate(secretsDir, request)
			if err != nil {
				return failErr(1, err)
			}
			printOK(stdout, request.name, action)
		}
		for _, request := range credRequests {
			action, err := materializeGeneratedCredentials(secretsDir, request)
			if err != nil {
				return failErr(1, err)
			}
			printOK(stdout, request.name, action)
		}
		return nil
	}
	return cmd
}

func materializeGeneratedCredentials(secretsDir string, request generatedCredentialsRequest) (string, error) {
	target := filepath.Join(secretsDir, request.name)
	wantUser := request.credentials.Username
	if wantUser == "" {
		wantUser = "admin"
	}
	exists, err := regularFileExists(target)
	if err != nil {
		return "", err
	}
	if exists {
		data, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read existing credentials %s: %w", target, err)
		}
		gotUser, _, perr := parseBMCCredentials(data)
		if perr != nil {
			return "", fmt.Errorf("existing credentials %s: %w; remove the file to regenerate", target, perr)
		}
		if gotUser != wantUser {
			return "", fmt.Errorf("existing credentials %q at %s use username %q but desired spec wants %q; remove %s to regenerate", request.name, target, gotUser, wantUser, target)
		}
		return "reused existing credentials", nil
	}
	password, err := generateBMCPassword()
	if err != nil {
		return "", err
	}
	payload := []byte(wantUser + ":" + password + "\n")
	if err := atomicWriteFile(target, payload, 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("generated %s (user %q)", target, wantUser), nil
}

func newSecretsPullSecretCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull-secret",
		Short: "Manage local OpenShift pull secret material",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSecretsPullSecretSetCmd(stdout, stderr))
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newSecretsPullSecretSetCmd(stdout io.Writer, _ io.Writer) *cobra.Command {
	var (
		name       string
		fromFile   string
		secretsDir string
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Store an OpenShift pull secret from a local JSON file",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&name, "name", "", "SecretRef name to write")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "path to pull secret JSON")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory for local install secret material")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		if name == "" {
			return failf(2, "--name is required")
		}
		if !infra.IsDNSLabel(name) {
			return failf(2, "--name must be a lowercase DNS label")
		}
		if fromFile == "" {
			return failf(2, "--from-file is required")
		}
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return failErr(1, fmt.Errorf("read pull secret file %s: %w", fromFile, err))
		}
		if err := validatePullSecretJSON(data); err != nil {
			return failErr(1, err)
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
		}
		if err := os.Chmod(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
		}
		target := filepath.Join(secretsDir, name)
		exists, err := regularFileExists(target)
		if err != nil {
			return failErr(1, err)
		}
		if err := atomicWriteFile(target, data, 0o600); err != nil {
			return failErr(1, err)
		}
		action := "wrote"
		if exists {
			action = "updated"
		}
		printTitle(stdout, "Secrets")
		printOK(stdout, name, fmt.Sprintf("%s pull secret at %s", action, target))
		return nil
	}
	return cmd
}

func newSecretsCredentialsCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "Manage local username:password credentials",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSecretsCredentialsSetCmd(stdout))
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newSecretsCredentialsSetCmd(stdout io.Writer) *cobra.Command {
	var (
		name          string
		fromFile      string
		username      string
		password      string
		passwordStdin bool
		generate      bool
		secretsDir    string
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Store username:password credentials for a SecretRef",
		Long: `Store credentials at <secrets-dir>/<name> as a single line "username:password",
mode 0600. Three input modes are supported:

  --from-file <path>           read an existing "username:password" file
  --username <u> --password <p>      pass credentials directly
  --username <u> --password-stdin    read the password from stdin
  --generate                   generate a random password (default username "admin")

Inputs are mutually exclusive. Use --generate for test fixtures; use --from-file
or --username/--password for real credentials provided by the operator.`,
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&name, "name", "", "SecretRef name to write")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "path to a file containing one line: username:password")
	cmd.Flags().StringVar(&username, "username", "", "username (required with --password, --password-stdin, or --generate)")
	cmd.Flags().StringVar(&password, "password", "", "password (mutually exclusive with --password-stdin and --generate)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin instead of --password")
	cmd.Flags().BoolVar(&generate, "generate", false, "generate a strong random password (intended for test fixtures)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory for local install secret material")
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		if name == "" {
			return failf(2, "--name is required")
		}
		if !infra.IsDNSLabel(name) {
			return failf(2, "--name must be a lowercase DNS label")
		}
		modes := 0
		if fromFile != "" {
			modes++
		}
		if password != "" {
			modes++
		}
		if passwordStdin {
			modes++
		}
		if generate {
			modes++
		}
		if modes == 0 {
			return failf(2, "one of --from-file, --password, --password-stdin, or --generate is required")
		}
		if modes > 1 {
			return failf(2, "--from-file, --password, --password-stdin, and --generate are mutually exclusive")
		}
		var resolvedUser, resolvedPass string
		switch {
		case fromFile != "":
			data, err := os.ReadFile(fromFile)
			if err != nil {
				return failErr(1, fmt.Errorf("read credentials file %s: %w", fromFile, err))
			}
			u, p, err := parseBMCCredentials(data)
			if err != nil {
				return failErr(1, err)
			}
			resolvedUser, resolvedPass = u, p
		case password != "":
			if username == "" {
				return failf(2, "--username is required with --password")
			}
			resolvedUser, resolvedPass = username, password
		case passwordStdin:
			if username == "" {
				return failf(2, "--username is required with --password-stdin")
			}
			stdin := c.InOrStdin()
			if stdin == nil {
				return failErr(1, errors.New("--password-stdin requires stdin"))
			}
			line, err := bufio.NewReader(stdin).ReadString('\n')
			if err != nil && line == "" {
				return failErr(1, fmt.Errorf("read password from stdin: %w", err))
			}
			resolvedUser, resolvedPass = username, strings.TrimRight(line, "\r\n")
		case generate:
			resolvedUser = username
			if resolvedUser == "" {
				resolvedUser = "admin"
			}
			generated, err := generateBMCPassword()
			if err != nil {
				return failErr(1, err)
			}
			resolvedPass = generated
		}
		if err := validateBMCUsername(resolvedUser); err != nil {
			return failErr(1, err)
		}
		if resolvedPass == "" {
			return failErr(1, errors.New("password must not be empty"))
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
		}
		if err := os.Chmod(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
		}
		target := filepath.Join(secretsDir, name)
		exists, err := regularFileExists(target)
		if err != nil {
			return failErr(1, err)
		}
		payload := []byte(resolvedUser + ":" + resolvedPass + "\n")
		if err := atomicWriteFile(target, payload, 0o600); err != nil {
			return failErr(1, err)
		}
		action := "wrote"
		if exists {
			action = "updated"
		}
		printTitle(stdout, "Secrets")
		message := fmt.Sprintf("%s credentials at %s (user %q)", action, target, resolvedUser)
		if generate {
			message += " — password generated; copy it from the file above before sharing"
		}
		printOK(stdout, name, message)
		return nil
	}
	return cmd
}

func parseBMCCredentials(data []byte) (string, string, error) {
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return "", "", errors.New("credentials file is empty; expected one line in the form username:password")
	}
	if strings.Contains(text, "\n") {
		return "", "", errors.New("credentials file must contain a single username:password line")
	}
	idx := strings.IndexByte(text, ':')
	if idx <= 0 || idx == len(text)-1 {
		return "", "", errors.New("credentials file must contain a single username:password line")
	}
	return text[:idx], text[idx+1:], nil
}

func validateBMCUsername(username string) error {
	if username == "" {
		return errors.New("username must not be empty")
	}
	if strings.ContainsAny(username, ":\r\n\t ") {
		return errors.New("username must not contain whitespace or ':'")
	}
	return nil
}

func generateBMCPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate BMC password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validatePullSecretJSON(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("pull secret must be valid JSON with top-level .auths object: %w", err)
	}
	authsRaw, ok := root["auths"]
	if !ok {
		return errors.New("pull secret must contain top-level .auths object")
	}
	var auths map[string]json.RawMessage
	if err := json.Unmarshal(authsRaw, &auths); err != nil {
		return errors.New("pull secret .auths must be a JSON object")
	}
	if auths == nil {
		return errors.New("pull secret .auths must be a JSON object")
	}
	return nil
}

func generatedSelfSignedRequests(state v1alpha1.State) ([]generatedSelfSignedRequest, error) {
	env := primaryEnvironmentForSync(state)
	if env == nil {
		return nil, nil
	}
	names := make([]string, 0, len(env.Spec.Keys))
	for name, key := range env.Spec.Keys {
		if key.Generated == nil || key.Generated.SelfSignedCertificate == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]generatedSelfSignedRequest, 0, len(names))
	for _, name := range names {
		cert := *env.Spec.Keys[name].Generated.SelfSignedCertificate
		cert.DNSNames = append([]string(nil), cert.DNSNames...)
		cert.IPAddresses = append([]string(nil), cert.IPAddresses...)
		result = append(result, generatedSelfSignedRequest{name: name, certificate: cert})
	}
	return result, nil
}

type generatedCredentialsRequest struct {
	name        string
	credentials v1alpha1.GeneratedCredentialsSpec
}

func generatedCredentialsRequestsFor(state v1alpha1.State) []generatedCredentialsRequest {
	env := primaryEnvironmentForSync(state)
	if env == nil {
		return nil
	}
	names := make([]string, 0, len(env.Spec.Keys))
	for name, key := range env.Spec.Keys {
		if key.Generated == nil || key.Generated.Credentials == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]generatedCredentialsRequest, 0, len(names))
	for _, name := range names {
		out = append(out, generatedCredentialsRequest{name: name, credentials: *env.Spec.Keys[name].Generated.Credentials})
	}
	return out
}

func materializeSelfSignedCertificate(secretsDir string, request generatedSelfSignedRequest) (string, error) {
	certPath := filepath.Join(secretsDir, request.name)
	keyPath := certPath + ".key"
	certExists, err := regularFileExists(certPath)
	if err != nil {
		return "", err
	}
	keyExists, err := regularFileExists(keyPath)
	if err != nil {
		return "", err
	}
	if certExists != keyExists {
		return "", fmt.Errorf("generated self-signed certificate %q is partially present; expected both %s and %s", request.name, certPath, keyPath)
	}
	if certExists {
		if err := verifySelfSignedCertificateMatchesRequest(certPath, request.certificate); err != nil {
			return "", fmt.Errorf("existing self-signed certificate %q at %s no longer matches the desired spec: %w; remove %s and %s to regenerate", request.name, certPath, err, certPath, keyPath)
		}
		return "reused existing certificate and key", nil
	}
	certPEM, keyPEM, err := selfSignedCertificatePEM(request.certificate)
	if err != nil {
		return "", err
	}
	if err := writeNewFile(certPath, certPEM, 0o600); err != nil {
		return "", err
	}
	if err := writeNewFile(keyPath, keyPEM, 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", err
	}
	return fmt.Sprintf("generated %s and %s", certPath, keyPath), nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory; expected a file", path)
	}
	return true, nil
}

func selfSignedCertificatePEM(source v1alpha1.SelfSignedCertificateSpec) ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("generate private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	dnsNames, ipAddresses := certificateSANs(source)
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: source.CommonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, source.ValidityDays),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	var certPEM bytes.Buffer
	if err := pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return nil, nil, fmt.Errorf("encode certificate: %w", err)
	}
	var keyPEM bytes.Buffer
	if err := pem.Encode(&keyPEM, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
		return nil, nil, fmt.Errorf("encode private key: %w", err)
	}
	return certPEM.Bytes(), keyPEM.Bytes(), nil
}

func verifySelfSignedCertificateMatchesRequest(certPath string, source v1alpha1.SelfSignedCertificateSpec) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("certificate is not PEM-encoded")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	if cert.Subject.CommonName != source.CommonName {
		return fmt.Errorf("commonName drift: got %q, want %q", cert.Subject.CommonName, source.CommonName)
	}
	wantDNS, wantIP := certificateSANs(source)
	gotDNS := normalizeStringSet(cert.DNSNames)
	expectedDNS := normalizeStringSet(wantDNS)
	if !reflect.DeepEqual(gotDNS, expectedDNS) {
		return fmt.Errorf("dnsNames drift: got %v, want %v", gotDNS, expectedDNS)
	}
	gotIP := normalizeIPSet(cert.IPAddresses)
	expectedIP := normalizeIPSet(wantIP)
	if !reflect.DeepEqual(gotIP, expectedIP) {
		return fmt.Errorf("ipAddresses drift: got %v, want %v", gotIP, expectedIP)
	}
	return nil
}

func normalizeStringSet(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeIPSet(in []net.IP) []string {
	out := make([]string, 0, len(in))
	for _, ip := range in {
		out = append(out, ip.String())
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func certificateSANs(source v1alpha1.SelfSignedCertificateSpec) ([]string, []net.IP) {
	dnsNames := append([]string(nil), source.DNSNames...)
	ipAddressStrings := append([]string(nil), source.IPAddresses...)
	if len(dnsNames) == 0 && len(ipAddressStrings) == 0 {
		if _, err := netip.ParseAddr(source.CommonName); err == nil {
			ipAddressStrings = append(ipAddressStrings, source.CommonName)
		} else if source.CommonName != "" {
			dnsNames = append(dnsNames, source.CommonName)
		}
	}
	ipAddresses := make([]net.IP, 0, len(ipAddressStrings))
	for _, item := range ipAddressStrings {
		address, err := netip.ParseAddr(item)
		if err != nil {
			continue
		}
		ipAddresses = append(ipAddresses, net.IP(append([]byte(nil), address.AsSlice()...)))
	}
	return dnsNames, ipAddresses
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempPath := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temp file for %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	cleanup = false
	return nil
}
