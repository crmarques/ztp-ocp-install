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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"github.com/crmarques/ztp-ocp-install-lab/internal/infra"
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
		newSecretsBMCCmd(stdout, stderr),
	)
	return cmd
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
		requests, err := generatedSelfSignedRequests(state)
		if err != nil {
			return failErr(1, err)
		}
		printTitle(stdout, "Secrets")
		if len(requests) == 0 {
			fmt.Fprintln(stdout, "secrets: no generated secret requests found")
			return nil
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
		}
		if err := os.Chmod(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
		}
		for _, request := range requests {
			action, err := materializeSelfSignedCertificate(secretsDir, request)
			if err != nil {
				return failErr(1, err)
			}
			printOK(stdout, request.name, action)
		}
		return nil
	}
	return cmd
}

func newSecretsPullSecretCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull-secret",
		Short: "Manage local OpenShift pull secret material",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSecretsPullSecretSetCmd(stdout, stderr))
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

func newSecretsBMCCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bmc",
		Short: "Manage local BMC credentials",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSecretsBMCSetCmd(stdout))
	return cmd
}

func newSecretsBMCSetCmd(stdout io.Writer) *cobra.Command {
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
		Short: "Store BMC credentials (username:password) for a SecretRef",
		Long: `Store BMC credentials at <secrets-dir>/<name> as a single line "username:password",
mode 0600. Three input modes are supported:

  --from-file <path>           read an existing "username:password" file
  --username <u> --password <p>      pass credentials directly
  --username <u> --password-stdin    read the password from stdin
  --generate                   generate a random password (default username "admin")

Inputs are mutually exclusive. Use --generate for test fixtures; use --from-file
or --username/--password for real BMC credentials provided by the operator.`,
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&name, "name", "", "SecretRef name to write")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "path to a file containing one line: username:password")
	cmd.Flags().StringVar(&username, "username", "", "BMC username (required with --password, --password-stdin, or --generate)")
	cmd.Flags().StringVar(&password, "password", "", "BMC password (mutually exclusive with --password-stdin and --generate)")
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
				return failErr(1, fmt.Errorf("read BMC credentials file %s: %w", fromFile, err))
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
			return failErr(1, errors.New("BMC password must not be empty"))
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
		message := fmt.Sprintf("%s BMC credentials at %s (user %q)", action, target, resolvedUser)
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
		return "", "", errors.New("BMC credentials file is empty; expected one line in the form username:password")
	}
	if strings.Contains(text, "\n") {
		return "", "", errors.New("BMC credentials file must contain a single username:password line")
	}
	idx := strings.IndexByte(text, ':')
	if idx <= 0 || idx == len(text)-1 {
		return "", "", errors.New("BMC credentials file must contain a single username:password line")
	}
	return text[:idx], text[idx+1:], nil
}

func validateBMCUsername(username string) error {
	if username == "" {
		return errors.New("BMC username must not be empty")
	}
	if strings.ContainsAny(username, ":\r\n\t ") {
		return errors.New("BMC username must not contain whitespace or ':'")
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
	byName := map[string]v1alpha1.SelfSignedCertificateSpec{}
	var names []string
	for _, cluster := range state.OCPClusters {
		for _, item := range cluster.Spec.Install.GeneratedSecrets {
			if item.Type != v1alpha1.GeneratedSecretSelfSigned || item.SelfSignedCertificate == nil {
				continue
			}
			cert := *item.SelfSignedCertificate
			cert.DNSNames = append([]string(nil), cert.DNSNames...)
			cert.IPAddresses = append([]string(nil), cert.IPAddresses...)
			if existing, ok := byName[item.Name]; ok {
				if !reflect.DeepEqual(existing, cert) {
					return nil, fmt.Errorf("generated secret %q has conflicting self-signed certificate requests", item.Name)
				}
				continue
			}
			byName[item.Name] = cert
			names = append(names, item.Name)
		}
	}
	result := make([]generatedSelfSignedRequest, 0, len(names))
	for _, name := range names {
		result = append(result, generatedSelfSignedRequest{name: name, certificate: byName[name]})
	}
	return result, nil
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
