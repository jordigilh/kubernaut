/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package infrastructure

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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

// InterServiceCAPath returns the deterministic file path where the inter-service
// CA PEM is written. Derived from kubeconfigPath so all E2E suites can find it
// without serialization changes in SynchronizedBeforeSuite.
//
// Issue #753: Eliminates E2E serialization risk (C-2 mitigation).
func InterServiceCAPath(kubeconfigPath string) string {
	return filepath.Join(filepath.Dir(kubeconfigPath), "inter-service-ca.pem")
}

// GenerateInterServiceTLS generates a self-signed CA and per-service ECDSA P-256
// leaf certificates, creates the corresponding Kubernetes Secrets and ConfigMap,
// and writes the CA PEM to the deterministic path from InterServiceCAPath.
//
// Services: data-storage-service, gateway-service, kubernaut-agent
//
// Issue #753 (S-4): Uses ECDSA P-256 instead of RSA 2048.
// Issue #753 (C-2): Returns caPEMPath for host-side TLS-aware test clients.
func GenerateInterServiceTLS(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) (string, error) {
	_, _ = fmt.Fprintln(writer, "🔐 Issue #753: Generating inter-service TLS certificates (ECDSA P-256)...")

	// Idempotency guard: if the CA ConfigMap already exists in this namespace and the
	// host-side CA PEM file is present, skip regeneration. This prevents a race condition
	// in fullpipeline E2E where multiple component deployers call this function in parallel
	// goroutines, each generating a different CA and overwriting the Secrets/ConfigMap.
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	checkCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "configmap", "inter-service-ca", "-n", namespace, "--ignore-not-found", "-o", "name")
	checkOut, checkErr := checkCmd.Output()
	if checkErr == nil && strings.TrimSpace(string(checkOut)) != "" {
		if _, statErr := os.Stat(caPEMPath); statErr == nil {
			_, _ = fmt.Fprintf(writer, "  ✅ inter-service TLS already exists (ConfigMap + %s) — skipping\n", caPEMPath)
			return caPEMPath, nil
		}
	}

	// Generate CA key pair
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kubernaut-inter-service-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return "", fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return "", fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Write CA PEM to deterministic path (caPEMPath declared in idempotency guard above)
	if err := os.WriteFile(caPEMPath, caCertPEM, 0600); err != nil {
		return "", fmt.Errorf("failed to write CA PEM to %s: %w", caPEMPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ CA PEM written to %s\n", caPEMPath)

	// Create ConfigMap with CA bundle
	caConfigMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: inter-service-ca
data:
  ca.crt: |
%s`, indentPEM(string(caCertPEM), 4))

	if err := kubectlApply(ctx, kubeconfigPath, namespace, caConfigMap, writer); err != nil {
		return "", fmt.Errorf("failed to create CA ConfigMap: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ inter-service-ca ConfigMap created")

	// Generate leaf certs for each service.
	// Include "localhost" + 127.0.0.1 so host-side E2E clients connecting
	// through NodePort can verify the certificate.
	services := []struct {
		name       string
		secretName string
		dnsNames   []string
		ipAddrs    []net.IP
	}{
		{
			name:       "data-storage-service",
			secretName: "datastorage-tls",
			dnsNames: []string{
				"localhost",
				"data-storage-service",
				fmt.Sprintf("data-storage-service.%s", namespace),
				fmt.Sprintf("data-storage-service.%s.svc", namespace),
				fmt.Sprintf("data-storage-service.%s.svc.cluster.local", namespace),
			},
			ipAddrs: []net.IP{net.IPv4(127, 0, 0, 1)},
		},
		{
			name:       "gateway-service",
			secretName: "gateway-tls",
			dnsNames: []string{
				"localhost",
				"gateway-service",
				fmt.Sprintf("gateway-service.%s", namespace),
				fmt.Sprintf("gateway-service.%s.svc", namespace),
				fmt.Sprintf("gateway-service.%s.svc.cluster.local", namespace),
			},
			ipAddrs: []net.IP{net.IPv4(127, 0, 0, 1)},
		},
		{
			name:       "kubernaut-agent",
			secretName: "kubernautagent-tls",
			dnsNames: []string{
				"localhost",
				"kubernaut-agent",
				fmt.Sprintf("kubernaut-agent.%s", namespace),
				fmt.Sprintf("kubernaut-agent.%s.svc", namespace),
				fmt.Sprintf("kubernaut-agent.%s.svc.cluster.local", namespace),
			},
			ipAddrs: []net.IP{net.IPv4(127, 0, 0, 1)},
		},
	}

	for _, svc := range services {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return "", fmt.Errorf("failed to generate key for %s: %w", svc.name, err)
		}

		leafTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: svc.name},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			DNSNames:     svc.dnsNames,
			IPAddresses:  svc.ipAddrs,
		}

		leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
		if err != nil {
			return "", fmt.Errorf("failed to create certificate for %s: %w", svc.name, err)
		}

		leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

		leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
		if err != nil {
			return "", fmt.Errorf("failed to marshal key for %s: %w", svc.name, err)
		}
		leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})

		secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
type: kubernetes.io/tls
stringData:
  tls.crt: |
%s
  tls.key: |
%s`, svc.secretName, indentPEM(string(leafCertPEM), 4), indentPEM(string(leafKeyPEM), 4))

		if err := kubectlApply(ctx, kubeconfigPath, namespace, secret, writer); err != nil {
			return "", fmt.Errorf("failed to create TLS Secret for %s: %w", svc.name, err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s Secret created (ECDSA P-256, SANs: %v)\n", svc.secretName, svc.dnsNames)
	}

	_, _ = fmt.Fprintln(writer, "🔐 Inter-service TLS generation complete")
	return caPEMPath, nil
}

// NewTLSAwareClient creates an HTTP client that trusts the inter-service CA.
// Used by host-side E2E test code to call HTTPS NodePort endpoints.
//
// Issue #753 (C-2): Host-side test clients must verify service certs signed
// by the inter-service CA.
func NewTLSAwareClient(kubeconfigPath string, timeout time.Duration) (*http.Client, error) {
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	caPEM, err := os.ReadFile(caPEMPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA PEM from %s: %w", caPEMPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid PEM certificates found in %s", caPEMPath)
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

// NewTLSAwareTransport creates an http.Transport that trusts the inter-service CA.
// Useful for wrapping with additional transports (e.g., ServiceAccountTransport).
func NewTLSAwareTransport(kubeconfigPath string) (*http.Transport, error) {
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	caPEM, err := os.ReadFile(caPEMPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA PEM from %s: %w", caPEMPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid PEM certificates found in %s", caPEMPath)
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
	}, nil
}

// TLSCertVolumeYAML returns the YAML snippet for mounting a TLS cert Secret as a volume.
// Indent is the number of spaces to prepend to each line.
func TLSCertVolumeYAML(secretName string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	return fmt.Sprintf(`
%s- name: tls-certs
%s  secret:
%s    secretName: %s`, prefix, prefix, prefix, secretName)
}

// TLSCertVolumeMountYAML returns the YAML snippet for a TLS cert volume mount.
func TLSCertVolumeMountYAML(indent int) string {
	prefix := strings.Repeat(" ", indent)
	return fmt.Sprintf(`
%s- name: tls-certs
%s  mountPath: /etc/tls
%s  readOnly: true`, prefix, prefix, prefix)
}

// TLSCAVolumeYAML returns the YAML snippet for mounting the CA ConfigMap as a volume.
func TLSCAVolumeYAML(indent int) string {
	prefix := strings.Repeat(" ", indent)
	return fmt.Sprintf(`
%s- name: tls-ca
%s  configMap:
%s    name: inter-service-ca`, prefix, prefix, prefix)
}

// TLSCAVolumeMountYAML returns the YAML snippet for a CA volume mount.
func TLSCAVolumeMountYAML(indent int) string {
	prefix := strings.Repeat(" ", indent)
	return fmt.Sprintf(`
%s- name: tls-ca
%s  mountPath: /etc/tls-ca
%s  readOnly: true`, prefix, prefix, prefix)
}

// TLSCAEnvYAML returns the YAML snippet for the TLS_CA_FILE environment variable.
func TLSCAEnvYAML(indent int) string {
	prefix := strings.Repeat(" ", indent)
	return fmt.Sprintf(`
%s- name: TLS_CA_FILE
%s  value: /etc/tls-ca/ca.crt`, prefix, prefix)
}

// kubectlApply applies a YAML manifest via kubectl.
func kubectlApply(ctx context.Context, kubeconfigPath, namespace, manifest string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// indentPEM indents each line of a PEM string by n spaces.
func indentPEM(pemStr string, n int) string {
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(pemStr, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
