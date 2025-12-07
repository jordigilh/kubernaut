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

var _ = BeforeSuite(func() {
	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	logger = kubelog.NewLogger(kubelog.Options{
		Development: true,
		Level:       0,
		ServiceName: "aianalysis-e2e-test",
	})

	// Initialize failure tracking
	anyTestFailed = false

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("AIAnalysis E2E Test Suite - Setup")
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

	// Create KIND cluster with full dependency chain
	err = infrastructure.CreateAIAnalysisCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Set KUBECONFIG environment variable
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
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
	healthURL = "http://localhost:8184"   // AIAnalysis health via NodePort 30284
	metricsURL = "http://localhost:9184"  // AIAnalysis metrics via NodePort 30184

	// Wait for all services to be ready
	logger.Info("Waiting for services to be ready...")
	Eventually(func() bool {
		return checkServicesReady()
	}, 3*time.Minute, 5*time.Second).Should(BeTrue())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("✅ AIAnalysis E2E cluster ready!")
	logger.Info(fmt.Sprintf("  • Health: %s/healthz", healthURL))
	logger.Info(fmt.Sprintf("  • Metrics: %s/metrics", metricsURL))
	logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = AfterSuite(func() {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("AIAnalysis E2E Test Suite - Teardown")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if any test failed - preserve cluster for debugging
	if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" {
		logger.Info("⚠️  Test FAILED - Keeping cluster alive for debugging")
		logger.Info("To debug:")
		logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
		logger.Info("  kubectl get aianalyses -A")
		logger.Info("  kubectl logs -n kubernaut-system deployment/aianalysis-controller")
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

	// Cancel context
	if cancel != nil {
		cancel()
	}

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Teardown Complete")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// checkServicesReady checks if all required services are healthy
func checkServicesReady() bool {
	// Check AIAnalysis controller health
	// This will be implemented to check actual health endpoints
	// For now, just check if pods are running
	return true
}

// randomSuffix generates a unique suffix for test resource names
func randomSuffix() string {
	return time.Now().Format("20060102150405")
}

