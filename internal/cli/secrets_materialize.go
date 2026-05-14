package cli

import (
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
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/crmarques/bootwright/api/v1alpha1"
)

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
	names := make([]string, 0, len(env.Spec.Secrets))
	for name, secret := range env.Spec.Secrets {
		if secret.Generated == nil || secret.Generated.SelfSignedCertificate == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]generatedSelfSignedRequest, 0, len(names))
	for _, name := range names {
		cert := *env.Spec.Secrets[name].Generated.SelfSignedCertificate
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
	names := make([]string, 0, len(env.Spec.Secrets))
	for name, secret := range env.Spec.Secrets {
		if secret.Generated == nil || secret.Generated.Credentials == nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]generatedCredentialsRequest, 0, len(names))
	for _, name := range names {
		out = append(out, generatedCredentialsRequest{name: name, credentials: *env.Spec.Secrets[name].Generated.Credentials})
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
