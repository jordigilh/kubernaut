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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// DeployAuthWebhookToCluster deploys the AuthWebhook service for SOC2-compliant CRD operations
// This is the AUTHORITATIVE shared function for all E2E test suites requiring webhook attribution
//
// SOC2 Compliance (CC8.1): User Attribution
//   - Captures authenticated user identity for manual CRD operations
//   - Required for: WorkflowExecution block clearance, RemediationApprovalRequest approvals, NotificationRequest deletions
//
// Authority:
//   - DD-WEBHOOK-001: CRD Webhook Requirements Matrix
//   - DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//   - DD-AUTH-001: Shared Authentication Webhook
//
// Parameters:
//   - ctx: Context for deployment operations
//   - clusterName: Kind cluster name (for image loading)
//   - namespace: K8s namespace for webhook deployment
//   - kubeconfigPath: Isolated kubeconfig for E2E cluster
//   - writer: Output stream for deployment logs
//
// Returns:
//   - error: Deployment failure or nil on success
//
// Deployment Steps:
//   1. Build AuthWebhook image (if not already built)
//   2. Load image to Kind cluster
//   3. Generate webhook TLS certificates
//   4. Apply all CRDs (required for webhook registration)
//   5. Deploy AuthWebhook service + webhook configurations
//   6. Patch webhook configurations with CA bundle
//   7. Wait for webhook pod readiness
//
// Usage:
//
//	// In any E2E suite infrastructure setup:
//	err := infrastructure.DeployAuthWebhookToCluster(ctx, clusterName, namespace, kubeconfigPath, writer)
//	if err != nil {
//	    return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
//	}
func DeployAuthWebhookToCluster(ctx context.Context, clusterName, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🔐 Deploying AuthWebhook for SOC2 User Attribution (CC8.1)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "   Cluster: %s\n", clusterName)
	_, _ = fmt.Fprintf(writer, "   Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "   Authority: DD-WEBHOOK-001, DD-AUTH-001\n")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Get workspace root for config paths
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// STEP 1: Build AuthWebhook image
	_, _ = fmt.Fprintln(writer, "\n📦 STEP 1: Building AuthWebhook image...")
	cfg := E2EImageConfig{
		ServiceName:      "webhooks",
		ImageName:        "webhooks",
		DockerfilePath:   "docker/webhooks.Dockerfile",
		BuildContextPath: "",
		EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
	}
	awImageName, err := BuildImageForKind(cfg, writer)
	if err != nil {
		return fmt.Errorf("AuthWebhook image build failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ AuthWebhook image built: %s\n", awImageName)

	// STEP 2: Load image to Kind cluster
	_, _ = fmt.Fprintln(writer, "\n📦 STEP 2: Loading AuthWebhook image to Kind cluster...")
	err = LoadImageToKind(awImageName, "webhooks", clusterName, writer)
	if err != nil {
		return fmt.Errorf("AuthWebhook image load failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ AuthWebhook image loaded")

	// STEP 3: Generate webhook TLS certificates
	_, _ = fmt.Fprintln(writer, "\n🔐 STEP 3: Generating webhook TLS certificates...")
	if err := generateWebhookCerts(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate webhook certs: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ TLS certificates generated")

	// STEP 4: Apply ALL CRDs (required for webhook registration)
	_, _ = fmt.Fprintln(writer, "\n📋 STEP 4: Applying ALL CRDs...")
	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "config/crd/bases/")
	cmd.Dir = workspaceRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ CRD apply failed: %s\n", output)
		return fmt.Errorf("kubectl apply crds failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ All CRDs applied")

	// STEP 5: Deploy AuthWebhook service + webhook configurations
	_, _ = fmt.Fprintln(writer, "\n🚀 STEP 5: Deploying AuthWebhook service...")
	// Read and substitute namespace in manifest (Go-based replacement for envsubst)
	manifestPath := filepath.Join(workspaceRoot, "test/e2e/authwebhook/manifests/authwebhook-deployment.yaml")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Replace ${WEBHOOK_NAMESPACE} with actual namespace
	substitutedManifest := strings.ReplaceAll(string(manifestContent), "${WEBHOOK_NAMESPACE}", namespace)
	_, _ = fmt.Fprintf(writer, "   🔧 Substituted namespace: ${WEBHOOK_NAMESPACE} → %s\n", namespace)

	// Apply substituted manifest via kubectl
	cmd = exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "-") // Read from stdin
	cmd.Stdin = strings.NewReader(substitutedManifest)
	cmd.Dir = workspaceRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Deployment failed: %s\n", output)
		return fmt.Errorf("kubectl apply authwebhook deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ AuthWebhook deployment applied")

	// STEP 6: Patch deployment with correct image
	_, _ = fmt.Fprintf(writer, "\n🔧 STEP 6: Patching deployment with image: %s\n", awImageName)
	cmd = exec.Command("kubectl", "set", "image",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"deployment/authwebhook",
		fmt.Sprintf("authwebhook=%s", awImageName))
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Image patch failed: %s\n", output)
		return fmt.Errorf("kubectl set image failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ Deployment image patched")

	// STEP 7: Patch webhook configurations with CA bundle
	_, _ = fmt.Fprintln(writer, "\n🔐 STEP 7: Patching webhook configurations with CA bundle...")
	if err := patchWebhookConfigsWithCABundle(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to patch webhook configurations: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ Webhook configurations patched")

	// STEP 8: Wait for webhook pod readiness
	// Per DD-TEST-008: K8s v1.35.0 probe bug workaround
	// kubectl wait relies on kubelet probes (broken in v1.35.0 - prober_manager.go:197 error)
	// Solution: Poll Pod status directly via K8s API (same as AuthWebhook E2E)
	// See: docs/handoff/AUTHWEBHOOK_POD_READINESS_ISSUE_JAN09.md
	_, _ = fmt.Fprintln(writer, "\n⏳ STEP 8: Waiting for AuthWebhook pod readiness...")
	_, _ = fmt.Fprintln(writer, "   ⏱️  Workaround: Polling Pod API directly (K8s v1.35.0 probe bug)")

	// Use direct Pod status polling instead of kubectl wait
	if err := waitForAuthWebhookPodReady(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("AuthWebhook pod did not become ready: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ AuthWebhook pod ready")

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ AuthWebhook Deployment Complete!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "   SOC2 CC8.1: User attribution enabled for CRD operations\n")
	_, _ = fmt.Fprintf(writer, "   Webhooks: WorkflowExecution, RemediationApprovalRequest, NotificationRequest\n")
	_, _ = fmt.Fprintf(writer, "   Operations: STATUS updates (manual), DELETE (cancellations)\n")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// generateWebhookCerts generates TLS certificates for webhook admission
// Uses openssl to create self-signed certificates for E2E testing
// Production deployments should use cert-manager
func generateWebhookCerts(kubeconfigPath, namespace string, writer io.Writer) error {
	// Generate private key
	cmd := exec.Command("openssl", "genrsa", "-out", "/tmp/webhook-key.pem", "2048")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Key generation failed: %s\n", output)
		return fmt.Errorf("openssl genrsa failed: %w", err)
	}

	// Generate certificate with SAN (Subject Alternative Names) for webhook service
	cmd = exec.Command("openssl", "req", "-new", "-x509",
		"-key", "/tmp/webhook-key.pem",
		"-out", "/tmp/webhook-cert.pem",
		"-days", "365",
		"-subj", fmt.Sprintf("/CN=authwebhook.%s.svc", namespace),
		"-addext", fmt.Sprintf("subjectAltName=DNS:authwebhook.%s.svc,DNS:authwebhook.%s.svc.cluster.local", namespace, namespace))
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Cert generation failed: %s\n", output)
		return fmt.Errorf("openssl req failed: %w", err)
	}

	// Create K8s TLS secret directly
	cmd = exec.Command("kubectl", "create", "secret", "tls", "authwebhook-tls",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"--cert=/tmp/webhook-cert.pem",
		"--key=/tmp/webhook-key.pem")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Secret creation failed: %s\n", output)
		return fmt.Errorf("kubectl create secret failed: %w", err)
	}

	return nil
}

// patchWebhookConfigsWithCABundle patches webhook configurations with CA bundle from TLS cert
// Must be called AFTER webhook configurations are created
func patchWebhookConfigsWithCABundle(kubeconfigPath string, writer io.Writer) error {
	// Base64 encode the certificate for CA bundle
	caBundleOutput, err := exec.Command("bash", "-c", "cat /tmp/webhook-cert.pem | base64 | tr -d '\\n'").Output()
	if err != nil {
		return fmt.Errorf("failed to base64 encode CA bundle: %w", err)
	}
	caBundleB64 := string(caBundleOutput)

	// Patch each webhook in MutatingWebhookConfiguration
	webhookNames := []string{"workflowexecution.mutate.kubernaut.ai", "remediationapprovalrequest.mutate.kubernaut.ai"}
	for i, webhookName := range webhookNames {
		patchCmd := exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "authwebhook-mutating",
			"--kubeconfig", kubeconfigPath,
			"--type=json",
			"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/%d/clientConfig/caBundle","value":"%s"}]`, i, caBundleB64))
		if output, err := patchCmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(writer, "   ❌ Failed to patch %s: %s\n", webhookName, output)
			return fmt.Errorf("failed to patch mutating webhook %s: %w", webhookName, err)
		}
	}

	// Patch ValidatingWebhookConfiguration
	patchCmd := exec.Command("kubectl", "patch", "validatingwebhookconfiguration", "authwebhook-validating",
		"--kubeconfig", kubeconfigPath,
		"--type=json",
		"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/0/clientConfig/caBundle","value":"%s"}]`, caBundleB64))
	if output, err := patchCmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Failed to patch validating webhook: %s\n", output)
		return fmt.Errorf("failed to patch validating webhook: %w", err)
	}

	return nil
}

// waitForAuthWebhookPodReady waits for AuthWebhook pod to be ready by polling Pod status directly
// Per DD-TEST-008: Workaround for K8s v1.35.0 prober_manager bug
// kubectl wait relies on kubelet probes which are broken (prober_manager.go:197 error affects ALL pods)
// Solution: Poll Pod.Status.Conditions directly via K8s API (bypasses kubelet probe mechanism)
// See: docs/handoff/AUTHWEBHOOK_POD_READINESS_ISSUE_JAN09.md
func waitForAuthWebhookPodReady(kubeconfigPath, namespace string, writer io.Writer) error {
	ctx := context.Background()

	// Create Kubernetes clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Poll Pod status directly (same pattern as AuthWebhook E2E)
	// Per authwebhook_e2e.go:1093-1110
	timeout := 5 * time.Minute
	pollInterval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// List AuthWebhook pods
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=authwebhook",
		})

		if err == nil && len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					for _, condition := range pod.Status.Conditions {
						if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
							return nil // Pod is ready!
						}
					}
				}
			}
		}

		// Not ready yet, wait before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for AuthWebhook pod to become ready (5 minutes)")
}

