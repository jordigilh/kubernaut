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
//	K8s Event (OOMKill) → Gateway → RO → SP → AA → HAPI(MockLLM) → WE(Job) → Notification
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
	"fmt"
	"net/http"
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
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

const (
	timeout  = 10 * time.Minute // Longer timeout for full pipeline E2E (real LLM needs more time)
	interval = 1 * time.Second

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

	// Track built images for cleanup
	builtImages map[string]string

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
		images, err := infrastructure.SetupFullPipelineInfrastructure(
			ctx, clusterName, tempKubeconfigPath, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Full pipeline infrastructure setup failed")
		_ = images // builtImages stored locally on process 1 for cleanup

		// DD-AUTH-014: Create E2E ServiceAccount for DataStorage authentication
		By("Creating E2E ServiceAccount for DataStorage authentication (DD-AUTH-014)")
		e2eSAName := "fullpipeline-e2e-sa"
		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			ctx, namespace, tempKubeconfigPath, e2eSAName, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("✅ Full Pipeline E2E infrastructure ready (Process 1)")

		// Return kubeconfig path and auth token to all processes
		return []byte(fmt.Sprintf("%s|%s", tempKubeconfigPath, token))
	},
	// ALL processes: connect to the cluster
	func(data []byte) {
		parts := strings.Split(string(data), "|")
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			e2eAuthToken = parts[1]
		}

		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("Full Pipeline E2E - Setup (Process %d)\n", GinkgoParallelProcess())
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		By("Setting KUBECONFIG")
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		By("Registering ALL CRD schemes")
		Expect(remediationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme.Scheme)).To(Succeed()) // ADR-EM-001: EA types

		By("Creating Kubernetes client")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		apiReader, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		By("Setting up authenticated DataStorage client (DD-TEST-001: port 30081)")
		dataStorageURL := "http://localhost:30081"
		saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		dataStorageClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("✅ Setup Complete - Process %d ready\n", GinkgoParallelProcess())
	},
)

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
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
		anyFailure := setupFailed || anyTestFailed
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
