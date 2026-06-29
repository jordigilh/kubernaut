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

// Package fullpipeline contains the Full Pipeline E2E test suite (Issue #39).
//
// This suite deploys ALL Kubernaut services in a single Kind cluster and validates
// the complete remediation lifecycle end-to-end:
//
//	K8s Event (OOMKill) → Gateway → RO → SP → AA → KA(MockLLM) → WE(Job) → Notification
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
//   - Unit tests (70%+): Business logic in isolation (test/unit/)
//   - Integration tests (>50%): Infrastructure interaction with envtest (test/integration/)
//   - E2E tests (10-15%): Complete workflow validation with KIND (this suite)
//
// CRITICAL: Uses isolated kubeconfig to avoid overwriting ~/.kube/config
// Per TESTING_GUIDELINES.md: kubeconfig at ~/.kube/fullpipeline-e2e-config
//
// Test Execution:
//
//	ginkgo -v ./test/e2e/fullpipeline/...
//
// IMPORTANT: This suite requires significant resources (~6GB RAM).
// Recommended to run in CI/CD with pre-built images (IMAGE_REGISTRY + IMAGE_TAG).
package fullpipeline

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Import ALL CRD types for the full pipeline
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

const (
	timeout  = 10 * time.Minute       // Longer timeout for full pipeline E2E (real LLM needs more time)
	interval = 500 * time.Millisecond // Tighter polling for faster state-transition detection

	clusterName = "fullpipeline-e2e"
	namespace   = "kubernaut-system"
)

var (
	ctx    context.Context
	cancel context.CancelFunc

	kubeconfigPath string

	k8sClient client.Client
	apiReader client.Reader // Direct API reader to bypass client cache

	// DataStorage client for workflow seeding and audit queries
	dataStorageClient *ogenclient.Client

	// DD-AUTH-014: ServiceAccount token for DataStorage authentication
	e2eAuthToken string

	// BR-GATEWAY-036/037: ServiceAccount token for Gateway /api/v1/signals/* authentication.
	// Used by tests that make direct HTTP requests to signal endpoints (e.g., future tests).
	// Infrastructure components (event-exporter, AlertManager) get their token at deploy time.
	fpAuthToken string

	// API Frontend (AF): HTTPS client and DEX OIDC token for AF E2E tests in the FP cluster.
	afBaseURL    string
	afHTTPClient *http.Client
	afAuthToken  string

	// Workflow UUIDs seeded once in SynchronizedBeforeSuite, shared by all tests.
	// Map of "workflowID:environment" → UUID. Tests must NOT modify this or the Mock LLM ConfigMap.
	workflowUUIDs map[string]string

	// Per-test namespace isolation for AF remediate scenarios.
	// Keys match mock-LLM keyword scenario suffixes (e.g., "autonomous", "interactive").
	// Values are UUID-based K8s namespace names generated during infrastructure setup.
	fpRemediateNS map[string]string

	// Track test failures for cluster cleanup decision
	anyTestFailed bool
)

func TestFullPipelineE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Full Pipeline E2E Suite (Issue #39)")
}

var _ = SynchronizedBeforeSuite(
	// Process 1: Create cluster and deploy all services
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
		GinkgoWriter.Printf("📂 Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		By("Setting up Full Pipeline E2E infrastructure (Issue #39)")
		ctx := context.Background()
		images, seededUUIDs, remediateNS, err := infrastructure.SetupFullPipelineInfrastructure(
			ctx, clusterName, tempKubeconfigPath, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Full pipeline infrastructure setup failed")
		_ = images // builtImages stored locally on process 1 for cleanup

		// Validate workflow catalog was seeded (Phase 6b of infrastructure setup)
		Expect(seededUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))
		Expect(seededUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"))
		Expect(seededUUIDs).To(HaveKey("fix-certificate-v1:production"))

		// DD-AUTH-014: Create E2E ServiceAccount for DataStorage authentication
		By("Creating E2E ServiceAccount for DataStorage authentication (DD-AUTH-014)")
		e2eSAName := "fullpipeline-e2e-sa"
		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			ctx, namespace, tempKubeconfigPath, e2eSAName, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")

		// BR-GATEWAY-036/037: Create E2E ServiceAccount for Gateway signal endpoint auth
		// (fullpipeline-gateway-sa is created in SetupFullPipelineInfrastructure for event-exporter/AlertManager)
		By("Creating E2E ServiceAccount for Gateway authentication (BR-GATEWAY-036/037)")
		gatewaySAName := "fullpipeline-gateway-sa"
		gatewayToken, gtwErr := infrastructure.GetServiceAccountToken(ctx, namespace, gatewaySAName, tempKubeconfigPath)
		Expect(gtwErr).ToNot(HaveOccurred(), "Failed to get Gateway SA token (SA created in SetupFullPipelineInfrastructure)")

		By("Labeling kubernaut-system namespace as managed (for AF E2E tests)")
		labelCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
			"label", "namespace", namespace, "kubernaut.ai/managed=true", "--overwrite")
		labelOut, labelErr := labelCmd.CombinedOutput()
		Expect(labelErr).ToNot(HaveOccurred(), "Failed to label namespace: %s", string(labelOut))

		By("Creating per-test remediate namespaces (dynamic, UUID-based)")
		for key, ns := range remediateNS {
			GinkgoWriter.Printf("  Creating namespace %s (scenario: %s)\n", ns, key)
			createNSCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
				"create", "namespace", ns)
			createNSOut, createNSErr := createNSCmd.CombinedOutput()
			if createNSErr != nil && !strings.Contains(string(createNSOut), "already exists") {
				Expect(createNSErr).ToNot(HaveOccurred(), "Failed to create namespace %s: %s", ns, string(createNSOut))
			}
			labelNSCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
				"label", "namespace", ns,
				"kubernaut.ai/managed=true", "kubernaut.ai/environment=staging", "--overwrite")
			labelNSOut, labelNSErr := labelNSCmd.CombinedOutput()
			Expect(labelNSErr).ToNot(HaveOccurred(), "Failed to label namespace %s: %s", ns, string(labelNSOut))
		}

		By("Deploying memory-eater in kubernaut-system for AF E2E tests")
		err = infrastructure.DeployMemoryEater(ctx, namespace, tempKubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater for AF tests")

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("✅ Full Pipeline E2E infrastructure ready (Process 1)")

		// Encode kubeconfig + tokens + workflow UUIDs + remediate namespaces for all processes
		uuidsJSON, jsonErr := json.Marshal(seededUUIDs)
		Expect(jsonErr).ToNot(HaveOccurred())
		remediateNSJSON, nsErr := json.Marshal(remediateNS)
		Expect(nsErr).ToNot(HaveOccurred())
		return []byte(fmt.Sprintf("%s|%s|%s|%s|%s", tempKubeconfigPath, token, string(uuidsJSON), gatewayToken, string(remediateNSJSON)))
	},
	// ALL processes: connect to the cluster
	func(data []byte) {
		parts := strings.SplitN(string(data), "|", 5)
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			e2eAuthToken = parts[1]
		}
		if len(parts) > 2 {
			Expect(json.Unmarshal([]byte(parts[2]), &workflowUUIDs)).To(Succeed(),
				"Failed to decode workflow UUIDs from Process 1")
		}
		if len(parts) > 3 {
			fpAuthToken = parts[3]
		}
		if len(parts) > 4 {
			Expect(json.Unmarshal([]byte(parts[4]), &fpRemediateNS)).To(Succeed(),
				"Failed to decode remediate namespaces from Process 1")
		}

		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("Full Pipeline E2E - Setup (Process %d)\n", GinkgoParallelProcess())
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		By("Setting KUBECONFIG")
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Issue #785: Configure http.DefaultTransport to trust the inter-service CA.
		tlsTransport, tlsTErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsTErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		By("Registering ALL CRD schemes")
		Expect(remediationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme.Scheme)).To(Succeed())          // ADR-EM-001: EA types
		Expect(isv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed()) // BR-INTERACTIVE-010: IS CRD

		By("Creating Kubernetes client")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		apiReader, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		By("Setting up authenticated DataStorage client (DD-TEST-001: port 30081, Issue #785: HTTPS)")
		dataStorageURL := "https://localhost:30081"
		tlsBase, tlsCErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsCErr).ToNot(HaveOccurred(), "TLS transport for DataStorage client")
		saTransport := testauth.NewServiceAccountTransportWithBase(e2eAuthToken, tlsBase)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		dataStorageClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred())

		By("Setting up API Frontend HTTP client (NodePort 30443, self-signed TLS)")
		afBaseURL = "https://localhost:30443"
		afHTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // E2E self-signed cert
			},
			Timeout: 30 * time.Second,
		}

		GinkgoWriter.Printf("✅ Setup Complete - Process %d ready\n", GinkgoParallelProcess())
	},
)

// getAFToken obtains an OIDC token from DEX for API Frontend authentication (password grant).
func getAFToken() string {
	if afAuthToken != "" {
		return afAuthToken
	}
	tlsClient, tlsErr := infrastructure.NewTLSAwareClient(kubeconfigPath, 10*time.Second)
	Expect(tlsErr).NotTo(HaveOccurred(), "TLS client for Dex token endpoint")

	resp, err := tlsClient.PostForm("https://localhost:30556/dex/token", url.Values{
		"grant_type":    {"password"},
		"client_id":     {"kubernaut-apifrontend"},
		"client_secret": {"e2e-client-secret"},
		"username":      {"sre@kubernaut.ai"},
		"password":      {"password"},
		"scope":         {"openid email profile groups"},
	})
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	afAuthToken = tokenResp.AccessToken
	return afAuthToken
}

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		infrastructure.MarkTestFailure(clusterName)
	}
})

var _ = SynchronizedAfterSuite(
	// ALL processes: cleanup context
	func() {
		if cancel != nil {
			cancel()
		}
	},
	// Process 1 only: cleanup cluster
	func() {
		By("Cleaning up Full Pipeline E2E environment")

		setupFailed := k8sClient == nil
		anyFailure := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			GinkgoWriter.Println("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", kubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
			return
		}

		// Collect must-gather BEFORE coverage collection.
		// Coverage collection scales down controllers, which terminates pods and loses their logs.
		if anyFailure && !setupFailed {
			homeDir, _ := os.UserHomeDir()
			kp := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-system", "fullpipeline", GinkgoWriter)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-workflows", "fullpipeline", GinkgoWriter)
		}

		// Clean up all test-created CRs and namespaces so the cluster
		// is ready for a retry run. Must-gather has already collected logs.
		if !setupFailed {
			infrastructure.CleanupFullPipelineTestResources(kubeconfigPath, GinkgoWriter)
		}

		// DD-TEST-007: Collect coverage before cluster deletion
		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
			// Collect coverage for each Go controller service
			for _, svc := range []struct{ service, deployment string }{
				{"signalprocessing", "signalprocessing-controller"},
				{"remediationorchestrator", "remediationorchestrator-controller"},
				{"aianalysis", "aianalysis-controller"},
				{"workflowexecution", "workflowexecution-controller"},
				{"notification", "notification-controller"},
				{"effectivenessmonitor", "effectivenessmonitor-controller"},
			} {
				if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
					ServiceName:    svc.service,
					ClusterName:    clusterName,
					DeploymentName: svc.deployment,
					Namespace:      namespace,
					KubeconfigPath: kubeconfigPath,
				}, GinkgoWriter); err != nil {
					GinkgoWriter.Printf("⚠️  Coverage collection failed for %s (non-fatal): %v\n", svc.service, err)
				}
			}
		}

		By("Deleting KIND cluster (preserves on failure in CI for must-gather)")
		if err := infrastructure.DeleteCluster(clusterName, "fullpipeline", anyFailure, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("⚠️  Warning: Failed to delete cluster: %v\n", err)
		}

		By("Removing isolated kubeconfig file")
		if kubeconfigPath != "" {
			defaultConfig := os.ExpandEnv("$HOME/.kube/config")
			if kubeconfigPath != defaultConfig {
				_ = os.Remove(kubeconfigPath)
				GinkgoWriter.Printf("🗑️  Removed kubeconfig: %s\n", kubeconfigPath)
			}
		}

		By("Cleaning up built images")
		if !infrastructure.IsRunningInCICD() {
			pruneCmd := exec.Command("podman", "image", "prune", "-f")
			_, _ = pruneCmd.CombinedOutput()
		}

		GinkgoWriter.Println("✅ Full Pipeline E2E cleanup complete")
	},
)
