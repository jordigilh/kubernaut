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
	operatorInternalCASecretName     = "kubernaut-internal-ca"
	operatorDataStorageSigningSecret = "datastorage-signing-cert"
	mcpGatewayTLSSecretName          = "mcp-gateway-tls"
	operatorTrustBundleConfigMapName = "inter-service-trust-bundle"
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
	"kube-mcp-server.127-0-0-1.sslip.io",
	"kube-mcp-server-remote.127-0-0-1.sslip.io",
}

// EnsureOperatorFleetTLS creates the qualification CA before the Kubernaut CR
// is applied and signs the MCP Gateway listener certificate with that CA. The
// latest v1alpha2 manual TLS contract keeps the CA and serving material
// administrator-owned while sharing one explicit trust root.
func EnsureOperatorFleetTLS(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	caPEM, caKeyPEM, err := ensureOperatorInternalCA(ctx, kubeconfigPath, namespace, writer)
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
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
`, mcpGatewayTLSSecretName, namespace,
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

// EnsureOperatorApplicationTLS prepares the read-only manual TLS material used
// by the current v1alpha2 CR contract. The operator validates these Secrets but
// does not own or rotate them; the demo owns this disposable material.
func EnsureOperatorApplicationTLS(ctx context.Context, kubeconfigPath, namespace string, includeFleet bool, writer io.Writer) error {
	caPEM, caKeyPEM, err := ensureOperatorInternalCA(ctx, kubeconfigPath, namespace, writer)
	if err != nil {
		return fmt.Errorf("prepare operator CA for manual service TLS: %w", err)
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildOperatorTLSTrustBundleManifest(namespace, caPEM)); err != nil {
		return fmt.Errorf("apply manual operator TLS trust bundle: %w", err)
	}
	// Keep the legacy chart-owned ConfigMap name as a test-owned compatibility
	// alias. Existing DEX/host-side E2E helpers consume inter-service-ca while
	// operator-managed operands consume inter-service-trust-bundle; both must
	// contain the same administrator-owned CA in this qualification profile.
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildOperatorInterServiceCAManifest(namespace, caPEM)); err != nil {
		return fmt.Errorf("apply compatibility inter-service CA ConfigMap: %w", err)
	}
	services := operatorApplicationTLSServiceDefinitions(includeFleet)
	var manifest strings.Builder
	for _, service := range services {
		dnsNames := qualificationServiceDNSNames(service.serviceName, namespace)
		leafCert, leafKey, signErr := signQualificationLeafForDNS(caPEM, caKeyPEM, time.Now(), dnsNames, service.serviceName)
		if signErr != nil {
			return fmt.Errorf("sign %s: %w", service.secretName, signErr)
		}
		if manifest.Len() > 0 {
			manifest.WriteString("---\n")
		}
		caData := ""
		if service.secretName == "authwebhook-tls" {
			caData = fmt.Sprintf("  ca.crt: %s\n  ca.key: %s\n",
				base64.StdEncoding.EncodeToString(caPEM),
				base64.StdEncoding.EncodeToString(caKeyPEM))
		}
		fmt.Fprintf(&manifest, `apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
%s`, service.secretName, namespace, base64.StdEncoding.EncodeToString(leafCert), base64.StdEncoding.EncodeToString(leafKey), caData)
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest.String()); err != nil {
		return fmt.Errorf("apply manual operator service TLS: %w", err)
	}
	if err := generateSigningCertSecret(ctx, kubeconfigPath, namespace, operatorDataStorageSigningSecret, writer); err != nil {
		return fmt.Errorf("apply operator DataStorage signing Secret: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ manual operator service TLS Secrets ready")
	return nil
}

// BuildOperatorTLSTrustBundleManifest publishes the administrator-prepared
// manual-TLS CA under both keys consumed by the generic operator workloads and
// the qualification evidence reader. The pinned operator's API Frontend
// volume expects service-ca.crt even in manual mode.
func BuildOperatorTLSTrustBundleManifest(namespace string, caPEM []byte) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
data:
  ca.crt: |
%s
  service-ca.crt: |
%s
`, operatorTrustBundleConfigMapName, namespace, indentPEM(string(caPEM)), indentPEM(string(caPEM)))
}

// BuildOperatorInterServiceCAManifest preserves the chart-era ConfigMap
// contract consumed by existing qualification-only DEX and host probe helpers.
func BuildOperatorInterServiceCAManifest(namespace string, caPEM []byte) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: inter-service-ca
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
data:
  ca.crt: |
%s
`, namespace, indentPEM(string(caPEM)))
}

type operatorApplicationTLSService struct {
	secretName  string
	serviceName string
}

func operatorApplicationTLSServiceDefinitions(includeFleet bool) []operatorApplicationTLSService {
	services := []operatorApplicationTLSService{
		{secretName: "gateway-tls", serviceName: "gateway-service"},
		{secretName: "datastorage-tls", serviceName: "data-storage-service"},
		{secretName: "kubernautagent-tls", serviceName: "kubernaut-agent"},
		{secretName: "apifrontend-tls", serviceName: "apifrontend"},
		{secretName: "authwebhook-tls", serviceName: "authwebhook-service"},
	}
	if includeFleet {
		services = append(services, operatorApplicationTLSService{secretName: "fleetmetadatacache-tls", serviceName: "fleetmetadatacache-service"})
	}
	return services
}

func qualificationServiceDNSNames(serviceName, namespace string) []string {
	names := []string{
		"localhost",
		serviceName,
		serviceName + "." + namespace,
		serviceName + "." + namespace + ".svc",
		serviceName + "." + namespace + ".svc.cluster.local",
	}
	if serviceName == "authwebhook-service" {
		names = append(names,
			"authwebhook",
			"authwebhook."+namespace,
			"authwebhook."+namespace+".svc",
			"authwebhook."+namespace+".svc.cluster.local",
		)
	}
	return names
}

func ensureOperatorInternalCA(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) ([]byte, []byte, error) {
	caPEM, caKeyPEM, err := readOperatorCASecret(ctx, kubeconfigPath, namespace)
	if err == nil {
		return caPEM, caKeyPEM, nil
	}
	if !errors.Is(err, errOperatorCASecretMissing) {
		return nil, nil, err
	}
	caPEM, caKeyPEM, err = generateQualificationCA(time.Now())
	if err != nil {
		return nil, nil, err
	}
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
type: Opaque
data:
  ca.crt: %s
  ca.key: %s
`, operatorInternalCASecretName, namespace,
		base64.StdEncoding.EncodeToString(caPEM),
		base64.StdEncoding.EncodeToString(caKeyPEM))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return nil, nil, fmt.Errorf("apply operator internal CA Secret: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ operator internal CA Secret ready")
	return caPEM, caKeyPEM, nil
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
			"get", "secret", operatorInternalCASecretName, "-n", namespace, "--ignore-not-found", "-o", "jsonpath="+jsonPath)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("read %s from operator ca secret: %w", key, err)
		}
		if strings.TrimSpace(string(output)) == "" {
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
	return signQualificationLeafForDNS(caPEM, caKeyPEM, now, mcpGatewayTLSDNSNames, mcpGatewayTLSSecretName)
}

func signQualificationLeafForDNS(caPEM, caKeyPEM []byte, now time.Time, dnsNames []string, commonName string) ([]byte, []byte, error) {
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
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		DNSNames:     append([]string(nil), dnsNames...),
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
