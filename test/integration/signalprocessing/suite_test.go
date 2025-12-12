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

// Package signalprocessing_test contains integration tests for the SignalProcessing controller.
// These tests use ENVTEST with real Kubernetes API (etcd + kube-apiserver).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/signalprocessing/)
// - Integration tests (>50%): Infrastructure interaction, microservices coordination (this file)
// - E2E tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/integration/signalprocessing/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package signalprocessing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	"github.com/jordigilh/kubernaut/pkg/audit"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test constants for timeout and polling intervals
const (
	timeout  = 30 * time.Second
	interval = 250 * time.Millisecond
)

// Package-level variables for test environment
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
	auditStore audit.AuditStore // Audit store for BR-SP-090
)

func TestSignalProcessingIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Controller Integration Suite (ENVTEST)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// This follows AIAnalysis/Gateway pattern for automated infrastructure startup
var _ = SynchronizedBeforeSuite(func() []byte {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PROCESS 1 ONLY: Start shared infrastructure (runs ONCE)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("SignalProcessing Integration Test Suite - Automated Setup")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • PostgreSQL (port 15436)")
	GinkgoWriter.Println("  • Redis (port 16382)")
	GinkgoWriter.Println("  • Data Storage API (port 18094)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel = context.WithCancel(context.TODO())

	By("Starting SignalProcessing integration infrastructure (podman-compose)")
	// This starts: PostgreSQL, Redis, DataStorage (with migrations)
	// Per DD-TEST-001: Ports 15435, 16381, 18094
	err := infrastructure.StartSignalProcessingIntegrationInfrastructure(GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All services started and healthy")

	By("Registering SignalProcessing CRD scheme")
	err = signalprocessingv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Registering Remediation CRD scheme for BR-SP-003 tests")
	err = remediationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register additional K8s types needed for integration tests
	err = appsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = autoscalingv2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = policyv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = networkingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating namespaces for testing")
	// Create kubernaut-system namespace for controller
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-system",
		},
	}
	err = k8sClient.Create(ctx, systemNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	By("Creating environment classification ConfigMap")
	createEnvironmentConfigMap()
	GinkgoWriter.Println("✅ Environment ConfigMap created in kubernaut-system namespace")

	// Create audit store (BufferedStore pattern per ADR-038)
	// Uses DataStorage API on port 18094 (per DD-TEST-001)
	GinkgoWriter.Println("📋 Setting up audit store...")
	dsClient := audit.NewHTTPDataStorageClient(
		fmt.Sprintf("http://localhost:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
		nil,
	)
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	logger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "signalprocessing", logger)
	Expect(err).ToNot(HaveOccurred(), "Audit store creation must succeed for BR-SP-090")
	GinkgoWriter.Println("✅ Audit store configured")

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the SignalProcessing controller with audit client")
	// Create audit client for BR-SP-090 compliance
	auditClient := spaudit.NewAuditClient(auditStore, logger)

	By("Creating temporary Rego policy files for classifiers")
	// Day 10 Integration: Create Rego policy files (IMPLEMENTATION_PLAN_V1.31.md)
	// These files match the classifier's expected input schema
	envPolicyFile, err := os.CreateTemp("", "environment-*.rego")
	Expect(err).ToNot(HaveOccurred())
	_, err = envPolicyFile.WriteString(`package signalprocessing.environment

import rego.v1

# BR-SP-051: Namespace label priority (confidence 0.95)
# Timestamps set by Go code (metav1.Time), not Rego
result := {"environment": lower(env), "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# BR-SP-052: ConfigMap fallback (confidence 0.80)
# Use else to prevent conflicts (only ONE result rule can match)
else := {"environment": "production", "confidence": 0.80, "source": "configmap"} if {
    startswith(input.namespace.name, "prod")
}

else := {"environment": "staging", "confidence": 0.80, "source": "configmap"} if {
    startswith(input.namespace.name, "staging")
}

else := {"environment": "development", "confidence": 0.80, "source": "configmap"} if {
    startswith(input.namespace.name, "dev")
}

# BR-SP-053: Default fallback (confidence 0.0)
else := {"environment": "unknown", "confidence": 0.0, "source": "default"}
`)
	Expect(err).ToNot(HaveOccurred())
	envPolicyFile.Close()

	priorityPolicyFile, err := os.CreateTemp("", "priority-*.rego")
	Expect(err).ToNot(HaveOccurred())
	_, err = priorityPolicyFile.WriteString(`package signalprocessing.priority

import rego.v1

# BR-SP-070: Rego-based priority assignment
# BR-SP-071: Severity fallback matrix
# Timestamps set by Go code (metav1.Time), not Rego
# Using else chain to prevent eval_conflict_error
# NOTE: input.environment is a STRING (e.g., "production"), not a struct

# Priority matrix: environment × severity
result := {"priority": "P0", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "production"
    input.signal.severity == "critical"
}

else := {"priority": "P1", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "production"
    input.signal.severity == "warning"
}

else := {"priority": "P1", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "staging"
    input.signal.severity == "critical"
}

else := {"priority": "P2", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "staging"
    input.signal.severity == "warning"
}

else := {"priority": "P2", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "development"
    input.signal.severity == "critical"
}

else := {"priority": "P3", "confidence": 1.0, "source": "policy-matrix"} if {
    input.environment == "development"
    input.signal.severity == "warning"
}

# BR-SP-071: Severity-only fallback
else := {"priority": "P1", "confidence": 0.7, "source": "severity-fallback"} if {
    input.environment == "unknown"
    input.signal.severity == "critical"
}

else := {"priority": "P2", "confidence": 0.7, "source": "severity-fallback"} if {
    input.environment == "unknown"
    input.signal.severity == "warning"
}

# Default
else := {"priority": "P3", "confidence": 0.5, "source": "default"}
`)
	Expect(err).ToNot(HaveOccurred())
	priorityPolicyFile.Close()

	By("Initializing classifiers (Day 10 integration)")
	// Initialize Environment Classifier (BR-SP-051, BR-SP-052, BR-SP-053)
	envClassifier, err := classifier.NewEnvironmentClassifier(
		ctx,
		envPolicyFile.Name(),
		k8sManager.GetClient(),
		logger,
	)
	Expect(err).ToNot(HaveOccurred())

	// Initialize Priority Engine (BR-SP-070, BR-SP-071, BR-SP-072)
	priorityEngine, err := classifier.NewPriorityEngine(
		ctx,
		priorityPolicyFile.Name(),
		logger,
	)
	Expect(err).ToNot(HaveOccurred())

	// Create business policy file for BR-SP-002, BR-SP-080, BR-SP-081
	businessPolicyFile, err := os.CreateTemp("", "business-*.rego")
	Expect(err).ToNot(HaveOccurred())
	_, err = businessPolicyFile.WriteString(`package signalprocessing.business

import rego.v1

# BR-SP-002: Business unit classification
# Simple policy for testing - uses namespace labels
result := {
    "business_unit": input.namespace.labels["kubernaut.ai/business-unit"],
    "service_owner": input.namespace.labels["kubernaut.ai/service-owner"],
    "criticality": input.namespace.labels["kubernaut.ai/criticality"],
    "sla_requirement": input.namespace.labels["kubernaut.ai/sla"],
    "confidence": 0.95,
    "source": "namespace-labels"
}
`)
	Expect(err).ToNot(HaveOccurred())
	businessPolicyFile.Close()

	// Initialize Business Classifier (BR-SP-002, BR-SP-080, BR-SP-081)
	businessClassifier, err := classifier.NewBusinessClassifier(
		ctx,
		businessPolicyFile.Name(),
		logger,
	)
	Expect(err).ToNot(HaveOccurred())

	By("Initializing owner chain builder (Day 7 integration)")
	// Initialize Owner Chain Builder (BR-SP-100)
	ownerChainBuilder := ownerchain.NewBuilder(k8sManager.GetClient(), logger)

	// Create controller with MANDATORY audit client + classifiers + owner chain builder
	err = (&signalprocessing.SignalProcessingReconciler{
		Client:             k8sManager.GetClient(),
		Scheme:             k8sManager.GetScheme(),
		AuditClient:        auditClient,        // BR-SP-090: Audit is MANDATORY
		EnvClassifier:      envClassifier,      // BR-SP-051-053: Environment classification
		PriorityEngine:     priorityEngine,     // BR-SP-070-072: Priority assignment
		BusinessClassifier: businessClassifier, // BR-SP-002, BR-SP-080-081: Business classification
		OwnerChainBuilder:  ownerChainBuilder,  // BR-SP-100: Owner chain traversal
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Schedule cleanup of temp files
	DeferCleanup(func() {
		os.Remove(envPolicyFile.Name())
		os.Remove(priorityPolicyFile.Name())
		os.Remove(businessPolicyFile.Name())
	})

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager to be ready
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ SignalProcessing integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • SignalProcessing CRD installed")
	GinkgoWriter.Println("  • SignalProcessing controller running")
	GinkgoWriter.Println("  • Environment ConfigMap ready")
	GinkgoWriter.Println("  • Audit infrastructure: PostgreSQL:15436, Redis:16382, DataStorage:18094")
	GinkgoWriter.Println("")

	return []byte{} // No data to share across processes
}, func(data []byte) {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ALL PROCESSES: Setup per-process references (runs on EVERY parallel process)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// No per-process setup needed for SP integration tests
	// All processes share the same k8sClient, k8sManager, auditStore created in Process 1
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	cancel()

	// Clean up audit infrastructure (BR-SP-090)
	if auditStore != nil {
		GinkgoWriter.Println("🧹 Closing audit store...")
		err := auditStore.Close()
		Expect(err).ToNot(HaveOccurred())
	}

	// Stop podman-compose stack (PostgreSQL, Redis, DataStorage)
	GinkgoWriter.Println("🧹 Stopping SignalProcessing integration infrastructure...")
	err := infrastructure.StopSignalProcessingIntegrationInfrastructure(GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Cleanup complete")
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// createEnvironmentConfigMap creates the ConfigMap for environment classification
// Used by BR-SP-052: ConfigMap fallback for environment detection
func createEnvironmentConfigMap() {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "signalprocessing-environment-config",
			Namespace: "kubernaut-system",
		},
		Data: map[string]string{
			// Environment mappings for testing (BR-SP-052)
			"environment.rego": `package signalprocessing.environment

import rego.v1

# BR-SP-051: Namespace label priority (confidence 1.0)
environment := env if {
    env := input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
}

# BR-SP-052: ConfigMap fallback (confidence 0.8)
environment := "production" if {
    not input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
    startswith(input.kubernetes.namespace, "prod")
}

environment := "staging" if {
    not input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
    startswith(input.kubernetes.namespace, "staging")
}

environment := "development" if {
    not input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
    startswith(input.kubernetes.namespace, "dev")
}

# BR-SP-053: Default fallback (confidence 0.4)
default environment := "unknown"

confidence := 1.0 if {
    input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
}

confidence := 0.8 if {
    not input.kubernetes.namespaceLabels["kubernaut.ai/environment"]
    environment != "unknown"
}

default confidence := 0.4
`,
			// Priority mappings for testing (BR-SP-070)
			"priority.rego": `package signalprocessing.priority

import rego.v1

# BR-SP-070: Rego-based priority assignment
# BR-SP-071: Severity fallback matrix

# Priority matrix: environment × severity
priority := "P0" if {
    input.environment == "production"
    input.signal.severity == "critical"
}

priority := "P1" if {
    input.environment == "production"
    input.signal.severity == "warning"
}

priority := "P1" if {
    input.environment == "staging"
    input.signal.severity == "critical"
}

priority := "P2" if {
    input.environment == "staging"
    input.signal.severity == "warning"
}

priority := "P2" if {
    input.environment == "development"
    input.signal.severity == "critical"
}

priority := "P3" if {
    input.environment == "development"
    input.signal.severity == "warning"
}

# BR-SP-071: Severity-only fallback
priority := "P1" if {
    input.environment == "unknown"
    input.signal.severity == "critical"
}

priority := "P2" if {
    input.environment == "unknown"
    input.signal.severity == "warning"
}

default priority := "P3"

confidence := 1.0 if {
    input.environment != "unknown"
}

confidence := 0.7 if {
    input.environment == "unknown"
}
`,
		},
	}

	// Delete existing ConfigMap if it exists (idempotent)
	_ = k8sClient.Delete(ctx, configMap)

	// Wait a moment for deletion to complete
	time.Sleep(100 * time.Millisecond)

	// Create new ConfigMap
	err := k8sClient.Create(ctx, configMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to create environment ConfigMap")

	GinkgoWriter.Printf("✅ Environment ConfigMap created with Rego policies\n")
}

// ============================================================================
// TEST HELPER FUNCTIONS (for parallel execution isolation)
// ============================================================================

// createTestNamespace creates a unique namespace for test isolation in parallel execution.
// MANDATORY per 03-testing-strategy.mdc: Each test must use unique identifiers.
func createTestNamespace(prefix string) string {
	ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	GinkgoWriter.Printf("✅ Created test namespace: %s\n", ns)
	return ns
}

// createTestNamespaceWithLabels creates a unique namespace with custom labels.
// Used for testing environment classification from namespace labels (BR-SP-051).
func createTestNamespaceWithLabels(prefix string, labels map[string]string) string {
	ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	GinkgoWriter.Printf("✅ Created test namespace with labels: %s (labels: %v)\n", ns, labels)
	return ns
}

// deleteTestNamespace cleans up a test namespace.
// Called in defer to ensure cleanup even on test failure.
func deleteTestNamespace(ns string) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	err := k8sClient.Delete(ctx, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️ Warning: Failed to delete namespace %s: %v\n", ns, err)
	} else {
		GinkgoWriter.Printf("✅ Deleted test namespace: %s\n", ns)
	}
}

// waitForPhase waits for a SignalProcessing CR to reach a specific phase.
// Returns error if timeout is exceeded.
func waitForPhase(name, namespace string, expectedPhase signalprocessingv1alpha1.SignalProcessingPhase, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		sp := &signalprocessingv1alpha1.SignalProcessing{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sp)
		if err != nil {
			return false, err
		}
		return sp.Status.Phase == expectedPhase, nil
	})
}

// waitForCompletion waits for a SignalProcessing CR to reach Completed phase with CompletionTime set.
func waitForCompletion(name, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		sp := &signalprocessingv1alpha1.SignalProcessing{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sp)
		if err != nil {
			return false, err
		}
		// Check phase AND CompletionTime (controller finished)
		return sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted &&
			sp.Status.CompletionTime != nil, nil
	})
}

// deleteAndWait deletes a SignalProcessing CR and waits for it to be fully removed.
// CRITICAL: Prevents test pollution by ensuring complete cleanup before next test.
func deleteAndWait(sp *signalprocessingv1alpha1.SignalProcessing, timeout time.Duration) error {
	// Delete the CRD
	if err := k8sClient.Delete(ctx, sp); err != nil {
		return err
	}

	// Wait for deletion to complete
	return wait.PollImmediate(100*time.Millisecond, timeout, func() (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      sp.Name,
			Namespace: sp.Namespace,
		}, &signalprocessingv1alpha1.SignalProcessing{})

		if err != nil {
			// Object not found = deletion complete
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			// Other error
			return false, err
		}

		// Still exists, keep waiting
		return false, nil
	})
}

// createTestPod creates a Pod for testing K8s enrichment scenarios.
func createTestPod(namespace, name string, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	return pod
}

// createTestDeployment creates a Deployment for testing K8s enrichment scenarios.
func createTestDeployment(namespace, name string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
	return deployment
}

// createTestPDB creates a PodDisruptionBudget for testing BR-SP-101.
func createTestPDB(namespace, name string, selector map[string]string) *policyv1.PodDisruptionBudget {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
		},
	}
	Expect(k8sClient.Create(ctx, pdb)).To(Succeed())
	return pdb
}

// createTestHPA creates a HorizontalPodAutoscaler for testing BR-SP-101.
func createTestHPA(namespace, name, targetDeployment string) *autoscalingv2.HorizontalPodAutoscaler {
	minReplicas := int32(1)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       targetDeployment,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: 10,
		},
	}
	Expect(k8sClient.Create(ctx, hpa)).To(Succeed())
	return hpa
}

// createTestNetworkPolicy creates a NetworkPolicy for testing BR-SP-101.
func createTestNetworkPolicy(namespace, name string) *networkingv1.NetworkPolicy {
	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}
	Expect(k8sClient.Create(ctx, netpol)).To(Succeed())
	return netpol
}

// createSignalProcessingCR creates a SignalProcessing CR for testing.
// createSignalProcessingCR creates a SignalProcessing CR with proper parent RemediationRequest.
// This follows production architecture: RO creates RR, RR creates SP.
// Per architectural fix: ALL SP CRs MUST have parent RR for correlation_id.
func createSignalProcessingCR(namespace, name string, signal signalprocessingv1alpha1.SignalData) *signalprocessingv1alpha1.SignalProcessing {
	// 1. Create parent RemediationRequest (matches production architecture)
	rr := CreateTestRemediationRequest(
		name+"-rr",
		namespace,
		signal.Fingerprint,
		signal.Severity, // Use severity from signal data
		signal.TargetResource,
	)
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())

	// 2. Create SignalProcessing with parent reference
	sp := CreateTestSignalProcessingWithParent(
		name,
		namespace,
		rr,
		signal.Fingerprint,
		signal.TargetResource,
	)
	Expect(k8sClient.Create(ctx, sp)).To(Succeed())

	return sp
}

