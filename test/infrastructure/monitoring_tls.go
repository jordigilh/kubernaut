package infrastructure

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	monitoringTLSSecretName       = "monitoring-tls"
	monitoringWebConfigSecretName = "monitoring-web-config"
	monitoringCAConfigMapName     = "monitoring-ca"
	monitoringClientCASecretName  = "monitoring-client-ca"
	monitoringCAIssuerSecretName  = "monitoring-ca-issuer"
	monitoringCAIssuerName        = "monitoring-ca-issuer"
	monitoringCAMinLifetime       = 364 * 24 * time.Hour
	monitoringCAMinRemaining      = time.Hour
)

// MonitoringCAPath returns the host-side trust path for the monitoring CA.
func MonitoringCAPath(kubeconfigPath string) string {
	return filepath.Join(filepath.Dir(kubeconfigPath), "monitoring-ca.pem")
}

func NewMonitoringTLSAwareClient(caPath string) (*http.Client, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read monitoring CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("monitoring CA contains no certificates")
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}},
	}, nil
}

func monitoringCABytesMatch(hostPEM []byte, encodedSecretCA string) bool {
	secretPEM, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedSecretCA))
	return err == nil && bytes.Equal(hostPEM, secretPEM)
}

func monitoringCertificateValid(encoded string) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return false
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	return err == nil && time.Until(certificate.NotAfter) >= time.Hour
}

func monitoringTLSCacheMatches(ctx context.Context, kubeconfigPath, applicationNamespace, monitoringNamespace string) (bool, error) {
	hostPEM, err := os.ReadFile(MonitoringCAPath(kubeconfigPath))
	if err != nil {
		return false, fmt.Errorf("read host monitoring CA: %w", err)
	}

	monitoringSecretJSON, err := kubectlJSON(ctx, kubeconfigPath, "-n", monitoringNamespace, "get", "secret", monitoringTLSSecretName, "-o", "json")
	if err != nil {
		return false, fmt.Errorf("read monitoring TLS Secret: %w", err)
	}
	clientSecretJSON, err := kubectlJSON(ctx, kubeconfigPath, "-n", applicationNamespace, "get", "secret", monitoringClientCASecretName, "-o", "json")
	if err != nil {
		return false, fmt.Errorf("read monitoring client CA Secret: %w", err)
	}
	var monitoringSecret, clientSecret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(monitoringSecretJSON, &monitoringSecret); err != nil {
		return false, fmt.Errorf("decode monitoring TLS Secret: %w", err)
	}
	if err := json.Unmarshal(clientSecretJSON, &clientSecret); err != nil {
		return false, fmt.Errorf("decode monitoring client CA Secret: %w", err)
	}
	monitoringCA := monitoringSecret.Data["ca.crt"]
	clientCA := clientSecret.Data["ca.crt"]
	if !monitoringCABytesMatch(hostPEM, monitoringCA) {
		return false, nil
	}
	if !monitoringCertificateValid(monitoringCA) || !monitoringCertificateValid(monitoringSecret.Data["tls.crt"]) {
		return false, nil
	}
	decodedClientCA, err := base64.StdEncoding.DecodeString(strings.TrimSpace(clientCA))
	if err != nil {
		return false, fmt.Errorf("decode monitoring client CA: %w", err)
	}
	return bytes.Equal(hostPEM, decodedClientCA), nil
}

// BuildDemoMonitoringCAIssuerSecretManifest renders the stable administrator-owned
// monitoring CA Secret consumed by the cert-manager Issuer in each demo cluster.
// DD-PLATFORM-012 deliberately keeps this CA shared while issuing independent
// monitoring leaves per cluster.
func BuildDemoMonitoringCAIssuerSecretManifest(namespace string, certificatePEM, keyPEM []byte) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
`, monitoringCAIssuerSecretName, namespace,
		base64.StdEncoding.EncodeToString(certificatePEM),
		base64.StdEncoding.EncodeToString(keyPEM))
}

// BuildDemoMonitoringTLSManifest renders the per-cluster cert-manager Issuer,
// Certificate, and independent web configuration Secret. cert-manager owns the
// monitoring-tls target Secret and rebuilds its data map during issuance, so
// web-config.yml must not share that Secret (DD-PLATFORM-012).
func BuildDemoMonitoringTLSManifest(namespace string) string {
	dnsNames := []string{
		"localhost",
		"prometheus-svc",
		"prometheus-svc." + namespace,
		"prometheus-svc." + namespace + ".svc",
		"prometheus-svc." + namespace + ".svc.cluster.local",
		"alertmanager-svc",
		"alertmanager-svc." + namespace,
		"alertmanager-svc." + namespace + ".svc",
		"alertmanager-svc." + namespace + ".svc.cluster.local",
		"thanos-querier-svc",
		"thanos-querier-svc." + namespace,
		"thanos-querier-svc." + namespace + ".svc",
		"thanos-querier-svc." + namespace + ".svc.cluster.local",
	}
	var dnsNamesYAML strings.Builder
	for _, name := range dnsNames {
		_, _ = fmt.Fprintf(&dnsNamesYAML, "    - %s\n", name)
	}
	webConfig := `tls_server_config:
  cert_file: /etc/monitoring-tls/tls.crt
  key_file: /etc/monitoring-tls/tls.key
`

	return fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
type: Opaque
stringData:
  web-config.yml: |
%s
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: %s
  namespace: %s
spec:
  ca:
    secretName: %s
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
spec:
  secretName: %s
  duration: 2160h
  renewBefore: 720h
  commonName: monitoring-services
  ipAddresses:
    - 127.0.0.1
  dnsNames:
%s  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always
  usages:
    - server auth
  issuerRef:
    name: %s
    kind: Issuer
    group: cert-manager.io
`, monitoringWebConfigSecretName, namespace, indentYAMLBlock(webConfig, 4),
		monitoringCAIssuerName, namespace, monitoringCAIssuerSecretName,
		monitoringTLSSecretName, namespace, monitoringTLSSecretName,
		dnsNamesYAML.String(), monitoringCAIssuerName)
}

// BuildDemoMonitoringTrustManifest renders the public CA projections consumed
// by application clients, monitoring workloads, and host-side probes.
func BuildDemoMonitoringTrustManifest(applicationNamespace, monitoringNamespace string, caPEM []byte) string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
data:
  ca.crt: |
%s
`, monitoringCAConfigMapName, applicationNamespace, indentPEM(string(caPEM)))
	if monitoringNamespace != applicationNamespace {
		manifest += fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
data:
  ca.crt: |
%s
`, monitoringCAConfigMapName, monitoringNamespace, indentPEM(string(caPEM)))
	}
	manifest += fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/demo-owner: setup-demo-infra
type: Opaque
data:
  ca.crt: %s
`, monitoringClientCASecretName, applicationNamespace, base64.StdEncoding.EncodeToString(caPEM))
	return manifest
}

// ensureDemoMonitoringCertificateDependencies installs the certificate and
// reload controllers in both clusters. The hub installation is intentionally
// idempotent because the demo OIDC path already installs these dependencies.
func ensureDemoMonitoringCertificateDependencies(ctx context.Context, hubKubeconfigPath, spokeKubeconfigPath string, writer io.Writer) error {
	clusters := []struct {
		name           string
		kubeconfigPath string
	}{
		{name: "hub", kubeconfigPath: hubKubeconfigPath},
		{name: "spoke", kubeconfigPath: spokeKubeconfigPath},
	}
	for _, cluster := range clusters {
		if err := InstallCertManager(ctx, cluster.kubeconfigPath, writer); err != nil {
			return fmt.Errorf("install cert-manager in %s cluster: %w", cluster.name, err)
		}
		if err := WaitForCertManagerReady(ctx, cluster.kubeconfigPath, writer); err != nil {
			return fmt.Errorf("wait for cert-manager in %s cluster: %w", cluster.name, err)
		}
		if err := installReloader(ctx, cluster.kubeconfigPath, writer); err != nil {
			return fmt.Errorf("install Reloader in %s cluster: %w", cluster.name, err)
		}
	}
	return nil
}

// EnsureDemoMonitoringTLS provisions the shared monitoring CA and independent
// cert-manager-managed serving leaves in the hub and spoke demo clusters.
// DD-PLATFORM-012 replaces the old one-time hub leaf copy with a trust-root
// bootstrap plus per-cluster Certificates.
func EnsureDemoMonitoringTLS(ctx context.Context, hubKubeconfigPath, spokeKubeconfigPath, applicationNamespace, monitoringNamespace string, writer io.Writer) error {
	caPEM, keyPEM, caCert, err := loadOrCreateDemoMonitoringCA(ctx, hubKubeconfigPath, monitoringNamespace, writer)
	if err != nil {
		return fmt.Errorf("prepare demo monitoring CA: %w", err)
	}

	clusters := []struct {
		name           string
		kubeconfigPath string
	}{
		{name: "hub", kubeconfigPath: hubKubeconfigPath},
		{name: "spoke", kubeconfigPath: spokeKubeconfigPath},
	}
	for _, cluster := range clusters {
		if err := kubectlApplyManifest(ctx, cluster.kubeconfigPath, writer, BuildDemoMonitoringCAIssuerSecretManifest(monitoringNamespace, caPEM, keyPEM)); err != nil {
			return fmt.Errorf("apply demo monitoring CA Secret in %s cluster: %w", cluster.name, err)
		}
		if err := kubectlApplyManifest(ctx, cluster.kubeconfigPath, writer, BuildDemoMonitoringTrustManifest(applicationNamespace, monitoringNamespace, caPEM)); err != nil {
			return fmt.Errorf("apply demo monitoring trust projections in %s cluster: %w", cluster.name, err)
		}
		if err := os.WriteFile(MonitoringCAPath(cluster.kubeconfigPath), caPEM, 0o600); err != nil {
			return fmt.Errorf("write %s host monitoring CA: %w", cluster.name, err)
		}
		if err := removeStaleDemoMonitoringTLSSecret(ctx, cluster.kubeconfigPath, monitoringNamespace, caCert, writer); err != nil {
			return fmt.Errorf("validate existing monitoring TLS Secret in %s cluster: %w", cluster.name, err)
		}
		if err := kubectlApplyManifest(ctx, cluster.kubeconfigPath, writer, BuildDemoMonitoringTLSManifest(monitoringNamespace)); err != nil {
			return fmt.Errorf("apply demo monitoring Certificate in %s cluster: %w", cluster.name, err)
		}
		if err := runKubectl(ctx, cluster.kubeconfigPath, writer, "wait", "--for=condition=Ready", "certificate/"+monitoringTLSSecretName, "-n", monitoringNamespace, "--timeout=120s"); err != nil {
			return fmt.Errorf("wait for demo monitoring Certificate in %s cluster: %w", cluster.name, err)
		}
	}

	_, _ = fmt.Fprintln(writer, "  ✅ demo monitoring CA and independent cert-manager leaves ready in hub and spoke clusters")
	return nil
}

func loadOrCreateDemoMonitoringCA(ctx context.Context, kubeconfigPath, monitoringNamespace string, writer io.Writer) ([]byte, []byte, *x509.Certificate, error) {
	secretJSON, err := kubectlJSON(ctx, kubeconfigPath, "-n", monitoringNamespace, "get", "secret", monitoringCAIssuerSecretName, "--ignore-not-found", "-o", "json")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read existing monitoring CA Secret: %w", err)
	}
	if len(bytes.TrimSpace(secretJSON)) > 0 {
		var secret struct {
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal(secretJSON, &secret); err != nil {
			_, _ = fmt.Fprintf(writer, "  ⚠️  existing monitoring CA Secret is not valid JSON; replacing it: %v\n", err)
		} else {
			certificatePEM, certificateErr := base64.StdEncoding.DecodeString(strings.TrimSpace(secret.Data["tls.crt"]))
			keyPEM, keyErr := base64.StdEncoding.DecodeString(strings.TrimSpace(secret.Data["tls.key"]))
			if certificateErr == nil && keyErr == nil {
				caCert, parseErr := parseMonitoringCACertificate(certificatePEM, keyPEM)
				if parseErr == nil {
					return certificatePEM, keyPEM, caCert, nil
				}
				_, _ = fmt.Fprintf(writer, "  ⚠️  existing monitoring CA Secret is stale or invalid; replacing it: %v\n", parseErr)
			} else {
				_, _ = fmt.Fprintf(writer, "  ⚠️  existing monitoring CA Secret has invalid key material; replacing it: certificate=%v key=%v\n", certificateErr, keyErr)
			}
		}
	}

	ca, err := generateMonitoringCAWithValidity(365 * 24 * time.Hour)
	if err != nil {
		return nil, nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(ca.key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal demo monitoring CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildDemoMonitoringCAIssuerSecretManifest(monitoringNamespace, ca.certificatePEM, keyPEM)); err != nil {
		return nil, nil, nil, fmt.Errorf("apply generated monitoring CA Secret: %w", err)
	}
	return ca.certificatePEM, keyPEM, ca.certificate, nil
}

func parseMonitoringCACertificate(certificatePEM, keyPEM []byte) (*x509.Certificate, error) {
	certificate, err := parseMonitoringCACertificatePEM(certificatePEM)
	if err != nil {
		return nil, err
	}
	if err := validateMonitoringCACertificate(certificate); err != nil {
		return nil, err
	}
	privateKey, err := parseMonitoringCAKey(keyPEM)
	if err != nil {
		return nil, err
	}
	if err := validateMonitoringCAKeyPair(certificate, privateKey); err != nil {
		return nil, err
	}
	return certificate, nil
}

func parseMonitoringCACertificatePEM(certificatePEM []byte) (*x509.Certificate, error) {
	certificateBlock, _ := pem.Decode(certificatePEM)
	if certificateBlock == nil {
		return nil, errors.New("monitoring CA certificate PEM block not found")
	}
	certificate, err := x509.ParseCertificate(certificateBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse monitoring CA certificate: %w", err)
	}
	return certificate, nil
}

func validateMonitoringCACertificate(certificate *x509.Certificate) error {
	if !certificate.IsCA || !certificate.BasicConstraintsValid {
		return errors.New("monitoring CA certificate lacks valid CA constraints")
	}
	if certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		return errors.New("monitoring CA certificate cannot sign certificates")
	}
	if certificate.NotAfter.Before(certificate.NotBefore.Add(monitoringCAMinLifetime)) {
		return errors.New("monitoring CA certificate validity is shorter than one year")
	}
	if time.Now().Add(monitoringCAMinRemaining).After(certificate.NotAfter) {
		return errors.New("monitoring CA certificate is expired or too close to expiry")
	}
	if err := certificate.CheckSignatureFrom(certificate); err != nil {
		return fmt.Errorf("monitoring CA certificate is not self-signed: %w", err)
	}
	return nil
}

func parseMonitoringCAKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("monitoring CA key PEM block not found")
	}
	privateKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse monitoring CA key: %w", err)
	}
	return privateKey, nil
}

func validateMonitoringCAKeyPair(certificate *x509.Certificate, privateKey *ecdsa.PrivateKey) error {
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.X == nil || publicKey.Y == nil {
		return errors.New("monitoring CA certificate does not contain an ECDSA public key")
	}
	if publicKey.X.Cmp(privateKey.X) != 0 || publicKey.Y.Cmp(privateKey.Y) != 0 {
		return errors.New("monitoring CA certificate does not match its key")
	}
	return nil
}

func removeStaleDemoMonitoringTLSSecret(ctx context.Context, kubeconfigPath, monitoringNamespace string, caCert *x509.Certificate, writer io.Writer) error {
	secretJSON, err := kubectlJSON(ctx, kubeconfigPath, "-n", monitoringNamespace, "get", "secret", monitoringTLSSecretName, "--ignore-not-found", "-o", "json")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(secretJSON)) == 0 {
		return nil
	}
	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(secretJSON, &secret); err != nil {
		return fmt.Errorf("decode monitoring TLS Secret: %w", err)
	}
	encodedCertificate := strings.TrimSpace(secret.Data["tls.crt"])
	if encodedCertificate == "" {
		return nil
	}
	certificatePEM, err := base64.StdEncoding.DecodeString(encodedCertificate)
	if err != nil {
		return fmt.Errorf("decode monitoring TLS certificate: %w", err)
	}
	matches, verifyErr := certificateSignedByCA(certificatePEM, caCert)
	if verifyErr == nil && matches && monitoringCertificateValid(encodedCertificate) {
		return nil
	}
	if err := runKubectl(ctx, kubeconfigPath, writer, "delete", "secret", monitoringTLSSecretName, "-n", monitoringNamespace, "--ignore-not-found"); err != nil {
		return fmt.Errorf("delete stale monitoring TLS Secret: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ♻️  removed stale monitoring TLS Secret; cert-manager will issue it from the demo CA")
	return nil
}

// EnsureMonitoringTLS creates an administrator-owned monitoring CA bundle and
// signs the isolated monitoring endpoint certificate with it. The operator's
// monitoring clients consume the bundle through monitoring.tls.caCertSecretRef;
// the ConfigMap copy remains for monitoring-server fixtures and host tooling.
func EnsureMonitoringTLS(ctx context.Context, kubeconfigPath, applicationNamespace, monitoringNamespace string, writer io.Writer) error {
	if monitoringTLSReady(ctx, kubeconfigPath, applicationNamespace, monitoringNamespace, writer) {
		return nil
	}
	// Issue #564: monitoring trust is a separate administrator-owned CA. Do
	// not reuse the operator inter-service CA even when it is already present.
	ca, err := generateMonitoringCA()
	if err != nil {
		return fmt.Errorf("prepare monitoring CA: %w", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate monitoring TLS key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate monitoring TLS serial: %w", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "monitoring-services"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames: []string{
			"localhost",
			"prometheus-svc",
			"prometheus-svc." + monitoringNamespace,
			"prometheus-svc." + monitoringNamespace + ".svc",
			"prometheus-svc." + monitoringNamespace + ".svc.cluster.local",
			"alertmanager-svc",
			"alertmanager-svc." + monitoringNamespace,
			"alertmanager-svc." + monitoringNamespace + ".svc",
			"alertmanager-svc." + monitoringNamespace + ".svc.cluster.local",
			"thanos-querier-svc",
			"thanos-querier-svc." + monitoringNamespace,
			"thanos-querier-svc." + monitoringNamespace + ".svc",
			"thanos-querier-svc." + monitoringNamespace + ".svc.cluster.local",
		},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca.certificate, &leafKey.PublicKey, ca.key)
	if err != nil {
		return fmt.Errorf("create monitoring TLS certificate: %w", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return fmt.Errorf("marshal monitoring TLS key: %w", err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})
	webConfig := `tls_server_config:
  cert_file: /etc/monitoring-tls/tls.crt
  key_file: /etc/monitoring-tls/tls.key
`
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/tls
stringData:
  tls.crt: |
%s
  tls.key: |
%s
  ca.crt: |
%s
  web-config.yml: |
%s
`, monitoringTLSSecretName, monitoringNamespace, indentPEM(string(leafCertPEM)), indentPEM(string(leafKeyPEM)), indentPEM(string(ca.certificatePEM)), indentYAMLBlock(webConfig, 4))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("create monitoring TLS Secret: %w", err)
	}
	caManifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  ca.crt: |
%s
`, monitoringCAConfigMapName, applicationNamespace, indentPEM(string(ca.certificatePEM)))
	if monitoringNamespace != applicationNamespace {
		caManifest += fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  ca.crt: |
%s
`, monitoringCAConfigMapName, monitoringNamespace, indentPEM(string(ca.certificatePEM)))
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, caManifest); err != nil {
		return fmt.Errorf("create monitoring CA ConfigMap: %w", err)
	}
	clientCASecret := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
  ca.crt: %s
`, monitoringClientCASecretName, applicationNamespace, base64.StdEncoding.EncodeToString(ca.certificatePEM))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, clientCASecret); err != nil {
		return fmt.Errorf("create monitoring client CA Secret: %w", err)
	}
	if err := os.WriteFile(MonitoringCAPath(kubeconfigPath), ca.certificatePEM, 0o600); err != nil {
		return fmt.Errorf("write monitoring CA: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ monitoring TLS Secret ready in %s\n", monitoringNamespace)
	return nil
}

func monitoringTLSReady(ctx context.Context, kubeconfigPath, applicationNamespace, monitoringNamespace string, writer io.Writer) bool {
	checkCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "secret/"+monitoringTLSSecretName, "-n", monitoringNamespace, "--ignore-not-found", "-o", "name")
	clientCheckCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "secret/"+monitoringClientCASecretName, "-n", applicationNamespace, "--ignore-not-found", "-o", "name")
	monitoringReady := commandHasOutput(checkCmd)
	clientCAReady := commandHasOutput(clientCheckCmd)
	if !monitoringReady || !clientCAReady {
		return false
	}
	cacheMatches, cacheErr := monitoringTLSCacheMatches(ctx, kubeconfigPath, applicationNamespace, monitoringNamespace)
	switch {
	case cacheErr != nil:
		_, _ = fmt.Fprintf(writer, "  ⚠️  monitoring TLS cache validation failed; regenerating trust material: %v\n", cacheErr)
	case cacheMatches:
		_, _ = fmt.Fprintf(writer, "  ✅ monitoring TLS already exists in %s\n", monitoringNamespace)
		return true
	default:
		_, _ = fmt.Fprintln(writer, "  ⚠️  monitoring TLS cache is stale; regenerating trust material")
	}
	return false
}

func commandHasOutput(cmd *exec.Cmd) bool {
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

// ReplicateMonitoringTLS copies the demo monitoring serving Secret and CA
// ConfigMap between the hub and spoke kubeconfigs. Fleet Prometheus instances
// must trust the hub AlertManager certificate; generating an independent spoke
// CA would make the HTTPS alerting hop fail closed.
func ReplicateMonitoringTLS(ctx context.Context, sourceKubeconfig, targetKubeconfig, monitoringNamespace string, writer io.Writer) error {
	secretJSON, err := kubectlJSON(ctx, sourceKubeconfig, "-n", monitoringNamespace, "get", "secret", monitoringTLSSecretName, "-o", "json")
	if err != nil {
		return fmt.Errorf("read source monitoring TLS Secret: %w", err)
	}
	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(secretJSON, &secret); err != nil {
		return fmt.Errorf("decode source monitoring TLS Secret: %w", err)
	}
	ca := secret.Data["ca.crt"]
	if ca == "" {
		return errors.New("source monitoring TLS Secret has no ca.crt")
	}
	secretManifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
  ca.crt: %s
  web-config.yml: %s
`, monitoringTLSSecretName, monitoringNamespace, secret.Data["tls.crt"], secret.Data["tls.key"], ca, secret.Data["web-config.yml"])
	if err := kubectlApplyManifest(ctx, targetKubeconfig, writer, secretManifest); err != nil {
		return fmt.Errorf("apply target monitoring TLS Secret: %w", err)
	}
	caPEM := decodeMonitoringCA(ca)
	configMapManifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  ca.crt: |
%s
`, monitoringCAConfigMapName, monitoringNamespace, indentPEM(string(caPEM)))
	if err := kubectlApplyManifest(ctx, targetKubeconfig, writer, configMapManifest); err != nil {
		return fmt.Errorf("apply target monitoring CA ConfigMap: %w", err)
	}
	if err := os.WriteFile(MonitoringCAPath(targetKubeconfig), caPEM, 0o600); err != nil {
		return fmt.Errorf("write target monitoring CA: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ monitoring TLS trust replicated to %s\n", monitoringNamespace)
	return nil
}

func kubectlJSON(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--kubeconfig", kubeconfigPath}, args...)
	return exec.CommandContext(ctx, "kubectl", commandArgs...).Output()
}

func decodeMonitoringCA(value string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return []byte(value)
	}
	return decoded
}

type monitoringCA struct {
	certificate    *x509.Certificate
	certificatePEM []byte
	key            *ecdsa.PrivateKey
}

func generateMonitoringCA() (monitoringCA, error) {
	return generateMonitoringCAWithValidity(24 * time.Hour)
}

func generateMonitoringCAWithValidity(validity time.Duration) (monitoringCA, error) {
	if validity <= 0 {
		return monitoringCA{}, errors.New("monitoring CA validity must be positive")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return monitoringCA{}, fmt.Errorf("generate monitoring CA key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return monitoringCA{}, fmt.Errorf("generate monitoring CA serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kubernaut-monitoring-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(validity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return monitoringCA{}, fmt.Errorf("create monitoring CA certificate: %w", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return monitoringCA{}, fmt.Errorf("parse monitoring CA certificate: %w", err)
	}
	return monitoringCA{
		certificate:    certificate,
		certificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		key:            key,
	}, nil
}

func indentYAMLBlock(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimSuffix(value, "\n"), "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
