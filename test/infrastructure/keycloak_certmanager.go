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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// reloaderManifestURL installs stakater/Reloader (verified current release,
// v1.4.21) into the "default" namespace, matching its own upstream manifest
// convention. cert-manager itself is installed separately via the shared
// InstallCertManager/WaitForCertManagerReady helpers (test/infrastructure/datastorage.go).
const reloaderManifestURL = "https://raw.githubusercontent.com/stakater/Reloader/v1.4.21/deployments/kubernetes/reloader.yaml"

// installReloader installs stakater/Reloader (BR-PLATFORM-014) into the
// demo cluster and waits for its rollout. Reloader watches Secrets/
// ConfigMaps annotated with "secret.reloader.stakater.com/reload"/
// "configmap.reloader.stakater.com/reload" and rolling-restarts the
// annotated Deployment automatically -- used here so a cert-manager
// renewal of keycloak-tls (ensureKeycloakCertManagerIssuer) picks up
// without an engineer manually restarting Keycloak.
func installReloader(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📦 Installing stakater/Reloader v1.4.21...")

	cmd := exec.CommandContext(ctx, "kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", reloaderManifestURL)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Reloader: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "⏳ Waiting for Reloader rollout...")
	waitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/reloader-reloader",
		"-n", "default",
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("reloader did not become ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Reloader installed and ready")
	return nil
}

// interServiceCAIssuerSecretName is the Secret name (and Issuer name) used
// to chain cert-manager's "ca"-type Issuer to the chart's own inter-service
// CA (loaded from authwebhook-tls) -- distinct from "keycloak-tls", which is
// the leaf Certificate/Secret this Issuer issues.
const interServiceCAIssuerSecretName = "inter-service-ca-issuer"

// ensureKeycloakCertManagerIssuer creates a namespaced cert-manager "ca"
// Issuer -- chained to the chart's own inter-service CA (the same CA every
// other service already trusts via its tls-ca mount) -- plus a Certificate
// requesting "keycloak-tls" from it (BR-PLATFORM-014).
//
// Unlike the ad hoc, one-shot signAndApplyLeafTLSSecret call
// (ensureKeycloakTLSFromChartCA, used by the FMC/Ginkgo suites), cert-manager
// owns "keycloak-tls" going forward and renews it automatically ahead of
// expiry (duration 2160h/90d, renewBefore 720h/30d) -- no code needs to run
// again after this call.
//
// namespace is where authwebhook-tls (and thus the inter-service CA) lives
// (loadChartCAFromAuthwebhookTLS); keycloakNamespace is where the Issuer,
// Certificate, and resulting keycloak-tls Secret are created -- the demo
// entry point's "idp" namespace, distinct from namespace.
func ensureKeycloakCertManagerIssuer(ctx context.Context, kubeconfigPath, namespace, keycloakNamespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔐 Creating cert-manager CA Issuer + Certificate for keycloak-tls (auto-renewing)...")

	caCert, caKey, err := loadChartCAFromAuthwebhookTLS(ctx, kubeconfigPath, namespace)
	if err != nil {
		return err
	}
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return fmt.Errorf("failed to marshal inter-service CA key: %w", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})

	dnsNames := []string{
		"localhost",
		"keycloak",
		fmt.Sprintf("keycloak.%s", namespace),
		fmt.Sprintf("keycloak.%s.svc", namespace),
		fmt.Sprintf("keycloak.%s.svc.cluster.local", namespace),
	}
	if keycloakNamespace != namespace {
		dnsNames = append(dnsNames,
			fmt.Sprintf("keycloak.%s", keycloakNamespace),
			fmt.Sprintf("keycloak.%s.svc", keycloakNamespace),
			fmt.Sprintf("keycloak.%s.svc.cluster.local", keycloakNamespace),
		)
	}
	dnsNamesYAML := ""
	for _, name := range dnsNames {
		dnsNamesYAML += fmt.Sprintf("  - %s\n", name)
	}

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: %[1]s
  namespace: %[2]s
type: kubernetes.io/tls
stringData:
  tls.crt: |
%[3]s
  tls.key: |
%[4]s
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  ca:
    secretName: %[1]s
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: keycloak-tls
  namespace: %[2]s
spec:
  secretName: keycloak-tls
  duration: 2160h
  renewBefore: 720h
  commonName: keycloak
  ipAddresses:
  - 127.0.0.1
  dnsNames:
%[5]s  issuerRef:
    name: %[1]s
    kind: Issuer
    group: cert-manager.io
`, interServiceCAIssuerSecretName, keycloakNamespace, indentPEM(string(caCertPEM)), indentPEM(string(caKeyPEM)), dnsNamesYAML)

	// Issue #1765 (see ApplyCertManagerIssuer, datastorage.go): cert-manager's
	// webhook Service/Endpoints can take a few seconds to propagate after
	// WaitForCertManagerReady's Deployment-level readiness check passes --
	// applying an Issuer/Certificate too early gets a transient "connection
	// refused". Mirror the same 8-attempt exponential-backoff retry.
	const maxRetries = 8
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if applyErr := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); applyErr != nil {
			lastErr = applyErr
			if attempt < maxRetries {
				backoff := time.Duration(attempt) * 3 * time.Second
				_, _ = fmt.Fprintf(writer, "   ⚠️  Issuer/Certificate apply failed (attempt %d/%d, likely webhook endpoint not yet propagated), retrying in %v: %v\n",
					attempt, maxRetries, backoff, applyErr)
				time.Sleep(backoff)
			}
			continue
		}

		if attempt > 1 {
			_, _ = fmt.Fprintf(writer, "   ✅ Issuer/Certificate apply succeeded on attempt %d/%d\n", attempt, maxRetries)
		}
		_, _ = fmt.Fprintln(writer, "  ✅ cert-manager Issuer + Certificate created for keycloak-tls (auto-renewing)")
		return nil
	}

	return fmt.Errorf("failed to create keycloak-tls Issuer/Certificate after %d attempts: %w", maxRetries, lastErr)
}
