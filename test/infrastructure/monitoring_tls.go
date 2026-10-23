package infrastructure

import (
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
	monitoringTLSSecretName   = "monitoring-tls"
	monitoringCAConfigMapName = "monitoring-ca"
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

// EnsureMonitoringTLS creates an administrator-owned monitoring CA bundle and
// signs the isolated monitoring endpoint certificate with it. The operator's
// monitoring clients consume the bundle through monitoring.tls.source=configMap.
func EnsureMonitoringTLS(ctx context.Context, kubeconfigPath, applicationNamespace, monitoringNamespace string, writer io.Writer) error {
	_, _, operatorCAErr := readOperatorCASecret(ctx, kubeconfigPath, applicationNamespace)
	checkCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "secret/"+monitoringTLSSecretName, "-n", monitoringNamespace, "--ignore-not-found", "-o", "name")
	if operatorCAErr != nil && !errors.Is(operatorCAErr, errOperatorCASecretMissing) {
		return fmt.Errorf("inspect operator monitoring CA: %w", operatorCAErr)
	}
	if errors.Is(operatorCAErr, errOperatorCASecretMissing) {
		if output, err := checkCmd.Output(); err == nil && strings.TrimSpace(string(output)) != "" {
			if _, statErr := os.Stat(MonitoringCAPath(kubeconfigPath)); statErr == nil {
				_, _ = fmt.Fprintf(writer, "  ✅ monitoring TLS already exists in %s\n", monitoringNamespace)
				return nil
			}
		}
	}
	ca, err := monitoringCAFromOperator(ctx, kubeconfigPath, applicationNamespace)
	if errors.Is(err, errOperatorCASecretMissing) {
		ca, err = generateMonitoringCA()
	}
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
	if err := os.WriteFile(MonitoringCAPath(kubeconfigPath), ca.certificatePEM, 0o600); err != nil {
		return fmt.Errorf("write monitoring CA: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ monitoring TLS Secret ready in %s\n", monitoringNamespace)
	return nil
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

// monitoringCAFromOperator reuses the operator inter-service trust root when
// the operator-managed application path is active. The monitoring endpoints
// are then valid for spec.monitoring.tls.source=interService, while the host
// probe and the in-cluster clients trust the same CA.
func monitoringCAFromOperator(ctx context.Context, kubeconfigPath, namespace string) (monitoringCA, error) {
	caPEM, keyPEM, err := readOperatorCASecret(ctx, kubeconfigPath, namespace)
	if err != nil {
		return monitoringCA{}, err
	}
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil {
		return monitoringCA{}, fmt.Errorf("decode operator monitoring CA certificate")
	}
	certificate, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return monitoringCA{}, fmt.Errorf("parse operator monitoring CA certificate: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return monitoringCA{}, fmt.Errorf("decode operator monitoring CA key")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return monitoringCA{}, fmt.Errorf("parse operator monitoring CA key: %w", err)
	}
	return monitoringCA{certificate: certificate, certificatePEM: caPEM, key: key}, nil
}

type monitoringCA struct {
	certificate    *x509.Certificate
	certificatePEM []byte
	key            *ecdsa.PrivateKey
}

func generateMonitoringCA() (monitoringCA, error) {
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
		NotAfter:              time.Now().Add(24 * time.Hour),
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
