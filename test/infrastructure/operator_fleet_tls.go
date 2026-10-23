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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	operatorInternalCASecretName = "kubernaut-internal-ca"
	mcpGatewayTLSSecretName      = "mcp-gateway-tls"
)

// mcpGatewayTLSDNSNames covers both Gateway API implementations used by the
// Fleet qualification harness. Envoy Gateway generates a hashed Service name
// in envoy-gateway-system, so the namespace wildcard is intentional. The
// localhost SAN is required by the host-side NodePort probes.
var mcpGatewayTLSDNSNames = []string{
	"localhost",
	"mcp-gateway",
	"mcp-gateway.gateway-system.svc",
	"mcp-gateway.gateway-system.svc.cluster.local",
	"mcp-gateway-istio.gateway-system.svc",
	"mcp-gateway-istio.gateway-system.svc.cluster.local",
	"*.gateway-system.svc",
	"*.gateway-system.svc.cluster.local",
	"*.envoy-gateway-system.svc",
	"*.envoy-gateway-system.svc.cluster.local",
}

// EnsureOperatorFleetTLS creates the qualification CA before the Kubernaut CR
// is applied and signs the MCP Gateway listener certificate with that CA. The
// operator's DevelopmentSelfSigned mode reuses a valid pre-existing
// kubernaut-internal-ca Secret, which makes the gateway and every
// operator-managed workload share one explicit trust root without disabling
// certificate verification.
func EnsureOperatorFleetTLS(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	caPEM, caKeyPEM, err := readOperatorCASecret(ctx, kubeconfigPath, namespace)
	if errors.Is(err, errOperatorCASecretMissing) {
		caPEM, caKeyPEM, err = generateQualificationCA(time.Now())
	}
	if err != nil {
		return fmt.Errorf("prepare operator Fleet CA: %w", err)
	}

	leafCertPEM, leafKeyPEM, err := signQualificationLeaf(caPEM, caKeyPEM, time.Now())
	if err != nil {
		return fmt.Errorf("sign MCP Gateway TLS certificate: %w", err)
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: kubernaut-operator
    app.kubernetes.io/part-of: kubernaut
    app.kubernetes.io/instance: kubernaut
type: Opaque
data:
  ca.crt: %s
  ca.key: %s
---
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
`, operatorInternalCASecretName, namespace,
		base64.StdEncoding.EncodeToString(caPEM),
		base64.StdEncoding.EncodeToString(caKeyPEM),
		mcpGatewayTLSSecretName, namespace,
		base64.StdEncoding.EncodeToString(leafCertPEM),
		base64.StdEncoding.EncodeToString(leafKeyPEM))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("apply operator Fleet TLS Secrets: %w", err)
	}
	if err := writeCombinedInterServiceCAPEM(InterServiceCAPath(kubeconfigPath), caPEM); err != nil {
		return fmt.Errorf("write operator Fleet CA for host probes: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ operator Fleet CA and %s TLS Secret ready\n", mcpGatewayTLSSecretName)
	return nil
}

// writeOperatorFleetCAPEMIfPresent restores the operator CA after the legacy
// Fleet infrastructure helper provisions its separate kube-mcp trust bundle.
// The operator qualification path needs the MCP Gateway probe to trust the
// pre-created operator CA; legacy non-operator callers simply skip this.
func writeOperatorFleetCAPEMIfPresent(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	caPEM, _, err := readOperatorCASecret(ctx, kubeconfigPath, applicationNamespace)
	if errors.Is(err, errOperatorCASecretMissing) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read operator Fleet CA for host probes: %w", err)
	}
	if err := writeCombinedInterServiceCAPEM(InterServiceCAPath(kubeconfigPath), caPEM); err != nil {
		return fmt.Errorf("write operator Fleet CA for host probes: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ restored operator Fleet CA for host probes: %s\n", InterServiceCAPath(kubeconfigPath))
	return nil
}

// writeCombinedInterServiceCAPEM keeps host-side probes connected to both
// trust domains used by the operator Fleet harness: the legacy inter-service
// CA signs Keycloak/kube-mcp-server, while the pre-created operator CA signs
// the MCP Gateway and operator-managed application Services.
func writeCombinedInterServiceCAPEM(path string, caPEM []byte) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing host CA bundle: %w", err)
	}
	combined := existing
	if len(combined) == 0 {
		combined = append([]byte(nil), caPEM...)
	} else if !strings.Contains(string(combined), string(caPEM)) {
		combined = append(combined, '\n')
		combined = append(combined, caPEM...)
	}
	if err := os.WriteFile(path, combined, 0o600); err != nil {
		return fmt.Errorf("write host CA bundle: %w", err)
	}
	return nil
}

var errOperatorCASecretMissing = errors.New("operator CA Secret is missing")

func readOperatorCASecret(ctx context.Context, kubeconfigPath, namespace string) ([]byte, []byte, error) {
	readKey := func(key string) ([]byte, error) {
		jsonPath := "{.data." + strings.ReplaceAll(key, ".", `\.`) + "}"
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "secret", operatorInternalCASecretName, "-n", namespace, "-o", "jsonpath="+jsonPath)
		output, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(output)) == "" {
			return nil, errOperatorCASecretMissing
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
		if err != nil {
			return nil, fmt.Errorf("decode %s from operator CA Secret: %w", key, err)
		}
		return decoded, nil
	}
	caPEM, err := readKey("ca.crt")
	if err != nil {
		return nil, nil, err
	}
	caKeyPEM, err := readKey("ca.key")
	if err != nil {
		return nil, nil, err
	}
	return caPEM, caKeyPEM, nil
}

func generateQualificationCA(now time.Time) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kubernaut qualification CA"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal CA key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func signQualificationLeaf(caPEM, caKeyPEM []byte, now time.Time) ([]byte, []byte, error) {
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil {
		return nil, nil, errors.New("qualification CA certificate is not PEM encoded")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse qualification CA certificate: %w", err)
	}
	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("qualification CA key is not PEM encoded")
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse qualification CA key: %w", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate MCP Gateway leaf key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate MCP Gateway leaf serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: mcpGatewayTLSSecretName},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		DNSNames:     append([]string(nil), mcpGatewayTLSDNSNames...),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create MCP Gateway leaf certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal MCP Gateway leaf key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}
