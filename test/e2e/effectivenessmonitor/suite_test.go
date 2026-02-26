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

// Package effectivenessmonitor contains E2E tests for the EffectivenessMonitor controller.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests: Business logic in isolation (test/unit/effectivenessmonitor/)
// - Integration tests: Infrastructure interaction with envtest (test/integration/effectivenessmonitor/)
// - E2E tests: Complete workflow validation with KIND (this package)
//
// Infrastructure:
// - Real Prometheus (metric comparison via OTLP injection)
// - Real AlertManager (alert resolution queries)
// - DataStorage (PostgreSQL + Redis) for audit event verification
//
// CRITICAL: Uses isolated kubeconfig to avoid overwriting ~/.kube/config
// Per TESTING_GUIDELINES.md: kubeconfig at ~/.kube/em-e2e-config
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/e2e/effectivenessmonitor/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package effectivenessmonitor

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

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Test constants
const (
	timeout  = 150 * time.Second
	interval = 500 * time.Millisecond

	clusterName = "em-e2e"

	// controllerNamespace is where the EM controller watches EAs (ADR-057).
	// EAs must be created here; Spec.SignalTarget.Namespace and Spec.RemediationTarget.Namespace
	// point to the test namespace where workload resources (Pods, etc.) live.
	controllerNamespace = "kubernaut-system"
)

// Package-level variables
var (
	ctx    context.Context
	cancel context.CancelFunc

	kubeconfigPath string
	k8sClient      client.Client
	apiReader      client.Reader // Direct API reader to bypass client cache for Eventually() blocks

	// DataStorage audit client for verifying audit events
	auditClient *ogenclient.Client

	// DD-AUTH-014: ServiceAccount token for DataStorage authentication
	e2eAuthToken string

	// Prometheus and AlertManager URLs for test data injection
	prometheusURL  string
	alertManagerURL string

	// Track test failures for cluster cleanup decision
	anyTestFailed bool
)

func TestEffectivenessMonitorE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EffectivenessMonitor Controller E2E Suite (KIND)")
}

var _ = SynchronizedBeforeSuite(
	// Process 1 only: create cluster and deploy all infrastructure
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())

		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-e2e-config", homeDir, clusterName)
		GinkgoWriter.Printf("  Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		By("Setting up EM E2E infrastructure using HYBRID PARALLEL approach (DD-TEST-002)")
		setupCtx := context.Background()
		err = infrastructure.SetupEMInfrastructure(setupCtx, clusterName, tempKubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// DD-AUTH-014: Create E2E ServiceAccount for DataStorage authentication
		By("Creating E2E ServiceAccount for DataStorage audit queries (DD-AUTH-014)")
		e2eSAName := "effectivenessmonitor-e2e-sa"
		namespace := "kubernaut-system"

		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			setupCtx, namespace, tempKubeconfigPath, e2eSAName, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		token, err := infrastructure.GetServiceAccountToken(setupCtx, namespace, e2eSAName, tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")
		By("E2E ServiceAccount token retrieved for authenticated DataStorage access")

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("  E2E test environment ready (Process 1)")
		GinkgoWriter.Printf("    Cluster: %s\n", clusterName)
		GinkgoWriter.Printf("    Kubeconfig: %s\n", tempKubeconfigPath)

		// Return kubeconfig path and auth token to all processes
		return []byte(fmt.Sprintf("%s|%s", tempKubeconfigPath, token))
	},
	// ALL processes: connect to the cluster created by process 1
	func(data []byte) {
		parts := strings.Split(string(data), "|")
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			e2eAuthToken = parts[1]
		}

		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("EM E2E Test Suite - Setup (Process %d)\n", GinkgoParallelProcess())
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("  Kubeconfig: %s\n", kubeconfigPath)

		By("Setting KUBECONFIG environment variable")
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		By("Registering CRD schemes")
		err = eav1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = remediationv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("Creating Kubernetes client from isolated kubeconfig")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		// Direct API reader for Eventually() blocks (bypass cache)
		apiReader, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		By("Setting up authenticated DataStorage audit client")
		// DD-TEST-001: EM E2E uses host port 8091 for DataStorage
		dataStorageURL := fmt.Sprintf("http://localhost:%d", infrastructure.DataStorageEMHostPort())
		saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		auditClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("  Authenticated DataStorage audit client: %s\n", dataStorageURL)

		// Set Prometheus and AlertManager URLs for test data injection
		prometheusURL = fmt.Sprintf("http://127.0.0.1:%d", infrastructure.PrometheusHostPort)
		alertManagerURL = fmt.Sprintf("http://127.0.0.1:%d", infrastructure.AlertManagerHostPort)
		GinkgoWriter.Printf("  Prometheus: %s\n", prometheusURL)
		GinkgoWriter.Printf("  AlertManager: %s\n", alertManagerURL)

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("Setup Complete - Process %d ready\n", GinkgoParallelProcess())
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		infrastructure.MarkTestFailure(clusterName)
	}
})

var _ = SynchronizedAfterSuite(
	// ALL processes: cleanup context
	func() {
		GinkgoWriter.Printf("Process %d - Cleaning up\n", GinkgoParallelProcess())
		if cancel != nil {
			cancel()
		}
	},
	// Process 1 only: cleanup cluster
	func() {
		By("Cleaning up test environment")

		setupFailed := k8sClient == nil
		anyFailure := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			GinkgoWriter.Println("  CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("    To access: export KUBECONFIG=%s\n", kubeconfigPath)
			GinkgoWriter.Printf("    To delete: kind delete cluster --name %s\n", clusterName)
			return
		}

		// DD-TEST-007: Collect E2E binary coverage BEFORE cluster deletion
		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
			if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "effectivenessmonitor",
				ClusterName:    clusterName,
				DeploymentName: "effectivenessmonitor-controller",
				Namespace:      "kubernaut-system",
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("  Failed to collect E2E coverage (non-fatal): %v\n", err)
			}
		}

		By("Deleting KIND cluster")
		if err := infrastructure.DeleteCluster(clusterName, "effectivenessmonitor", anyFailure, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("  Warning: Failed to delete cluster: %v\n", err)
		}

		By("Removing isolated kubeconfig file")
		if kubeconfigPath != "" {
			defaultConfig := os.ExpandEnv("$HOME/.kube/config")
			if kubeconfigPath != defaultConfig {
				_ = os.Remove(kubeconfigPath)
				GinkgoWriter.Printf("  Removed kubeconfig: %s\n", kubeconfigPath)
			}
		}

		By("Cleaning up service images")
		if !infrastructure.IsRunningInCICD() {
			pruneCmd := exec.Command("podman", "image", "prune", "-f")
			_, _ = pruneCmd.CombinedOutput()
		}

		GinkgoWriter.Println("  E2E cleanup complete")
	},
)

// ============================================================================
// Test Namespace Helpers
// ============================================================================

// createTestNamespace creates a managed test namespace and waits for Active.
func createTestNamespace(prefix string) string {
	return helpers.CreateTestNamespaceAndWait(k8sClient, prefix)
}

// deleteTestNamespace cleans up a test namespace and any EAs in the controller
// namespace that target it (ADR-057: EAs live in kubernaut-system, not in test NS).
func deleteTestNamespace(name string) {
	if name == "" {
		return
	}
	// Delete EAs in controller namespace that target this test namespace
	list := &eav1.EffectivenessAssessmentList{}
	if err := k8sClient.List(ctx, list, client.InNamespace(controllerNamespace)); err == nil {
		for i := range list.Items {
			ea := &list.Items[i]
			if ea.Spec.SignalTarget.Namespace == name || ea.Spec.RemediationTarget.Namespace == name {
				_ = k8sClient.Delete(ctx, ea)
			}
		}
	}
	helpers.DeleteTestNamespace(ctx, k8sClient, name)
}
