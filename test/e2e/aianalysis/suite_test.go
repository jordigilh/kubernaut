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

// Package aianalysis contains E2E tests for the AIAnalysis controller.
// These tests run against a real KIND cluster with deployed services.
//
// Business Requirements:
// - BR-AI-001: Complete reconciliation lifecycle
// - BR-AI-022: Metrics endpoint validation
// - BR-AI-025: Health endpoint validation
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - E2E tests use KIND cluster with real services
// - LLM is mocked in HolmesGPT-API (cost constraint)
// - Data Storage used for audit trails
//
// Port Allocation (per DD-TEST-001):
// - AIAnalysis Health: http://localhost:8184
// - AIAnalysis Metrics: http://localhost:9184
// - Data Storage: http://localhost:8081
// - HolmesGPT-API: http://localhost:8088
package aianalysis

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestAIAnalysisE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis E2E Test Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration
	clusterName    string
	kubeconfigPath string

	// Kubernetes client
	k8sClient client.Client

	// Service URLs (per DD-TEST-001)
	healthURL  string
	metricsURL string

	// Track failures for cleanup decision
	anyTestFailed bool
)

var _ = SynchronizedBeforeSuite(
	// This runs on process 1 only - create cluster once
	func() []byte {
		// Initialize logger for process 1
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: "aianalysis-e2e-test",
		})

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("AIAnalysis E2E Test Suite - Setup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Setting up KIND cluster with full dependency chain:")
		logger.Info("  • PostgreSQL + Redis (Data Storage dependencies)")
		logger.Info("  • Data Storage (audit trails)")
		logger.Info("  • HolmesGPT-API (AI analysis with mock LLM)")
		logger.Info("  • AIAnalysis controller")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "aianalysis-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		kubeconfigPath = fmt.Sprintf("%s/.kube/aianalysis-e2e-config", homeDir)

		// Create KIND cluster with full dependency chain (ONCE for all processes)
		logger.Info("Creating Kind cluster (this runs once)...")
		err = infrastructure.CreateAIAnalysisCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		logger.Info("✅ Cluster created successfully")
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info("  • Process 1 will now share kubeconfig with other processes")

		// Return kubeconfig path to all processes
		return []byte(kubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(data []byte) {
		kubeconfigPath = string(data)

		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: fmt.Sprintf("aianalysis-e2e-test-p%d", GinkgoParallelProcess()),
		})

		// Initialize failure tracking
		anyTestFailed = false

		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("AIAnalysis E2E Test Suite - Setup (Process %d)", GinkgoParallelProcess()))
		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("Connecting to cluster created by process 1"))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))

		// Set cluster name
		clusterName = "aianalysis-e2e"

		// Set KUBECONFIG environment variable
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Register AIAnalysis scheme
		err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).ToNot(HaveOccurred())

		// Create Kubernetes client
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		// Set service URLs (per DD-TEST-001 port allocation)
		// AIAnalysis ports: API=8084/30084, Metrics=9184/30184, Health=8184/30284
		healthURL = "http://localhost:8184"  // AIAnalysis health via NodePort 30284
		metricsURL = "http://localhost:9184" // AIAnalysis metrics via NodePort 30184

		// Wait for all services to be ready
		logger.Info("Waiting for services to be ready...")
		Eventually(func() bool {
			return checkServicesReady()
		}, 3*time.Minute, 5*time.Second).Should(BeTrue())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("✅ Process %d ready!", GinkgoParallelProcess()))
		logger.Info(fmt.Sprintf("  • Health: %s/healthz", healthURL))
		logger.Info(fmt.Sprintf("  • Metrics: %s/metrics", metricsURL))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - cleanup context
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("Process %d - Cleaning up", GinkgoParallelProcess()))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Cancel context for this process
		if cancel != nil {
			cancel()
		}
	},
	// This runs on process 1 only - delete cluster
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("AIAnalysis E2E Test Suite - Teardown (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if any test failed - preserve cluster for debugging
		if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" || os.Getenv("KEEP_CLUSTER") != "" {
			logger.Info("⚠️  Keeping cluster alive for debugging")
			logger.Info("Reason:")
			if anyTestFailed {
				logger.Info("  • At least one test failed")
			}
			if os.Getenv("SKIP_CLEANUP") == "true" {
				logger.Info("  • SKIP_CLEANUP=true")
			}
			if os.Getenv("KEEP_CLUSTER") != "" {
				logger.Info("  • KEEP_CLUSTER set")
			}
			logger.Info("")
			logger.Info("To debug:")
			logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			logger.Info("  kubectl get aianalyses -A")
			logger.Info("  kubectl logs -n kubernaut-system deployment/aianalysis-controller")
			logger.Info("")
			logger.Info("To cleanup manually:")
			logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// All tests passed - cleanup cluster
		logger.Info("✅ All tests passed - cleaning up cluster...")
		err := infrastructure.DeleteAIAnalysisCluster(clusterName, kubeconfigPath, GinkgoWriter)
		if err != nil {
			logger.Error(err, "Failed to delete cluster")
		}

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Teardown Complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// checkServicesReady checks if all required services are healthy
func checkServicesReady() bool {
	// Check AIAnalysis controller health endpoint
	healthResp, err := http.Get(healthURL + "/healthz")
	if err != nil || healthResp.StatusCode != 200 {
		return false
	}
	defer healthResp.Body.Close()

	// Check metrics endpoint
	metricsResp, err := http.Get(metricsURL + "/metrics")
	if err != nil || metricsResp.StatusCode != 200 {
		return false
	}
	defer metricsResp.Body.Close()

	return true
}

// randomSuffix generates a unique suffix for test resource names
// Uses nanosecond precision to avoid collisions in parallel test execution
func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
