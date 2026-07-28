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
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Full Pipeline E2E — Helm Chart Deployment (Issue #1737)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Replaces the manual Go-managed deployment of the 12 chart-managed Kubernaut
// services with a single `helm install charts/kubernaut`. This validates that
// the Helm chart itself deploys and configures a working system, closing the
// gap left by helm-smoke-test.sh (deployment-only, no functional assertions).
//
// Split-infra design (spike-validated, Issue #1737):
//   Chart-managed (this file):   Gateway, DataStorage, AuthWebhook, SignalProcessing,
//                                RemediationOrchestrator, AIAnalysis, WorkflowExecution,
//                                Notification, KubernautAgent, EffectivenessMonitor,
//                                APIFrontend, FleetMetadataCache, PostgreSQL, Redis/Valkey,
//                                DB migrations, inter-service TLS, AU-9 signing cert,
//                                DataStorage audit RBAC, CRDs (chart's own crds/ dir).
//   Go-managed (unchanged):      Mock LLM (+Shadow), mock-slack, Prometheus, AlertManager,
//                                kubernetes-event-exporter, memory-eater, workflow catalog
//                                seeding, workflow-job-executor RBAC (test fixture SA),
//                                Gateway SA/token for event-exporter+AlertManager webhooks.
//
// Image strategy: regardless of how Phase 1 obtained each image (local build,
// registry pull, or CI-artifact reuse), all 12 chart-managed images are retagged
// to one shared `localhost/<service>:<sharedTag>` reference before `helm install`,
// because the chart's `global.image.tag` is a single value shared by every
// service template. See ensureSharedChartImageTag.
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// fullPipelineChartServices lists the fullPipelineImageConfigs ServiceName keys
// that map onto the Helm chart's per-service image overrides (i.e., everything
// EXCEPT mock-llm, which stays Go-managed test infrastructure).
var fullPipelineChartServices = []string{
	"gateway",
	"signalprocessing",
	"remediationorchestrator",
	"aianalysis",
	"workflowexecution",
	"notification",
	"datastorage",
	"authwebhook",
	"kubernautagent",
	"effectivenessmonitor",
	"apifrontend",
	"fleetmetadatacache",
}

// ensureSharedChartImageTag retags all fullPipelineChartServices entries in
// builtImages to a single shared `localhost/<service>:<tag>` reference and
// mutates builtImages in place so the existing loadFullPipelineImages (Phase 3)
// loads the retagged images into Kind unchanged.
//
// Why: Helm's global.image.tag is one value applied to every service template.
// buildFullPipelineImages/BuildImageForKind may hand back per-service registry
// refs (CI/CD mode), a shared CI-artifact tag, or per-service random local-build
// tags — retagging here normalizes all three into one shared local reference,
// regardless of source, so the chart's image resolution is always exercised the
// same well-understood way (validated by the Issue #1737 spike pilot).
//
// Returns the registry ("localhost") and shared tag to pass as
// --set global.image.registry / --set global.image.tag.
func ensureSharedChartImageTag(ctx context.Context, builtImages map[string]string, writer io.Writer) (registry, tag string, err error) {
	sharedTag := "fp-helm-" + uuid.New().String()[:8]
	_, _ = fmt.Fprintf(writer, "  🏷️  Normalizing %d chart-managed images to shared tag localhost/*:%s...\n",
		len(fullPipelineChartServices), sharedTag)

	for _, svc := range fullPipelineChartServices {
		src, ok := builtImages[svc]
		if !ok || src == "" {
			return "", "", fmt.Errorf("no built image found for chart-managed service %q", svc)
		}
		dst := fmt.Sprintf("localhost/%s:%s", svc, sharedTag)
		cmd := exec.CommandContext(ctx, "podman", "tag", src, dst)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if runErr := cmd.Run(); runErr != nil {
			return "", "", fmt.Errorf("failed to retag %s (%s -> %s): %w", svc, src, dst, runErr)
		}
		builtImages[svc] = dst
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Retagged %d images to shared tag %s\n", len(fullPipelineChartServices), sharedTag)
	return "localhost", sharedTag, nil
}

// ensureDBMigrateImage builds (or reuses a CI-prebuilt artifact for) the
// db-migrate image (docker/db-migrate.Dockerfile) and retags it to the same
// shared `localhost/db-migrate:<sharedTag>` reference as the 12 app images,
// so the chart's post-install migration-job hook
// (charts/kubernaut/templates/hooks/migration-job.yaml, `kubernaut.image`
// helper) resolves it via the same global.image.registry/tag override.
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

// loadChartCAFromAuthwebhookTLS extracts the CA certificate + private key that
// the Helm chart's tls-cert-gen pre-install hook embeds in authwebhook-tls
// (ca.crt/ca.key, base64) so downstream consumers can mint additional leaf
// certs signed by the SAME CA without regenerating it.
func loadChartCAFromAuthwebhookTLS(ctx context.Context, kubeconfigPath, namespace string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
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

// signAndApplyLeafTLSSecret generates an ECDSA P-256 leaf certificate for
// commonName/dnsNames, signs it with caCert/caKey, and applies it as
// secretName (type kubernetes.io/tls) in namespace.
func signAndApplyLeafTLSSecret(ctx context.Context, kubeconfigPath, namespace, secretName, commonName string, dnsNames []string, ipAddrs []net.IP, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, writer io.Writer) error {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate %s leaf key: %w", secretName, err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddrs,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create %s leaf certificate: %w", secretName, err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return fmt.Errorf("failed to marshal %s leaf key: %w", secretName, err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})

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

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(secret)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create %s Secret: %w", secretName, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ %s Secret created (signed by chart's inter-service CA)\n", secretName)
	return nil
}

// ensureDexTLSFromChartCA creates the dex-tls Secret (tls.crt/tls.key) that
// deployDexOIDCProviderForAF's dex.yaml mounts, signed by the SAME
// inter-service CA the Helm chart's own tls-cert-job.yaml pre-install hook
// generates for gateway-tls/datastorage-tls/kubernautagent-tls/
// fleetmetadatacache-tls (see charts/kubernaut/templates/hooks/tls-cert-job.yaml
// Section 2). This is required, not cosmetic: KubernautAgent's JWT provider
// config (test/infrastructure/kubernautagent.go, tlsCaFile: /etc/tls-ca/ca.crt)
// validates DEX's JWKS endpoint against the chart's "inter-service-ca"
// ConfigMap -- a DEX cert signed by an unrelated/standalone CA would leave
// KubernautAgent unable to verify DEX's TLS handshake at all.
//
// Must run AFTER `helm install` (Phase 6): the chart's tls-cert-gen hook is
// pre-install and stores the CA's private key alongside authwebhook-tls
// (ca.crt/ca.key, base64) specifically so downstream consumers can mint
// additional leaf certs without regenerating the CA -- this mirrors that
// same pattern for the one leaf cert (dex) the chart itself doesn't need.
func ensureDexTLSFromChartCA(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Signing dex-tls leaf cert from the chart's inter-service CA...")
	caCert, caKey, err := loadChartCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
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

// resignHostAccessedTLSCertsWithLocalhostSAN re-signs gateway-tls,
// datastorage-tls, apifrontend-tls, and kubernautagent-tls (still using the
// chart's own CA from authwebhook-tls) to ADD "localhost"/127.0.0.1 to their SANs,
// overwriting the chart-generated leaf certs that the chart's
// tls-cert-job.yaml hook produced.
//
// Why: the chart's own hook (Section 2, SVC_NAME loop) deliberately omits
// "localhost"/127.0.0.1 from these SANs -- correct for a real cluster, which
// never accesses these Services via localhost. But host-side test clients
// dial DataStorage over "https://localhost:30081" and Kubernaut Agent's MCP
// endpoint over "https://localhost:8088", and AF's over "https://localhost:30443"
// via the chart's pinned NodePorts
// (Issue #1737: type=NodePort + networkPolicies.<svc>.ingressCIDRs=0.0.0.0/0
// admits the NodePort-sourced, SNAT'd-to-node-IP traffic through the
// default-deny NetworkPolicy) with full hostname verification (suite_test.go's
// NewTLSAwareClient/NewTLSAwareTransport, Issue #785) -- without "localhost"
// in the SAN, that handshake fails ("read: connection reset by peer",
// confirmed during Issue #1737 validation). gateway-tls is re-signed too even
// though Gateway itself still serves plain HTTP today (TLS wiring is a
// separate, tracked follow-up) -- keeping all certs consistent avoids a
// silent trap for whoever wires Gateway's HTTPS listener next. The Go-native
// harness's GenerateInterServiceTLS always included these SANs for exactly
// this reason; re-signing here restores that host-access property without
// modifying the chart's own hook (which stays correct for production).
func resignHostAccessedTLSCertsWithLocalhostSAN(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Re-signing gateway-tls/datastorage-tls/apifrontend-tls/kubernautagent-tls with localhost SAN (host NodePort access)...")
	caCert, caKey, err := loadChartCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
	if err != nil {
		return err
	}
	targets := []struct {
		secretName string
		commonName string
		svcName    string
	}{
		{"gateway-tls", "gateway-service", "gateway-service"},
		{"datastorage-tls", "data-storage-service", "data-storage-service"},
		{"apifrontend-tls", "apifrontend", "apifrontend"},
		{"kubernautagent-tls", "kubernaut-agent", "kubernaut-agent"},
	}
	for _, t := range targets {
		dnsNames := []string{
			"localhost",
			t.svcName,
			fmt.Sprintf("%s.%s", t.svcName, namespace),
			fmt.Sprintf("%s.%s.svc", t.svcName, namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", t.svcName, namespace),
		}
		if err := signAndApplyLeafTLSSecret(ctx, kubeconfigPath, namespace, t.secretName, t.commonName, dnsNames, []net.IP{net.IPv4(127, 0, 0, 1)}, caCert, caKey, writer); err != nil {
			return err
		}
	}

	// gateway/datastorage/apifrontend/kubernaut-agent load tls.crt/tls.key once
	// at startup (no hot-reload for the server's own leaf cert, unlike the CA
	// bundle) -- all pods are already Running from PHASE 6's `helm install`
	// using the ORIGINAL chart-generated certs, so a restart is required for
	// them to pick up the re-signed ones above.
	deployments := []string{"gateway", "datastorage", "apifrontend", "kubernaut-agent"}
	for _, deploy := range deployments {
		restartCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
			"rollout", "restart", "deployment/"+deploy)
		restartCmd.Stdout = writer
		restartCmd.Stderr = writer
		if err := restartCmd.Run(); err != nil {
			return fmt.Errorf("failed to restart %s for TLS cert reload: %w", deploy, err)
		}
	}
	for _, deploy := range deployments {
		waitCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
			"rollout", "status", "deployment/"+deploy, "--timeout=120s")
		waitCmd.Stdout = writer
		waitCmd.Stderr = writer
		if err := waitCmd.Run(); err != nil {
			return fmt.Errorf("failed waiting for %s rollout after TLS cert reload: %w", deploy, err)
		}
	}
	_, _ = fmt.Fprintln(writer, "  ✅ gateway/datastorage/apifrontend/kubernaut-agent restarted with localhost-SAN certs")
	return nil
}

// writeInterServiceCAPEMFromCluster copies the chart-generated "inter-service-ca"
// ConfigMap's ca.crt (created by the pre-install tls-cert-gen hook, PHASE 6) to
// the host-side path InterServiceCAPath expects (Issue #785). Host-side E2E test
// clients (suite_test.go's NewTLSAwareTransport, NewTLSAwareClient) read the CA
// from that local file, not from the cluster directly -- with the Go-native
// harness this file was a side effect of GenerateInterServiceTLS generating the
// CA itself; now that the chart generates the CA in-cluster, this is the missing
// counterpart that fetches it back out for host-side use (Issue #1737).
func writeInterServiceCAPEMFromCluster(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
		"get", "configmap", "inter-service-ca", "-o", `jsonpath={.data.ca\.crt}`)
	caPEM, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read inter-service-ca ConfigMap: %w", err)
	}
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	if err := os.WriteFile(caPEMPath, caPEM, 0o600); err != nil {
		return fmt.Errorf("failed to write CA PEM to %s: %w", caPEMPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ inter-service CA PEM written to %s (Issue #785)\n", caPEMPath)
	return nil
}

// deployDexOIDCProviderForAF deploys the DEX OIDC provider test double that
// APIFrontend authenticates against (Issue #1189). DEX is test-only
// infrastructure -- it is not part of the production Helm chart -- extracted
// unchanged from the DEX step of the old (now removed) deployAPIFrontendInFP
// so it keeps working now that AF's Deployment/Service/CRD/RBAC come from
// `helm install` instead (Issue #1737).
func deployDexOIDCProviderForAF(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Deploying DEX OIDC provider (Issue #1189, AF auth test double)...")
	projectRoot := getProjectRoot()
	dexPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "dex.yaml")
	dexData, err := os.ReadFile(dexPath) //nolint:gosec // G304: test infrastructure path
	if err != nil {
		return fmt.Errorf("failed to read dex.yaml: %w", err)
	}
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(dexData))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy DEX: %w", err)
	}
	return nil
}

// afPersonaGroupNames lists the persona names AF's SAR-based tool
// authorization recognizes (must match charts/kubernaut/values.yaml's
// apifrontend.config.rbac.personas keys exactly -- see
// bindAFPersonaToolClusterRoles doc comment for why this list is duplicated
// here instead of read from the chart).
var afPersonaGroupNames = []string{
	"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver",
}

// bindAFPersonaToolClusterRoles creates the ClusterRoleBindings that map each
// DEX-issued OIDC "groups" claim (e.g. "sre") to the chart's own
// per-persona ClusterRoles (kubernaut-tool-<persona>, see
// charts/kubernaut/templates/apifrontend/apifrontend.yaml).
//
// Issue #1737 gap found: the chart deliberately creates only the ClusterRoles
// -- its own comment says "Bind these ClusterRoles to OIDC groups via
// ClusterRoleBindings to grant tool access per persona", by design, since a
// production chart cannot presume to know the deployer's real OIDC group
// names. The old (now-removed) deployAPIFrontendInFP supplied this binding
// step for E2E via afDeployE2ERBAC -> PersonaToolClusterRolesYAML (which
// also duplicated the ClusterRoles themselves, now superseded by the chart).
// That whole call was dropped when deployAPIFrontendInFP was replaced by
// `helm install`, with nothing standing in for the ClusterRoleBinding half --
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
// rather than parsed out of charts/kubernaut/values.yaml: this is Go test
// infrastructure binding to chart-owned ClusterRoles by name/convention (same
// pattern as every other kubernaut-tool-<persona> reference in this package),
// not a second source of truth for which *tools* each persona can call --
// that authorization content lives solely in values.yaml/apifrontend.yaml.
func bindAFPersonaToolClusterRoles(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Binding AF persona ClusterRoles to DEX OIDC groups (Issue #1737 RBAC gap)...")

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

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(b.String())
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bind AF persona tool ClusterRoles: %w", err)
	}
	return nil
}

// createFullPipelineHelmSecrets creates the Secrets the Helm chart expects to
// already exist (the chart deliberately does not generate credential material —
// original #239 audit finding #4, unchanged by this migration). Idempotent:
// uses `kubectl apply` so re-running (retry after failure) does not error.
func createFullPipelineHelmSecrets(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔑 Creating pre-req Secrets for Helm chart (postgresql, valkey, llm-credentials)...")

	// Key names are the chart's own documented contract
	// (charts/kubernaut/templates/infrastructure/secrets.yaml): postgresql-secret
	// needs POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB (consumed directly by the
	// postgres:16-alpine container) plus db-secrets.yaml (YAML, consumed by
	// DataStorage); valkey-secret needs valkey-secrets.yaml (YAML).
	//
	// slm_user (not an arbitrary name): charts/kubernaut/templates/_helpers.tpl's
	// kubernaut.postgresql.username defaults to "slm_user" (.Values.postgresql.
	// auth.username not overridden here), and the db-migration hook's PGUSER/GRANT
	// target both resolve through that same helper. postgres:16-alpine only ever
	// creates the ONE role named by POSTGRES_USER at bootstrap, so this secret's
	// username MUST match the helper's default exactly or the migration Job's
	// `goose ... postgres ...` connection fails with "password authentication
	// failed" (role doesn't exist) -- confirmed during Issue #1737 validation.
	// "slm_user" also matches the pre-existing convention used by every other
	// Go-managed E2E/IT harness in this package (datastorage.go, migrations.go,
	// authwebhook_e2e.go, etc.) rather than introducing a second one.
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
  namespace: %[1]s
type: Opaque
stringData:
  POSTGRES_USER: slm_user
  POSTGRES_PASSWORD: kubernaut-e2e-password
  # action_history is DataStorage's hardcoded default database name
  # (DD-ADR-030 config.yaml), independent of the secret's own naming --
  # POSTGRES_DB must match it exactly so the postgres:16-alpine container
  # auto-creates the database DataStorage will actually connect to.
  POSTGRES_DB: action_history
  db-secrets.yaml: |
    username: slm_user
    password: kubernaut-e2e-password
---
apiVersion: v1
kind: Secret
metadata:
  name: valkey-secret
  namespace: %[1]s
type: Opaque
stringData:
  valkey-secrets.yaml: |
    password: ""
---
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials-primary
  namespace: %[1]s
type: Opaque
stringData:
  api_key: mock-llm-e2e-key
`, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Helm chart prerequisite secrets: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Secrets ready")
	return nil
}

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
default environment := {"environment": "unknown", "source": "default"}

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
default severity := "unknown"

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

const fullPipelineProactiveSignalMappingsYAML = `# BR-SP-106: Proactive Signal Mode Classification (E2E test config)
proactive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
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

// InstallFullPipelineHelmChart runs `helm install` for all 12 chart-managed
// Full Pipeline services using charts/kubernaut, replacing the manual
// PostgreSQL/Redis/DataStorage/AuthWebhook/controllers/Gateway/KA/EM/APIFrontend
// deployment phases. Namespace must already exist (createTestNamespace) and
// prerequisite Secrets must already be applied (createFullPipelineHelmSecrets).
//
// registry/tag: from ensureSharedChartImageTag (always "localhost"/<sharedTag>).
func InstallFullPipelineHelmChart(ctx context.Context, kubeconfigPath, namespace, registry, tag string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	chartPath := filepath.Join(projectRoot, "charts", "kubernaut")

	tmpDir, err := os.MkdirTemp("", "fp-helm-policies-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for policy files: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Failed to clean up temp policy dir %s: %v\n", tmpDir, rmErr)
		}
	}()

	spPolicyFile := filepath.Join(tmpDir, "sp-policy.rego")
	spMappingsFile := filepath.Join(tmpDir, "sp-proactive-mappings.yaml")
	aaPolicyFile := filepath.Join(tmpDir, "aa-policy.rego")
	for path, content := range map[string]string{
		spPolicyFile:   fullPipelineSignalProcessingPolicyRego,
		spMappingsFile: fullPipelineProactiveSignalMappingsYAML,
		aaPolicyFile:   fullPipelineAIAnalysisPolicyRego,
	} {
		if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("failed to write policy file %s: %w", path, writeErr)
		}
	}

	args := []string{
		"--kubeconfig", kubeconfigPath,
		"install", "kubernaut", chartPath,
		"--namespace", namespace,
		"--timeout", "8m",
		// Deliberately no --wait: charts/kubernaut/templates/hooks/migration-job.yaml
		// is a post-install hook (helm.sh/hook: post-install). Helm's --wait only
		// waits for the *main* release resources (Deployments) to become ready and
		// only runs post-install hooks AFTER that wait succeeds -- but DataStorage's
		// Deployment can never become ready without the migration having already
		// run (missing tables), which deadlocks --wait until the timeout (confirmed
		// empirically during Issue #1737 validation). Hooks always block for their
		// own completion regardless of --wait, so omitting it here still guarantees
		// the migration Job finishes before this function returns; DataStorage's
		// own crash-loop-and-retry self-heals once the migration lands, and PHASE 8
		// (waitForFullPipelineServicesReady) confirms actual readiness afterward.
		"--set", "global.image.registry=" + registry,
		"--set", "global.image.namespace=",
		"--set", "global.image.tag=" + tag,
		// IfNotPresent (not Never): global.image.pullPolicy also governs the
		// public postgres/valkey/db-migrate images the chart pulls itself,
		// which are NOT loaded into Kind and must remain pullable.
		"--set", "global.image.pullPolicy=IfNotPresent",
		"--set", "global.llmProfiles.primary.provider=openai_compatible",
		"--set", "global.llmProfiles.primary.model=mock-model",
		"--set", "global.llmProfiles.primary.endpoint=http://mock-llm." + namespace + ".svc.cluster.local:8080",
		"--set", "global.llmProfiles.primary.credentialsSecretName=llm-credentials-primary",
		"--set", "kubernautAgent.llmProfileRef=primary",
		// MCP interactive mode (#703) is opt-in (default false) -- without this,
		// KA never registers the POST /api/v1/mcp Streamable HTTP handler at all,
		// so every MCP client `initialize` call gets a plain HTTP 404 from Go's
		// default mux (no route registered), which the MCP SDK's client-side
		// checkResponse() unconditionally reinterprets as ErrSessionMissing
		// ("session not found") regardless of the actual cause (Issue #1737 gap
		// found: every interactive/mcp-labeled FP E2E spec failed this way against
		// the Helm-deployed chart, whose default leaves interactive mode off).
		"--set", "kubernautAgent.interactive.enabled=true",
		// Gateway/DataStorage/APIFrontend: pinned chart-level NodePort (DD-TEST-001
		// port allocation: 30080/30081/30443, pre-mapped host-reachable in
		// kind-fullpipeline-config.yaml's extraPortMappings) + ipBlock ingressCIDRs
		// to admit that NodePort-sourced traffic through the default-deny
		// NetworkPolicy (SNAT'd to the node's own IP by kube-proxy, so it matches
		// no podSelector/namespaceSelector no matter how broadly scoped -- Issue
		// #1737). This is a production-safe, default-off chart knob (type stays
		// ClusterIP, nodePort stays 0/unset unless explicitly overridden), not
		// test-only chart surface: it's the same "type=NodePort + ipBlock" pattern
		// a real bare-metal/on-prem install with fixed firewall/LB rules would use.
		"--set", "gateway.service.type=NodePort",
		"--set", "gateway.service.nodePort=30080",
		"--set", "datastorage.service.type=NodePort",
		"--set", "datastorage.service.nodePort=30081",
		"--set", "apifrontend.service.type=NodePort",
		"--set", "apifrontend.service.nodePort=30443",
		// AF OIDC auth (Issue #1737 gap found): the chart mounts apifrontend-tls
		// but never told AF's own auth middleware about an OIDC provider, so it
		// always fell back to K8s TokenReview-only mode -- every DEX-token-based
		// A2A test got 401 regardless of the TLS fix above. issuerURL/jwksURL
		// match deploy/apifrontend/overlays/e2e/dex.yaml's hardcoded
		// `issuer: https://dex:5556/dex` (DEX stamps this exact string into the
		// JWT's iss claim, so it must match verbatim -- not a namespace-qualified
		// FQDN) and audience matches AF_FP_CLIENT_ID's default
		// ("kubernaut-apifrontend") in suite_test.go/06_af_audit_trace_test.go.
		// oidcCaFile trusts DEX's leaf cert, which ensureDexTLSFromChartCA signs
		// with the chart's own inter-service CA (mounted automatically by the
		// chart once oidcCaFile is set -- see apifrontend.yaml's tlsCaVolume(Mount)
		// inclusion). This is the same DEX test double the pre-#1737 Go-native
		// harness (deployAPIFrontendInFP, Issue #1189) wired up directly; Fleet
		// E2E is the one using Keycloak, for FMC's token-exchange-dependent OAuth2
		// flow that these AF auth-middleware tests don't exercise.
		"--set", "apifrontend.config.auth.issuerURL=https://dex:5556/dex",
		"--set", "apifrontend.config.auth.jwksURL=https://dex:5556/dex/keys",
		"--set", "apifrontend.config.auth.audience=kubernaut-apifrontend",
		"--set", "apifrontend.config.auth.oidcCaFile=/etc/tls-ca/ca.crt",
		// Setting auth.issuerURL above also activates AF's NetworkPolicy
		// kubernaut.np.idpEgress egress rule (networkpolicy.yaml), but that rule
		// defaults to port 443 (values.yaml's networkPolicies.idp.port) -- DEX
		// listens on 5556, so without this override AF's egress to DEX's JWKS
		// endpoint is silently dropped by the default-deny NetworkPolicy even
		// though the OIDC config itself is fully correct (confirmed via AF pod
		// logs showing "auth mode: OIDC/JWKS" yet still 401ing every DEX-signed
		// token -- Issue #1737 gap found).
		"--set", "networkPolicies.idp.port=5556",
		// AF LLM egress (Issue #1737 gap found, distinct from the OIDC 401 fix
		// above): apifrontend.config.agent.llm.endpoint is mandatory AF config
		// (the A2A launcher agent calls its LLM directly, not just via KA), but
		// AF's NetworkPolicy egress allowlist never included it -- every A2A
		// call timed out with a TCP dial timeout once the 401 fix let requests
		// reach this stage. mock-llm listens on 8080 in-cluster; a real LLM
		// provider (OpenAI/Vertex AI/etc.) would use the chart's own default of
		// 443, so this override is E2E-specific, not a chart bug.
		"--set", "networkPolicies.llm.port=8080",
		// Kubernaut Agent: same pinned-NodePort + ipBlock pattern as above, for
		// the MCP interactive-session E2E tests that dial https://localhost:8088
		// directly (Kind's extraPortMappings maps hostPort 8088 -> containerPort
		// 30088, per DD-TEST-001). Without this KA stays ClusterIP-only (no
		// listener on 30088 for Kind to forward to) and every MCP/interactive
		// spec fails with "connection reset by peer" (Issue #1737 gap found
		// during first full-suite run against the Helm-deployed chart).
		"--set", "kubernautAgent.service.type=NodePort",
		"--set", "kubernautAgent.service.nodePort=30088",
		"--set", `networkPolicies.gateway.ingressCIDRs[0]=0.0.0.0/0`,
		"--set", `networkPolicies.datastorage.ingressCIDRs[0]=0.0.0.0/0`,
		"--set", `networkPolicies.apifrontend.ingressCIDRs[0]=0.0.0.0/0`,
		"--set", `networkPolicies.kubernautAgent.ingressCIDRs[0]=0.0.0.0/0`,
		// Event-exporter (Go-managed test infra) lives in the same namespace and
		// sends signals to Gateway in-cluster; without this the default-deny
		// NetworkPolicy blocks it (Issue #1737 spike finding).
		"--set", "networkPolicies.gateway.ingressNamespaces[0]=" + namespace,
		// Prometheus/AlertManager are Go-managed test infra (chart-external,
		// ADR-EM-001) deployed into the same namespace by DeployPrometheus/
		// DeployAlertManager.
		"--set", "monitoring.prometheus.enabled=true",
		"--set", "monitoring.prometheus.url=http://prometheus-svc." + namespace + ".svc.cluster.local:9090",
		"--set", "monitoring.alertManager.enabled=true",
		"--set", "monitoring.alertManager.url=http://alertmanager-svc." + namespace + ".svc.cluster.local:9093",
		"--set", "networkPolicies.monitoring.namespace=" + namespace,
		// EffectivenessAssessment stabilization window (Issue #1737 gap found):
		// RO stamps this value onto every EA CRD it creates (spec.stabilizationWindow,
		// see pkg/remediationorchestrator/creator/effectivenessassessment.go); EM's
		// controller then blocks assessment until this window elapses. The chart's
		// production default (remediationorchestrator.config.effectivenessAssessment.
		// stabilizationWindow, values.yaml) is "5m" -- correct for real workloads,
		// where health/alert/metric signals need time to settle -- but the old
		// (now-removed) Go-native FP harness's RO ConfigMap
		// (remediationorchestrator_e2e_hybrid.go) used "10s" specifically for fast
		// E2E cycles. Without this override every EA blocked for the full 5m
		// chart default, confirmed via full-suite E2E to add 5-7.5m of pure wait
		// time to both "Full Remediation Lifecycle" specs (BR-E2E-001) alone. EM's
		// own config.assessment.{stabilizationWindow,validityWindow} chart defaults
		// (30s/120s) already match the old Go-native harness's
		// effectivenessmonitor_e2e.go values verbatim, so no override needed there.
		"--set", "remediationorchestrator.config.effectivenessAssessment.stabilizationWindow=10s",
		"--set-file", "signalprocessing.policies.content=" + spPolicyFile,
		"--set-file", "signalprocessing.proactiveSignalMappings.content=" + spMappingsFile,
		"--set-file", "aianalysis.policies.content=" + aaPolicyFile,
	}

	_, _ = fmt.Fprintln(writer, "  🚀 helm install kubernaut charts/kubernaut ...")
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Helm chart installed (12 chart-managed services + PostgreSQL/Redis)")
	return nil
}
