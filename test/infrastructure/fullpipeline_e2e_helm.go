/*
Copyright 2025 Jordi Gil.

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
	"path/filepath"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Operator-backed FullPipeline support (Issue #2492).
//
// This file contains only shared test infrastructure that is still needed
// around the operator-managed application: dependency/Fleet fixtures, TLS
// material for external test IdPs, API Frontend RBAC, and the test-owned
// migration image. The application is deployed exclusively by the pinned
// operator bootstrap chart plus the user-owned Kubernaut CR.

// ensureDBMigrateImage builds (or reuses a CI-prebuilt artifact for) the
// db-migrate image used by the operator's migration workload.
//
// db-migrate is NOT one of the fullPipelineImageConfigs/buildFullPipelineImages
// app images -- it's a standalone infra image built from its own Dockerfile
// (mirrors CI's build-infra-images matrix job, alongside mock-llm), never
// built by the Go-native harness before Issue #1737 because migrations used
// to run via direct SQL exec (test/infrastructure/migrations.go), not this
// containerized goose job. Missing this image left the migration Job stuck
// in Init:ImagePullBackOff (confirmed during Issue #1737 validation), which
// in turn left DataStorage permanently crash-looping (no tables ever created).
func ensureDBMigrateImage(ctx context.Context, sharedTag string, writer io.Writer) (string, error) {
	dst := fmt.Sprintf("localhost/db-migrate:%s", sharedTag)

	if src, ok := resolvePrebuiltCIArtifact(ctx, "db-migrate", writer); ok {
		cmd := exec.CommandContext(ctx, "podman", "tag", src, dst)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to retag db-migrate CI artifact (%s -> %s): %w", src, dst, err)
		}
		return dst, nil
	}
	if registry := strings.TrimSpace(os.Getenv("IMAGE_REGISTRY")); registry != "" {
		if tag := strings.TrimSpace(os.Getenv("IMAGE_TAG")); tag != "" {
			image := fmt.Sprintf("%s/db-migrate:%s", strings.TrimRight(registry, "/"), tag)
			_, _ = fmt.Fprintf(writer, "  📦 Using published db-migrate operand %s (no local build)\n", image)
			return image, nil
		}
	}

	_, _ = fmt.Fprintln(writer, "  🔨 Building db-migrate image (docker/db-migrate.Dockerfile, infra image not covered by buildFullPipelineImages)...")
	projectRoot := getProjectRoot()
	cmd := exec.CommandContext(ctx, "podman", "build",
		"-t", dst,
		"-f", filepath.Join(projectRoot, "docker", "db-migrate.Dockerfile"),
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build db-migrate image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ db-migrate → %s\n", dst)
	return dst, nil
}

// loadOperatorCAFromAuthwebhookTLS extracts the CA certificate + private key that
// the operator's tls-cert-gen pre-install hook embeds in authwebhook-tls
// (ca.crt/ca.key, base64) so downstream consumers can mint additional leaf
// certs signed by the SAME CA without regenerating it.
func loadOperatorCAFromAuthwebhookTLS(ctx context.Context, kubeconfigPath, namespace string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	getField := func(field string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
			"get", "secret", "authwebhook-tls", "-o", fmt.Sprintf("jsonpath={.data.%s}", field))
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to read authwebhook-tls %s: %w", field, err)
		}
		decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
		if decErr != nil {
			return nil, fmt.Errorf("failed to base64-decode authwebhook-tls %s: %w", field, decErr)
		}
		return decoded, nil
	}

	caCertPEM, err := getField(`ca\.crt`)
	if err != nil {
		return nil, nil, err
	}
	caKeyPEM, err := getField(`ca\.key`)
	if err != nil {
		return nil, nil, err
	}

	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM from authwebhook-tls")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA private key PEM from authwebhook-tls")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}
	return caCert, caKey, nil
}

// generateSignedLeafCert generates an ECDSA P-256 leaf certificate for
// commonName/dnsNames/ipAddrs, valid from 1 hour ago for validity, signed by
// caCert/caKey, and returns its PEM-encoded certificate and private key.
// Shared by signAndApplyLeafTLSSecret (dex-tls/keycloak-tls, 24h validity, no
// IPs) and provisionInterServiceCA's own authwebhook leaf cert (365d
// validity, no IPs) to avoid duplicating the ECDSA/x509/PEM boilerplate.
func generateSignedLeafCert(commonName string, dnsNames []string, ipAddrs []net.IP, validity time.Duration, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte, err error) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate %s leaf key: %w", commonName, err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddrs,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create %s leaf certificate: %w", commonName, err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal %s leaf key: %w", commonName, err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})
	return certPEM, keyPEM, nil
}

// signAndApplyLeafTLSSecret generates an ECDSA P-256 leaf certificate for
// commonName/dnsNames, signs it with caCert/caKey, and applies it as
// secretName (type kubernetes.io/tls) in namespace.
func signAndApplyLeafTLSSecret(ctx context.Context, kubeconfigPath, namespace, secretName, commonName string, dnsNames []string, ipAddrs []net.IP, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, writer io.Writer) error {
	leafCertPEM, leafKeyPEM, err := generateSignedLeafCert(commonName, dnsNames, ipAddrs, 24*time.Hour, caCert, caKey)
	if err != nil {
		return err
	}

	secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/tls
stringData:
  tls.crt: |
%s
  tls.key: |
%s`, secretName, namespace, indentPEM(string(leafCertPEM)), indentPEM(string(leafKeyPEM)))

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, secret); err != nil {
		return fmt.Errorf("failed to create %s Secret: %w", secretName, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ %s Secret created (signed by chart's inter-service CA)\n", secretName)
	return nil
}

// FleetHelmOptions carries the pre-provisioned fleet infrastructure
// (MCP Gateway + OAuth2 IdP) endpoints and credentials needed to render
// global.fleet.* correctly on the FIRST `operator reconciliation`, instead of patching
// it in afterward via kubectl once the operator-managed pods are already
// running (DD-TEST-015, "deploy correctly the first time" -- see
// SetupFleetE2EInfrastructure). Nil (the FP suite's default, via
// SetupFullPipelineInfrastructure's fleetProvisioner=nil) leaves fleet
// disabled, matching the chart's own global.fleet.enabled=false default.
//
// Every referenced Secret (OAuth2CredentialsSecret/WEOAuth2CredentialsSecret)
// MUST already exist in the target namespace before operator-backed deployment
// runs, and the MCP Gateway (MCPGatewayEndpoint) MUST already be reachable
// and AuthPolicy-converged -- see FleetProvisioner's doc comment for why.
type FleetHelmOptions struct {
	// MCPGatewayEndpoint is the shared Kuadrant/EAIGW MCP Gateway URL
	// (global.fleet.mcpGatewayEndpoint), e.g.
	// "https://mcp-gateway-istio.gateway-system.svc:8080/mcp".
	MCPGatewayEndpoint string
	// MCPGatewayType is "kuadrant" or "eaigw" (global.fleet.mcpGatewayType).
	MCPGatewayType string
	// MCPGatewayNamespace is the namespace where the MCP Gateway manages its
	// registration/backend resources (global.fleet.mcpGatewayNamespace).
	MCPGatewayNamespace string
	// OAuth2TokenURL is the IdP's client_credentials token endpoint
	// (global.fleet.oauth2.tokenURL).
	OAuth2TokenURL string
	// OAuth2CredentialsSecret is the pre-created K8s Secret name (keys:
	// client-id, client-secret) shared by every fleet-aware service except
	// WorkflowExecution (global.fleet.oauth2.credentialsSecretRef).
	OAuth2CredentialsSecret string
	// OAuth2Scopes is the OAuth2 scope list requested from the IdP
	// (global.fleet.oauth2.scopes).
	OAuth2Scopes []string
	// WEOAuth2CredentialsSecret is WorkflowExecution's OWN required
	// credentialsSecretRef (workflowexecution.fleet.oauth2.credentialsSecretRef)
	// -- REQUIRED with NO fallback to the shared secret above (AC-6 least
	// privilege, see values.yaml doc comment). Callers may point this at
	// the same Secret as OAuth2CredentialsSecret if per-service credential
	// differentiation isn't in scope (e.g. E2E).
	WEOAuth2CredentialsSecret string
	// SignalProcessingNamespace restricts SP's ClusterRegistry CRD watch
	// (signalprocessing.fleet.namespace) -- empty watches cluster-wide.
	SignalProcessingNamespace string
	// FleetMetadataCacheNamespace is the namespace where the MCP Gateway
	// manages its MCPServerRegistration/Backend CRs
	// (fleetmetadatacache.namespace) for operator-managed FMC. Issue #2298:
	// unlike SignalProcessingNamespace above, this has no safe empty-string
	// fallback -- pkg/fleet/fmc/config.Validate() refuses to start FMC
	// without it, so chart callers MUST set it. Hub-only AF E2E reuses this
	// field for the namespace of its raw-manifest FMC Service, because that
	// path does not pass FleetHelmOptions to Helm.
	FleetMetadataCacheNamespace string

	ImageTag string

	// OperatorExposure is the upstream test-owned host boundary allocated
	// before Kind creation for the operator-managed application.
	OperatorExposure *OperatorExposurePlan
}

// FleetProvisioner, when non-nil, is invoked by SetupFullPipelineInfrastructure
// once the target namespace exists (end of PHASE 5) but before `operator reconciliation`
// (PHASE 6), to deploy fleet-specific infrastructure (IdP, MCP Gateway,
// remote cluster) that must already be reachable and AuthPolicy-converged
// BEFORE any operator-managed pod boots with global.fleet.enabled=true --
// otherwise AF/EM/WE/GW/RO/SP would spend their first boot racing an MCP
// Gateway that doesn't exist yet, the exact race the old patch-after-deploy
// design (kubectl-patching fleet config into already-running pods once
// infra was ready) worked around instead of resolving. Returns the
// global.fleet.* values operator-backed deployment should render with.
//
// Kept as a callback (not a pre-computed *FleetHelmOptions) specifically
// because fleet infrastructure deployment needs a live Kind cluster + the
// target namespace, both of which only exist partway through
// SetupFullPipelineInfrastructure's own PHASE 1/2/5 -- this lets Fleet's
// provisioning share that single cluster-creation path with FP instead of
// duplicating it.
type FleetProvisioner func(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) (*FleetHelmOptions, error)

// provisionInterServiceCA self-signs a fresh ECDSA P-256 CA and creates both
// the authwebhook-tls Secret (tls.crt/tls.key/ca.crt/ca.key) AND the
// inter-service-ca ConfigMap (ca.crt) from it, matching exactly the shapes
// the chart's own tls-cert-gen pre-install hook
// (the retired application chart templates/hooks/tls-cert-job.yaml, Sections 1-2) would
// otherwise generate on its own -- but doing it here, BEFORE `operator reconciliation`
// runs, serves two purposes:
//
//  1. Keycloak (ensureKeycloakTLSFromOperatorCA) can get a leaf cert signed
//     from the SAME CA and be fully deployed and AuthPolicy-converged
//     before any operator-managed pod boots with global.fleet.enabled=true.
//  2. kube-mcp-server (deployKubeMCPServer, via DeployFleetGatewayInfra)
//     mounts the inter-service-ca ConfigMap as a REQUIRED (non-optional)
//     volume -- without pre-creating it here, that Deployment would sit in
//     ContainerCreating ("configmap \"inter-service-ca\" not found") since
//     it now deploys before `operator reconciliation`'s hook would otherwise create it.
//
// See FleetProvisioner's doc comment for why this ordering matters.
//
// The hook is idempotent (checks authwebhook-tls's expiry/ca.crt/ca.key
// presence and skips regeneration if valid -- tls-cert-job.yaml Section 1,
// lines 61-77), so pre-creating both resources here makes the hook a no-op
// for authwebhook's own cert and makes it reuse THIS CA for Section 2
// (gateway-tls/datastorage-tls/kubernautagent-tls/fleetmetadatacache-tls/
// apifrontend-tls generation, plus re-applying the same inter-service-ca
// ConfigMap content it finds already there), keeping every operator-managed
// service, kube-mcp-server, and Keycloak on one shared root of trust.
func provisionInterServiceCA(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Pre-provisioning inter-service CA + authwebhook-tls (deploy fleet correctly the first time)...")

	var caCert *x509.Certificate
	var caKey *ecdsa.PrivateKey
	var caCertPEM, caKeyPEM []byte
	var err error

	// Operator qualification pre-creates kubernaut-internal-ca so the MCP
	// Gateway listener can be HTTPS before the Kubernaut CR is applied. Reuse
	// that exact CA for authwebhook/Keycloak as well; otherwise operator pods
	// trust one root while the Fleet IdP uses another and OAuth2 token fetches
	// fail with an unknown-authority error.
	operatorCAPEM, operatorCAKeyPEM, operatorCAErr := readOperatorCASecret(ctx, kubeconfigPath, namespace)
	if operatorCAErr == nil {
		caBlock, _ := pem.Decode(operatorCAPEM)
		keyBlock, _ := pem.Decode(operatorCAKeyPEM)
		if caBlock == nil || keyBlock == nil {
			return errors.New("operator Fleet CA Secret contains invalid PEM material")
		}
		caCert, err = x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			return fmt.Errorf("parse operator Fleet CA certificate: %w", err)
		}
		caKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return fmt.Errorf("parse operator Fleet CA key: %w", err)
		}
		caCertPEM = operatorCAPEM
		caKeyPEM = operatorCAKeyPEM
		_, _ = fmt.Fprintln(writer, "  🔗 Reusing operator Fleet CA for authwebhook/Keycloak trust")
	} else if !errors.Is(operatorCAErr, errOperatorCASecretMissing) {
		return fmt.Errorf("read operator Fleet CA: %w", operatorCAErr)
	}

	if caCert == nil {
		secretCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
			"get", "secret", "authwebhook-tls", "--ignore-not-found", "-o", "name")
		existingSecret, err := secretCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to check for an existing inter-service CA: %w", err)
		}
		if strings.TrimSpace(string(existingSecret)) != "" {
			// Reusing a retained Kind cluster must preserve its existing trust root:
			// operator-managed service certificates and the running API server already
			// trust this CA. Rotating it here leaves retained pods with certificates
			// signed by the previous root and makes the next Keycloak/OIDC bootstrap
			// fail TLS verification.
			caCert, caKey, err = loadOperatorCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
			if err != nil {
				return fmt.Errorf("failed to reuse existing inter-service CA: %w", err)
			}
			caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
			caKeyDER, err := x509.MarshalECPrivateKey(caKey)
			if err != nil {
				return fmt.Errorf("failed to marshal reused CA private key: %w", err)
			}
			caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})
			_, _ = fmt.Fprintln(writer, "  ♻️ Reusing existing inter-service CA for retained cluster")
		} else {
			caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return fmt.Errorf("failed to generate CA key: %w", err)
			}
			caTemplate := &x509.Certificate{
				SerialNumber:          big.NewInt(time.Now().UnixNano()),
				Subject:               pkix.Name{CommonName: "authwebhook-ca"},
				NotBefore:             time.Now().Add(-1 * time.Hour),
				NotAfter:              time.Now().Add(365 * 24 * time.Hour),
				KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
				BasicConstraintsValid: true,
				IsCA:                  true,
			}
			caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
			if err != nil {
				return fmt.Errorf("failed to self-sign CA certificate: %w", err)
			}
			caCert, err = x509.ParseCertificate(caDER)
			if err != nil {
				return fmt.Errorf("failed to parse newly-created CA certificate: %w", err)
			}
			caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
			caKeyDER, err := x509.MarshalECPrivateKey(caKey)
			if err != nil {
				return fmt.Errorf("failed to marshal CA private key: %w", err)
			}
			caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})
		}
	}

	leafCertPEM, leafKeyPEM, err := generateSignedLeafCert(
		fmt.Sprintf("authwebhook.%s.svc", namespace),
		[]string{
			"authwebhook",
			fmt.Sprintf("authwebhook.%s", namespace),
			fmt.Sprintf("authwebhook.%s.svc", namespace),
			fmt.Sprintf("authwebhook.%s.svc.cluster.local", namespace),
		},
		nil, 365*24*time.Hour, caCert, caKey)
	if err != nil {
		return err
	}

	secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: authwebhook-tls
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: kubernaut-operator
    app.kubernetes.io/part-of: kubernaut
    app.kubernetes.io/instance: kubernaut
type: kubernetes.io/tls
stringData:
  tls.crt: |
%s
  tls.key: |
%s
  ca.crt: |
%s
  ca.key: |
%s`, namespace,
		indentPEM(string(leafCertPEM)), indentPEM(string(leafKeyPEM)),
		indentPEM(string(caCertPEM)), indentPEM(string(caKeyPEM)))

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, secret); err != nil {
		return fmt.Errorf("failed to create authwebhook-tls Secret: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ authwebhook-tls Secret created (self-signed CA, will be reused by the chart's tls-cert-gen hook)")

	caConfigMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: inter-service-ca
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: kubernaut-operator
    app.kubernetes.io/part-of: kubernaut
    app.kubernetes.io/instance: kubernaut
data:
  ca.crt: |
%s`, namespace, indentPEM(string(caCertPEM)))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, caConfigMap); err != nil {
		return fmt.Errorf("failed to create inter-service-ca ConfigMap: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ inter-service-ca ConfigMap created (kube-mcp-server's required tls-ca volume can now mount it)")

	// Write the CA PEM to the deterministic host-side path (InterServiceCAPath)
	// so host-side TLS-aware clients (NewTLSAwareClient, e.g. waitForKeycloakReady)
	// trust the SAME CA that was just used to sign every service's leaf cert in
	// THIS run. Without this, a stale inter-service-ca.pem left on disk by a
	// prior E2E run (same kubeconfig path, different self-signed CA) causes
	// every host-side HTTPS health check to fail TLS verification silently
	// and retry until timeout -- this bit us for Keycloak's readiness poll.
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	if err := os.WriteFile(caPEMPath, caCertPEM, 0600); err != nil {
		return fmt.Errorf("failed to write CA PEM to %s: %w", caPEMPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ CA PEM written to %s (host-side TLS clients now trust this run's CA)\n", caPEMPath)
	return nil
}

// ensureDexTLSFromOperatorCA creates the dex-tls Secret (tls.crt/tls.key) that
// deployDexOIDCProviderForAF's dex.yaml mounts, signed by the SAME
// inter-service CA the operator's own tls-cert-job.yaml pre-install hook
// generates for gateway-tls/datastorage-tls/kubernautagent-tls/
// fleetmetadatacache-tls (see the retired application chart templates/hooks/tls-cert-job.yaml
// Section 2). This is required, not cosmetic: KubernautAgent's JWT provider
// config (test/infrastructure/kubernautagent.go, tlsCaFile: /etc/tls-ca/ca.crt)
// validates DEX's JWKS endpoint against the chart's "inter-service-ca"
// ConfigMap -- a DEX cert signed by an unrelated/standalone CA would leave
// KubernautAgent unable to verify DEX's TLS handshake at all.
//
// Must run AFTER `operator reconciliation` (Phase 6): the chart's tls-cert-gen hook is
// pre-install and stores the CA's private key alongside authwebhook-tls
// (ca.crt/ca.key, base64) specifically so downstream consumers can mint
// additional leaf certs without regenerating the CA -- this mirrors that
// same pattern for the one leaf cert (dex) the chart itself doesn't need.
func ensureDexTLSFromOperatorCA(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Signing dex-tls leaf cert from the chart's inter-service CA...")
	caCert, caKey, err := loadOperatorCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
	if err != nil {
		return err
	}
	return signAndApplyLeafTLSSecret(ctx, kubeconfigPath, namespace, "dex-tls", "dex", []string{
		"localhost",
		"dex",
		fmt.Sprintf("dex.%s", namespace),
		fmt.Sprintf("dex.%s.svc", namespace),
		fmt.Sprintf("dex.%s.svc.cluster.local", namespace),
	}, []net.IP{net.IPv4(127, 0, 0, 1)}, caCert, caKey, writer)
}

// ensureKeycloakTLSFromOperatorCA creates the keycloak-tls Secret (tls.crt/
// tls.key) that DeployKeycloakInfra's Deployment mounts, signed by the SAME
// inter-service CA the operator's tls-cert-job.yaml pre-install hook
// generates (see ensureDexTLSFromOperatorCA above -- this is the "fleet" suite's
// exact counterpart to that DEX fix, Issue #1737 regression). Without this,
// keycloak-tls is never created (the Go-native harness's GenerateInterServiceTLS,
// which used to create it as a side effect of minting its own standalone CA,
// is no longer called now that the chart owns inter-service TLS), and the
// keycloak Deployment's Pod sits in ContainerCreating forever
// ("FailedMount ... secret \"keycloak-tls\" not found") until the 180s
// rollout-status wait in DeployKeycloakInfra times out.
//
// Must run AFTER `operator reconciliation` (Phase 6, via SetupFullPipelineInfrastructure)
// and BEFORE DeployKeycloakInfra -- mirrors ensureDexTLSFromOperatorCA's ordering
// constraint for the same reason (authwebhook-tls's ca.crt/ca.key must exist).
//
// namespace is where authwebhook-tls (and thus the chart CA) lives;
// secretNamespace is where the resulting keycloak-tls Secret is applied --
// these differ for the fleet DEMO entry point only (secretNamespace=
// idpNamespace), never for the "fleet"/"fullpipeline" Ginkgo suites (both
// equal namespace there, unchanged). DNS SANs cover BOTH namespace and
// secretNamespace so the leaf cert verifies regardless of which one the
// caller ends up dialing "keycloak" through.
func ensureKeycloakTLSFromOperatorCA(ctx context.Context, kubeconfigPath, namespace, secretNamespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Signing keycloak-tls leaf cert from the chart's inter-service CA...")
	caCert, caKey, err := loadOperatorCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
	if err != nil {
		return err
	}
	dnsNames := []string{
		"localhost",
		"keycloak",
		fmt.Sprintf("keycloak.%s", namespace),
		fmt.Sprintf("keycloak.%s.svc", namespace),
		fmt.Sprintf("keycloak.%s.svc.cluster.local", namespace),
	}
	if secretNamespace != namespace {
		dnsNames = append(dnsNames,
			fmt.Sprintf("keycloak.%s", secretNamespace),
			fmt.Sprintf("keycloak.%s.svc", secretNamespace),
			fmt.Sprintf("keycloak.%s.svc.cluster.local", secretNamespace),
		)
	}
	return signAndApplyLeafTLSSecret(ctx, kubeconfigPath, secretNamespace, "keycloak-tls", "keycloak", dnsNames,
		[]net.IP{net.IPv4(127, 0, 0, 1)}, caCert, caKey, writer)
}

// PR #1790 round-14 RCA: resignHostAccessedTLSCertsWithLocalhostSAN (which
// re-signed gateway-tls/datastorage-tls/apifrontend-tls/kubernautagent-tls
// post-install, then did a rolling restart+wait per deployment to pick them
// up) was removed. Its restart pass was the deterministic root cause of
// fleet E2E's PHASE 6 rollout timeouts -- see operator-backed deployment's
// `hooks.tlsCerts.extraSANs` comment for the full RCA. "localhost" SAN is
// now baked into the ORIGINAL operator-issued certs at `operator reconciliation` time via
// that values override, so no re-sign or restart is needed at all.

// writeInterServiceCAPEMFromOperator copies the chart-generated "inter-service-ca"
// ConfigMap's ca.crt (created by the pre-install tls-cert-gen hook, PHASE 6) to
// the host-side path InterServiceCAPath expects (Issue #785). Host-side E2E test
// clients (suite_test.go's NewTLSAwareTransport, NewTLSAwareClient) read the CA
// from that local file, not from the cluster directly -- with the Go-native
// harness this file was a side effect of GenerateInterServiceTLS generating the
// CA itself; now that the chart generates the CA in-cluster, this is the missing
// counterpart that fetches it back out for host-side use (Issue #1737).
func writeInterServiceCAPEMFromOperator(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
		"get", "configmap", "inter-service-ca", "-o", `jsonpath={.data.ca\.crt}`)
	caPEM, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read inter-service-ca ConfigMap: %w", err)
	}
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	if err := writeCombinedInterServiceCAPEM(caPEMPath, caPEM); err != nil {
		return fmt.Errorf("failed to write CA PEM to %s: %w", caPEMPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ inter-service CA PEM written to %s (Issue #785)\n", caPEMPath)
	return nil
}

// deployDexOIDCProviderForAF deploys the DEX OIDC provider test double that
// APIFrontend authenticates against (Issue #1189). DEX is test-only
// infrastructure -- it is not part of the production operator deployment -- extracted
// unchanged from the DEX step of the old (now removed) deployAPIFrontendInFP
// so it keeps working now that AF's Deployment/Service/CRD/RBAC come from
// `operator reconciliation` instead (Issue #1737).
func deployDexOIDCProviderForAF(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Deploying DEX OIDC provider (Issue #1189, AF auth test double)...")
	projectRoot := getProjectRoot()
	dexPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "dex.yaml")
	dexData, err := os.ReadFile(dexPath) //nolint:gosec // G304: test infrastructure path
	if err != nil {
		return fmt.Errorf("failed to read dex.yaml: %w", err)
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, string(dexData)); err != nil {
		return fmt.Errorf("failed to deploy DEX: %w", err)
	}
	return nil
}

// afPersonaGroupNames lists the persona names AF's SAR-based tool
// authorization recognizes (must match the retired application chart values's
// apifrontend.config.rbac.personas keys exactly -- see
// bindAFPersonaToolClusterRoles doc comment for why this list is duplicated
// here instead of read from the chart).
var afPersonaGroupNames = []string{
	"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver",
}

// bindAFPersonaToolClusterRoles creates the ClusterRoleBindings that map each
// DEX-issued OIDC "groups" claim (e.g. "sre") to the chart's own
// per-persona ClusterRoles (kubernaut-tool-<persona>, see
// the retired application chart templates/apifrontend/apifrontend.yaml).
//
// Issue #1737 gap found: the chart deliberately creates only the ClusterRoles
// -- its own comment says "Bind these ClusterRoles to OIDC groups via
// ClusterRoleBindings to grant tool access per persona", by design, since a
// production chart cannot presume to know the deployer's real OIDC group
// names. The old (now-removed) deployAPIFrontendInFP supplied this binding
// step for E2E via afDeployE2ERBAC -> PersonaToolClusterRolesYAML (which
// also duplicated the ClusterRoles themselves, now superseded by the chart).
// That whole call was dropped when deployAPIFrontendInFP was replaced by
// `operator reconciliation`, with nothing standing in for the ClusterRoleBinding half --
// leaving every kubernaut-tool-<persona> ClusterRole with zero subjects
// bound to it. AF's SAR checks (pkg/apifrontend/auth/sar.go) therefore denied
// every tool call regardless of persona; the denial is only *observable* for
// kubernaut_remediate (whose failure prevents RemediationRequest creation,
// verified by a real K8s API assertion) because AF's own audit trail
// (newAuditToolCallback/buildToolAuditDetail, pkg/apifrontend/agent/root.go)
// misreports a nil-error-but-denied tool call as tool_outcome=success for
// every other tool, masking the identical denial for
// kubernaut_investigate/discover_workflows/select_workflow/watch.
//
// The persona name list is intentionally re-declared here (afPersonaGroupNames)
// rather than parsed out of the retired application chart values: this is Go test
// infrastructure binding to chart-owned ClusterRoles by name/convention (same
// pattern as every other kubernaut-tool-<persona> reference in this package),
// not a second source of truth for which *tools* each persona can call --
// that authorization content lives solely in values.yaml/apifrontend.yaml.
func bindAFPersonaToolClusterRoles(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Binding AF persona ClusterRoles to DEX OIDC groups (Issue #1737 RBAC gap)...")

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildAFPersonaToolClusterRoleBindingsManifest()); err != nil {
		return fmt.Errorf("failed to bind AF persona tool ClusterRoles: %w", err)
	}
	return nil
}

// BuildAFPersonaToolClusterRoleBindingsManifest returns the OIDC group bindings
// required by AF's SAR-based A2A tool guard. The operator creates the persona
// ClusterRoles, while this qualification wiring supplies the deployment-specific
// identity-to-persona mapping.
func BuildAFPersonaToolClusterRoleBindingsManifest() string {
	var b strings.Builder
	for _, persona := range afPersonaGroupNames {
		fmt.Fprintf(&b, `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-tool-%s-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-tool-%s
subjects:
  - kind: Group
    name: %s
    apiGroup: rbac.authorization.k8s.io
`, persona, persona, persona)
	}
	return b.String()
}

// BindAFPersonaRBAC is the exported, post-`operator reconciliation` entry point for
// `make bind-fleet-af-rbac` (hack/bind-fleet-af-rbac): binds the chart's
// kubernaut-tool-<persona> ClusterRoles (only ever *created*, never *bound*,
// by the retired application chart templates/apifrontend -- see
// bindAFPersonaToolClusterRoles's doc comment, Issue #1737) to the OIDC
// groups issued by the Keycloak realm's kubernaut-console client
// (test/infrastructure/keycloak-realm-demo.json's "sre" group/user).
//
// Does NOT also bind kubernaut-console-access: verified via `helm template`
// that the chart already renders a kubernaut-console-access-<group>
// ClusterRoleBinding for every apifrontend.config.rbac.consoleAccessGroups
// entry directly (values.yaml's default list includes "sre") -- unlike the
// per-tool personas above, that one's genuinely self-contained, no manual
// step needed.
//
// Must run after operator reconciliation -- the ClusterRoles this
// binds don't exist until the chart's apifrontend templates render them.
// Reuses bindAFPersonaToolClusterRoles unchanged (same binding shape works
// for any IdP's "groups" claim, not just DEX's) rather than duplicating it.
func BindAFPersonaRBAC(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Creating and binding AF persona ClusterRoles for operator-managed AF...")
	personaToolRBAC, err := PersonaToolClusterRolesYAML()
	if err != nil {
		return fmt.Errorf("failed to generate persona tool ClusterRoles: %w", err)
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, personaToolRBAC); err != nil {
		return fmt.Errorf("failed to apply persona tool ClusterRoles and bindings: %w", err)
	}
	if err := waitForAFPersonaRBAC(ctx, kubeconfigPath, writer); err != nil {
		return err
	}
	return nil
}

func waitForAFPersonaRBAC(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("create Kubernetes client for AF persona RBAC verification: %w", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		result, sarErr := clientset.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   "sre-user",
				Groups: []string{"sre"},
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:     "use",
					Group:    "kubernaut.ai",
					Resource: "tools",
					Name:     "kubectl_get",
				},
			},
		}, metav1.CreateOptions{})
		if sarErr == nil && result.Status.Allowed {
			_, _ = fmt.Fprintln(writer, "  ✅ AF persona RBAC SAR verified for sre/kubectl_get")
			return nil
		}
		if sarErr != nil {
			err = sarErr
		} else {
			err = fmt.Errorf("SubjectAccessReview denied sre/kubectl_get: %s", result.Status.Reason)
		}
		if sleepErr := sleepOrDone(ctx, 2*time.Second); sleepErr != nil {
			return fmt.Errorf("wait for AF persona RBAC SAR: %w", sleepErr)
		}
	}
	return fmt.Errorf("AF persona RBAC SAR did not allow sre/kubectl_get within 60s: %w", err)
}

// createFullPipelineHelmSecrets creates the Secrets the operator expects to
// already exist (the chart deliberately does not generate credential material —
// original #239 audit finding #4, unchanged by this migration). Idempotent:
// uses `kubectl apply` so re-running (retry after failure) does not error.

// fullPipelineSignalProcessingPolicyRego and fullPipelineProactiveSignalMappingsYAML
// and fullPipelineAIAnalysisPolicyRego mirror EXACTLY the policy content the
// current Go-native harness injects via deploySignalProcessingPolicies (ADR-060)
// and createInlineRegoPolicyConfigMap, so switching to Helm's own policy
// ConfigMap templates (signalprocessing.policies.content /
// signalprocessing.proactiveSignalMappings.content / aianalysis.policies.content)
// preserves existing E2E test semantics byte-for-byte.
const fullPipelineSignalProcessingPolicyRego = `package signalprocessing

import rego.v1

# ========== Environment Classification (BR-SP-051-053) ==========
default environment := {"environment": "development", "source": "default"}

environment := {"environment": lower(env), "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}
environment := {"environment": "production", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    input.namespace.labels["env"] == "production"
}
environment := {"environment": "staging", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    input.namespace.labels["env"] == "staging"
}
environment := {"environment": "development", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    input.namespace.labels["env"] == "development"
}

# ========== Severity Determination (BR-SP-105) ==========
default severity := "warning"

severity := "critical" if { lower(input.signal.severity) == "critical" }
severity := "critical" if { lower(input.signal.severity) == "sev1" }
severity := "critical" if { lower(input.signal.severity) == "p0" }
severity := "high" if { lower(input.signal.severity) == "high" }
severity := "high" if { lower(input.signal.severity) == "sev2" }
severity := "high" if { lower(input.signal.severity) == "p2" }
severity := "warning" if { lower(input.signal.severity) == "medium" }
severity := "warning" if { lower(input.signal.severity) == "warning" }
severity := "warning" if { lower(input.signal.severity) == "sev3" }
severity := "info" if { lower(input.signal.severity) == "low" }
severity := "info" if { lower(input.signal.severity) == "info" }
severity := "info" if { lower(input.signal.severity) == "sev4" }

# ========== Priority Assignment (BR-SP-070) ==========
default priority := {"priority": "P3", "policy_name": "default"}

priority := {"priority": "P0", "policy_name": "production-critical"} if {
    environment.environment == "production"
    severity == "critical"
}
priority := {"priority": "P1", "policy_name": "production-high"} if {
    environment.environment == "production"
    severity == "high"
}
priority := {"priority": "P1", "policy_name": "production-warning"} if {
    environment.environment == "production"
    severity == "warning"
}
priority := {"priority": "P1", "policy_name": "staging-critical"} if {
    environment.environment == "staging"
    severity == "critical"
}
priority := {"priority": "P2", "policy_name": "staging-any"} if {
    environment.environment == "staging"
    severity != "critical"
}

# ========== Custom Labels (BR-SP-102) ==========
default labels := {}

labels := {"team": [team], "tier": [tier]} if {
    team := input.namespace.labels["kubernaut.ai/team"]
    team != ""
    tier := input.namespace.labels["kubernaut.ai/tier"]
    tier != ""
}

labels := {"team": [team]} if {
    team := input.namespace.labels["kubernaut.ai/team"]
    team != ""
    not input.namespace.labels["kubernaut.ai/tier"]
}

# ========== Cluster Classification (BR-FLEET-003, #1511) ==========
# Optional: classifies the fleet cluster (via input.cluster.labels, sourced
# from the MCP Gateway's cluster-registration CRD) into a business value
# consumed downstream by DataStorage workflow discovery. No default rule --
# an undefined result is "no classification" (non-fleet/unregistered clusters).
cluster := input.cluster.labels.environment if {
    input.cluster.labels.environment != ""
}
`

const fullPipelineAIAnalysisPolicyRego = `package aianalysis.approval
import rego.v1

default require_approval := false

is_production if { lower(input.environment) == "production" }

require_approval if { is_production }
require_approval if { is_production; count(input.warnings) > 0 }
require_approval if { is_production; count(input.failed_detections) > 0 }

# Scored risk factors for reason generation (issue #98)
risk_factors contains {"score": 70, "reason": "Data quality warnings in production environment"} if {
    is_production
    count(input.warnings) > 0
}
risk_factors contains {"score": 60, "reason": "Data quality issues detected in production environment"} if {
    is_production
    count(input.failed_detections) > 0
}
risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
    is_production
}

all_scores contains f.score if { some f in risk_factors }
max_risk_score := max(all_scores) if { count(all_scores) > 0 }
reason := f.reason if { some f in risk_factors; f.score == max_risk_score }
default reason := "Auto-approved"
`
