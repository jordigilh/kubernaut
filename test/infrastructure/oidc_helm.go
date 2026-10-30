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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	demoConsoleOIDCSecretName   = "kubernaut-console-oidc"
	demoConsoleOIDCClientID     = "kubernaut-console"
	demoConsoleOIDCClientSecret = "e2e-console-secret"
	demoConsoleOIDCCookieSecret = "01234567890123456789012345678901"
)

// SetupOIDCInfrastructure provisions the shared browser-authentication
// dependencies used by local and fleet demo installations. Fleet-only MCP
// Gateway, spoke, and API-server OIDC wiring remain outside this helper.
func SetupOIDCInfrastructure(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	appNamespace := kubernautSystem
	if err := provisionInterServiceCA(ctx, kubeconfigPath, appNamespace, writer); err != nil {
		return fmt.Errorf("inter-service CA provisioning failed: %w", err)
	}
	if err := CreateTestNamespace(ctx, idpNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create %s namespace for Keycloak: %w", idpNamespace, err)
	}
	if err := InstallCertManager(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("cert-manager installation failed: %w", err)
	}
	if err := WaitForCertManagerReady(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("cert-manager readiness check failed: %w", err)
	}
	if err := installReloader(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("reloader installation failed: %w", err)
	}
	if err := ensureKeycloakCertManagerIssuer(ctx, kubeconfigPath, appNamespace, idpNamespace, writer); err != nil {
		return fmt.Errorf("keycloak TLS provisioning failed: %w", err)
	}
	if err := ensureDemoConsoleTLS(ctx, kubeconfigPath, appNamespace, writer); err != nil {
		return fmt.Errorf("console TLS provisioning failed: %w", err)
	}
	if err := deployOIDCKeycloak(ctx, kubeconfigPath, appNamespace, writer); err != nil {
		return err
	}
	if err := ensureDemoConsoleOIDCSecret(ctx, kubeconfigPath, appNamespace, writer); err != nil {
		return fmt.Errorf("console OIDC Secret provisioning failed: %w", err)
	}
	if err := deployTraefikForKind(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("traefik installation failed: %w", err)
	}
	return nil
}

// BuildDemoConsoleOIDCSecretManifest creates the demo-owned oauth2-proxy
// credentials Secret. The values match the kubernaut-console client imported
// by keycloak-realm-demo.json; stringData keeps the fixture out of Helm
// arguments while letting the Kubernetes API perform Secret encoding.
func BuildDemoConsoleOIDCSecretManifest(namespace string) (string, error) {
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      demoConsoleOIDCSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "setup-demo-infra",
				"kubernaut.ai/demo-owner":      "setup-demo-infra",
				"kubernaut.ai/e2e-owner":       operatorOwnerLabel,
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"client-id":     demoConsoleOIDCClientID,
			"client-secret": demoConsoleOIDCClientSecret,
			"cookie-secret": demoConsoleOIDCCookieSecret,
		},
	}
	manifest, err := json.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("marshal demo Console OIDC Secret: %w", err)
	}
	return string(manifest), nil
}

func ensureDemoConsoleOIDCSecret(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	manifest, err := BuildDemoConsoleOIDCSecretManifest(namespace)
	if err != nil {
		return err
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("apply %s Secret: %w", demoConsoleOIDCSecretName, err)
	}
	return nil
}

func deployOIDCKeycloak(ctx context.Context, kubeconfigPath, appNamespace string, writer io.Writer) error {
	getDeployment := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "deployment/keycloak", "-n", idpNamespace, "--ignore-not-found", "-o", "name")
	deploymentName, err := getDeployment.Output()
	if err != nil {
		return fmt.Errorf("failed to inspect existing Keycloak deployment: %w", err)
	}
	if strings.TrimSpace(string(deploymentName)) != "" {
		if err := runKubectl(ctx, kubeconfigPath, writer, "scale", "deployment/keycloak", "-n", idpNamespace, "--replicas=0"); err != nil {
			return fmt.Errorf("failed to scale Keycloak down before certificate rotation: %w", err)
		}
		if err := runKubectl(ctx, kubeconfigPath, writer, "wait", "--for=delete", "pod", "-l", "app=keycloak", "-n", idpNamespace, "--timeout=120s"); err != nil {
			return fmt.Errorf("failed to stop Keycloak before certificate rotation: %w", err)
		}
	}
	if err := DeployKeycloakInfra(ctx, idpNamespace, kubeconfigPath, DemoHostPort(keycloakHostPortDemo), true, writer); err != nil {
		return fmt.Errorf("keycloak deployment failed: %w", err)
	}
	aliasManifest := buildKeycloakServiceAliasManifest(appNamespace, idpNamespace)
	if aliasManifest == "" {
		return nil
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, aliasManifest); err != nil {
		return fmt.Errorf("keycloak service alias creation failed: %w", err)
	}
	return nil
}

// ensureDemoConsoleTLS copies the demo CA issuer into the application
// namespace and requests a browser-facing certificate with the Console host
// in its SAN. Traefik's generated default certificate cannot satisfy browser
// hostname validation for kubernaut-console.local.
func ensureDemoConsoleTLS(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	getSecret := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "secret", interServiceCAIssuerSecretName, "-n", idpNamespace, "-o", "json")
	secretJSON, err := getSecret.Output()
	if err != nil {
		return fmt.Errorf("failed to read demo CA issuer Secret: %w", err)
	}
	var caSecret corev1.Secret
	if err := json.Unmarshal(secretJSON, &caSecret); err != nil {
		return fmt.Errorf("failed to decode demo CA issuer Secret: %w", err)
	}
	caCert, okCert := caSecret.Data["tls.crt"]
	caKey, okKey := caSecret.Data["tls.key"]
	if !okCert || !okKey {
		return fmt.Errorf("demo CA issuer Secret is missing tls.crt or tls.key")
	}

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: demo-console-ca
  namespace: %s
spec:
  ca:
    secretName: %s
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: console-tls
  namespace: %s
spec:
  secretName: console-tls
  duration: 2160h
  renewBefore: 720h
  commonName: kubernaut-console.local
  dnsNames:
    - kubernaut-console.local
  issuerRef:
    name: demo-console-ca
    kind: Issuer
    group: cert-manager.io
`, interServiceCAIssuerSecretName, namespace,
		base64.StdEncoding.EncodeToString(caCert), base64.StdEncoding.EncodeToString(caKey),
		namespace, interServiceCAIssuerSecretName, namespace)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("failed to apply Console TLS resources: %w", err)
	}
	if err := runKubectl(ctx, kubeconfigPath, writer, "wait", "--for=condition=Ready", "certificate/console-tls", "-n", namespace, "--timeout=120s"); err != nil {
		return fmt.Errorf("console TLS certificate did not become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Console TLS ready; trust CA at %s\n", InterServiceCAPath(kubeconfigPath))
	return nil
}

// SetupDemoMonitoringInfrastructure deploys the local operator-managed
// Prometheus and AlertManager pair used by Console investigation and
// autonomous remediation.
func SetupDemoMonitoringInfrastructure(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	if err := CreateTestNamespace(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create %s namespace: %w", monitoringNamespace, err)
	}
	if err := EnsureMonitoringTLS(ctx, kubeconfigPath, kubernautSystem, monitoringNamespace, writer); err != nil {
		return fmt.Errorf("failed to prepare monitoring TLS: %w", err)
	}
	// AlertManager mounts inter-service-ca from its own namespace. OIDC setup
	// creates the shared CA in kubernaut-system, so replicate the trust bundle
	// before applying the monitoring workload (Issue #2405).
	if err := ReplicateInterServiceCAConfigMap(ctx, kubeconfigPath, monitoringNamespace, writer); err != nil {
		return fmt.Errorf("failed to replicate inter-service CA to %s: %w", monitoringNamespace, err)
	}
	const serviceAccountName = "demo-alertmanager-gateway"
	if err := CreateE2EServiceAccountWithGatewayAccess(ctx, kubernautSystem, kubeconfigPath, serviceAccountName, writer); err != nil {
		return fmt.Errorf("failed to create AlertManager Gateway ServiceAccount: %w", err)
	}
	token, err := GetServiceAccountToken(ctx, kubernautSystem, serviceAccountName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get AlertManager Gateway token: %w", err)
	}
	if err := InstallPrometheusOperator(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("prometheus operator installation failed: %w", err)
	}
	if err := DeployKubeStateMetrics(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("kube-state-metrics deployment failed: %w", err)
	}
	if err := DeployAlertManagerTLS(ctx, monitoringNamespace, kubernautSystem, kubeconfigPath, token, writer); err != nil {
		return fmt.Errorf("alertmanager deployment failed: %w", err)
	}
	alertManagerTarget := fmt.Sprintf("alertmanager-svc.%s.svc.cluster.local:9093", monitoringNamespace)
	if err := DeployLocalManagedPrometheusTLS(ctx, monitoringNamespace, kubeconfigPath, "local", alertManagerTarget, writer); err != nil {
		return fmt.Errorf("managed Prometheus deployment failed: %w", err)
	}
	return nil
}
