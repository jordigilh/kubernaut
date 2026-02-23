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
	"os/exec"
	"path/filepath"
	"sync"
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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/integration"
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
	auditStore audit.AuditStore                 // Audit store for BR-SP-090 (write operations)
	dsClient   *ogenclient.Client               // DataStorage HTTP API client (query operations - correct service boundary)
	dsInfra    *infrastructure.DSBootstrapInfra // DataStorage infrastructure (for must-gather diagnostics)
	// metricsAddr removed - not needed since metrics server uses dynamic port (BindAddress: "0")

	// BR-SP-072: Policy file paths for hot-reload testing
	labelsPolicyFilePath string     // CustomLabels Rego policy file path
	policyFileWriteMu    sync.Mutex // Protects policy file writes
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

	ctx, cancel = context.WithCancel(context.TODO())

	// DD-TEST-001 v1.1: Clean up stale containers from previous runs
	By("Cleaning up stale containers from previous runs")
	testDir, err := filepath.Abs(filepath.Join(".", "..", "..", ".."))
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", err)
	} else {
		// Use correct compose file name per DD-TEST-001
		cleanupCmd := exec.Command("podman-compose", "-f", "podman-compose.signalprocessing.test.yml", "down")
		cleanupCmd.Dir = filepath.Join(testDir, "test", "integration", "signalprocessing")
		_, cleanupErr := cleanupCmd.CombinedOutput()
		if cleanupErr != nil {
			GinkgoWriter.Printf("⚠️  Cleanup of stale containers failed (may not exist): %v\n", cleanupErr)
		} else {
			GinkgoWriter.Println("✅ Stale containers cleaned up")
		}
	}

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("SignalProcessing Integration Test Suite - Automated Setup")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • PostgreSQL (port 15436)")
	GinkgoWriter.Println("  • Redis (port 16382)")
	GinkgoWriter.Println("  • Data Storage API (port 18094)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-AUTH-014: Create envtest FIRST for ServiceAccount authentication
	By("Creating envtest for DataStorage authentication (DD-AUTH-014)")
	sharedTestEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	sharedK8sConfig, err := sharedTestEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start successfully")
	Expect(sharedK8sConfig).ToNot(BeNil(), "K8s config should not be nil")
	GinkgoWriter.Printf("✅ envtest started: %s\n", sharedK8sConfig.Host)

	// Write kubeconfig to temporary file for DataStorage container
	kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "signalprocessing-integration")
	Expect(err).ToNot(HaveOccurred(), "Failed to write envtest kubeconfig")
	GinkgoWriter.Printf("✅ envtest kubeconfig written: %s\n", kubeconfigPath)

	// DD-AUTH-014: Create ServiceAccount with DataStorage access
	GinkgoWriter.Println("🔐 Creating ServiceAccount for DataStorage authentication...")
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedK8sConfig,
		"signalprocessing-integration-sa",
		"default",
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create ServiceAccount")
	GinkgoWriter.Println("✅ ServiceAccount created with Bearer token")

	By("Starting SignalProcessing integration infrastructure (DD-TEST-002)")
	// This starts: PostgreSQL, Redis, DataStorage (with migrations)
	// Per DD-TEST-001 v2.6: PostgreSQL=15436, Redis=16382, DS=18094
	// DD-AUTH-014: Helper function ensures auth is properly configured
	cfg := infrastructure.NewDSBootstrapConfigWithAuth(
		"signalprocessing",
		15436, 16382, 18094, 19094,
		"test/integration/signalprocessing/config",
		authConfig,
	)
	dsInfra, err = infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	dsInfra.SharedTestEnv = sharedTestEnv // Store for cleanup
	GinkgoWriter.Println("✅ All services started and healthy (PostgreSQL, Redis, DataStorage)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		// DD-AUTH-014: Stop shared envtest
		if dsInfra != nil && dsInfra.SharedTestEnv != nil {
			if sharedEnv, ok := dsInfra.SharedTestEnv.(*envtest.Environment); ok {
				if err := sharedEnv.Stop(); err != nil {
					GinkgoWriter.Printf("⚠️  Failed to stop shared envtest: %v\n", err)
				} else {
					GinkgoWriter.Println("✅ Shared envtest stopped")
				}
			}
		}
	})

	// SP-BUG-006: Capture infrastructure state for diagnostics
	By("Verifying infrastructure container status")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("📋 Infrastructure Status Verification")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check PostgreSQL container
	psqlStatus := exec.Command("podman", "ps", "-a", "--filter", "name=signalprocessing_postgres", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}")
	if psqlOut, err := psqlStatus.CombinedOutput(); err == nil {
		GinkgoWriter.Printf("🐘 PostgreSQL: %s\n", string(psqlOut))
	} else {
		GinkgoWriter.Printf("⚠️  PostgreSQL: Failed to check status: %v\n", err)
	}

	// Check Redis container
	redisStatus := exec.Command("podman", "ps", "-a", "--filter", "name=signalprocessing_redis", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}")
	if redisOut, err := redisStatus.CombinedOutput(); err == nil {
		GinkgoWriter.Printf("🔴 Redis: %s\n", string(redisOut))
	} else {
		GinkgoWriter.Printf("⚠️  Redis: Failed to check status: %v\n", err)
	}

	// Check Data Storage container
	dsStatus := exec.Command("podman", "ps", "-a", "--filter", "name=signalprocessing_datastorage", "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}")
	if dsOut, err := dsStatus.CombinedOutput(); err == nil {
		GinkgoWriter.Printf("💾 Data Storage: %s\n", string(dsOut))
	} else {
		GinkgoWriter.Printf("⚠️  Data Storage: Failed to check status: %v\n", err)
	}

	// Check Migrations container (should be exited/completed)
	migrationsStatus := exec.Command("podman", "ps", "-a", "--filter", "name=signalprocessing_migrations", "--format", "{{.Names}}\t{{.Status}}\t{{.ExitCode}}")
	if migrationsOut, err := migrationsStatus.CombinedOutput(); err == nil {
		GinkgoWriter.Printf("🔧 Migrations: %s\n", string(migrationsOut))
	} else {
		GinkgoWriter.Printf("⚠️  Migrations: Failed to check status: %v\n", err)
	}

	// Check Data Storage health endpoint
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("🏥 Data Storage Health Check")
	healthCheck := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://127.0.0.1:18094/health")
	if healthOut, err := healthCheck.CombinedOutput(); err == nil {
		statusCode := string(healthOut)
		if statusCode == "200" {
			GinkgoWriter.Printf("✅ Data Storage health: HTTP %s (ready)\n", statusCode)
		} else {
			GinkgoWriter.Printf("⚠️  Data Storage health: HTTP %s (not ready)\n", statusCode)
		}
	} else {
		GinkgoWriter.Printf("❌ Data Storage health check failed: %v\n", err)
	}

	// Check Data Storage version endpoint
	versionCheck := exec.Command("curl", "-s", "http://127.0.0.1:18094/version")
	if versionOut, err := versionCheck.CombinedOutput(); err == nil {
		GinkgoWriter.Printf("📌 Data Storage version: %s\n", string(versionOut))
	} else {
		GinkgoWriter.Printf("⚠️  Data Storage version check failed: %v\n", err)
	}

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ Phase 1 Complete: Shared infrastructure ready")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-AUTH-014: Pass ServiceAccount token to all processes
	// Note: DataStorage health check now includes auth readiness validation
	// StartDSBootstrap waits for /health to return 200, which includes auth middleware check
	return []byte(authConfig.Token)
}, func(data []byte) {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: ALL PROCESSES (DD-TEST-010 Multi-Controller Pattern)
	// Each process creates: envtest + k8sManager + controller + all dependencies
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Create per-process context
	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering SignalProcessing CRD scheme")
	err := signalprocessingv1alpha1.AddToScheme(scheme.Scheme)
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

	By("Bootstrapping per-process test environment (envtest)")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// DD-AUTH-014: Create authenticated DataStorage clients (both audit + query)
	// Uses centralized helper to ensure both clients use ServiceAccount authentication
	saToken := string(data) // Extract token from Phase 1
	dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort)

	dsClients := integration.NewAuthenticatedDataStorageClients(
		dataStorageURL,
		saToken,
		5*time.Second,
	)
	GinkgoWriter.Println("✅ Authenticated DataStorage clients created (audit + query)")

	// SP-CACHE-001: Create audit store per-process (uses shared DataStorage infrastructure)
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	logger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsClients.AuditClient, auditConfig, "signalprocessing", logger)
	Expect(err).NotTo(HaveOccurred(), "Audit store creation must succeed for BR-SP-090")
	GinkgoWriter.Println("✅ Per-process audit store configured")

	// Use authenticated OpenAPI client for audit event queries in tests
	// DD-AUTH-014: Query client now uses ServiceAccount Bearer token
	dsClient = dsClients.OpenAPIClient
	GinkgoWriter.Printf("✅ Authenticated DataStorage query client ready for tests\n")

	By("Setting up the per-process controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // DD-TEST-010: Random port per process to avoid conflicts
		},
	})
	Expect(err).NotTo(HaveOccurred())

	By("Setting up the SignalProcessing controller with audit client and manager")
	// Create audit client for BR-SP-090 compliance (legacy)
	auditClient := spaudit.NewAuditClient(auditStore, logger)

	// Create audit manager (Phase 3 refactoring - 2026-01-22)
	// ADR-032: AuditManager is MANDATORY for all audit operations
	auditManager := spaudit.NewManager(auditClient)

	By("Creating temporary Rego policy files for classifiers")
	// Day 10 Integration: Create Rego policy files (IMPLEMENTATION_PLAN_V1.31.md)
	// These files match the classifier's expected input schema
	envPolicyFile, err := os.CreateTemp("", "environment-*.rego")
	Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())
	_ = envPolicyFile.Close()

	priorityPolicyFile, err := os.CreateTemp("", "priority-*.rego")
	Expect(err).NotTo(HaveOccurred())
	_, err = priorityPolicyFile.WriteString(`package signalprocessing.priority

import rego.v1

# BR-SP-070: Score-based priority aggregation (issue #98)
# Each dimension scored independently, then summed.
severity_score := 3 if { lower(input.signal.severity) == "critical" }
severity_score := 2 if { lower(input.signal.severity) == "warning" }
severity_score := 2 if { lower(input.signal.severity) == "high" }
severity_score := 1 if { lower(input.signal.severity) == "info" }
default severity_score := 0

env_scores contains 3 if { lower(input.environment) == "production" }
env_scores contains 2 if { lower(input.environment) == "staging" }
env_scores contains 1 if { lower(input.environment) == "development" }
env_scores contains 1 if { lower(input.environment) == "test" }
env_scores contains 3 if { input.namespace_labels["tier"] == "critical" }
env_scores contains 2 if { input.namespace_labels["tier"] == "high" }

env_score := max(env_scores) if { count(env_scores) > 0 }
default env_score := 0

composite_score := severity_score + env_score

result := {"priority": "P0", "policy_name": "score-based"} if { composite_score >= 6 }
result := {"priority": "P1", "policy_name": "score-based"} if { composite_score == 5 }
result := {"priority": "P2", "policy_name": "score-based"} if { composite_score == 4 }
result := {"priority": "P3", "policy_name": "score-based"} if { composite_score < 4; composite_score > 0 }

default result := {"priority": "P3", "policy_name": "default-catch-all"}
`)
	Expect(err).NotTo(HaveOccurred())
	_ = priorityPolicyFile.Close()

	// Schedule cleanup of temp files and hot-reload watchers
	DeferCleanup(func() {
		_ = os.Remove(envPolicyFile.Name())
		_ = os.Remove(priorityPolicyFile.Name())
	})

	// Initialize Environment Classifier (BR-SP-051, BR-SP-052, BR-SP-053)
	envClassifier, err := classifier.NewEnvironmentClassifier(
		ctx,
		envPolicyFile.Name(),
		logger,
	)
	Expect(err).NotTo(HaveOccurred())

	// BR-SP-072: Start hot-reload for Environment Classifier
	err = envClassifier.StartHotReload(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Initialize Priority Engine (BR-SP-070, BR-SP-071, BR-SP-072)
	priorityEngine, err := classifier.NewPriorityEngine(
		ctx,
		priorityPolicyFile.Name(),
		logger,
	)
	Expect(err).NotTo(HaveOccurred())

	// BR-SP-072: Start hot-reload for Priority Engine
	err = priorityEngine.StartHotReload(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Create business policy file for BR-SP-002, BR-SP-080, BR-SP-081
	businessPolicyFile, err := os.CreateTemp("", "business-*.rego")
	Expect(err).NotTo(HaveOccurred())
	_, err = businessPolicyFile.WriteString(`package signalprocessing.business
import rego.v1
# BR-SP-002: Business unit classification
result := {"business_unit": "platform", "criticality": "high", "confidence": 0.9, "source": "namespace-labels"} if {
    input.namespace.labels["kubernaut.ai/business-unit"] == "platform"
}
else := {"business_unit": "unknown", "criticality": "medium", "confidence": 0.5, "source": "default"}
`)
	Expect(err).NotTo(HaveOccurred())
	_ = businessPolicyFile.Close()

	// Initialize Business Classifier (BR-SP-002, BR-SP-080, BR-SP-081)
	businessClassifier, err := classifier.NewBusinessClassifier(
		ctx,
		businessPolicyFile.Name(),
		logger,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create severity policy file for BR-SP-105, DD-SEVERITY-001
	severityPolicyFile, err := os.CreateTemp("", "severity-*.rego")
	Expect(err).NotTo(HaveOccurred())
	_, err = severityPolicyFile.WriteString(`package signalprocessing.severity
import rego.v1
# BR-SP-105: Severity Determination via Rego Policy
# DD-SEVERITY-001 v1.1: Aligned with HAPI/workflow catalog (critical/high/medium/low/unknown)
determine_severity := "critical" if {
	input.signal.severity == "sev1"
} else := "critical" if {
	input.signal.severity == "p0"
} else := "critical" if {
	input.signal.severity == "p1"
} else := "high" if {
	input.signal.severity == "sev2"
} else := "high" if {
	input.signal.severity == "p2"
} else := "medium" if {
	input.signal.severity == "sev3"
} else := "medium" if {
	input.signal.severity == "p3"
} else := "low" if {
	input.signal.severity == "sev4"
} else := "low" if {
	input.signal.severity == "p4"
} else := "invalid-severity-enum" if {
	# Test case: Return invalid severity value to trigger validation error
	# This simulates operator error in policy configuration
	input.signal.severity == "trigger-error"
} else := "critical" if {
	# Fallback: unmapped → critical (conservative, operator-defined)
	true
}
`)
	Expect(err).NotTo(HaveOccurred())
	_ = severityPolicyFile.Close()

	// Initialize Severity Classifier (BR-SP-105, DD-SEVERITY-001)
	severityClassifier := classifier.NewSeverityClassifier(
		k8sManager.GetClient(),
		logger,
	)

	// Load severity policy
	severityPolicyContent, err := os.ReadFile(severityPolicyFile.Name())
	Expect(err).NotTo(HaveOccurred())
	err = severityClassifier.LoadRegoPolicy(string(severityPolicyContent))
	Expect(err).NotTo(HaveOccurred())

	// Set policy path for hot-reload (must be done before StartHotReload)
	severityClassifier.SetPolicyPath(severityPolicyFile.Name())

	// BR-SP-072: Start hot-reload for Severity Classifier (DD-SEVERITY-001)
	err = severityClassifier.StartHotReload(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Create predictive signal mappings file for BR-SP-106, ADR-054
	signalModeConfigFile, err := os.CreateTemp("", "predictive-signal-mappings-*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = signalModeConfigFile.WriteString(`# BR-SP-106: Predictive Signal Mode Classification (integration test config)
predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
`)
	Expect(err).NotTo(HaveOccurred())
	_ = signalModeConfigFile.Close()

	// Initialize SignalModeClassifier (BR-SP-106, ADR-054)
	signalModeClassifier := classifier.NewSignalModeClassifier(logger)
	err = signalModeClassifier.LoadConfig(signalModeConfigFile.Name())
	Expect(err).NotTo(HaveOccurred())

	// Schedule cleanup of business, severity, and signal mode config files
	DeferCleanup(func() {
		_ = os.Remove(businessPolicyFile.Name())
		_ = os.Remove(severityPolicyFile.Name())
		_ = os.Remove(signalModeConfigFile.Name())
	})

	// Initialize owner chain builder (Day 7 integration)
	ownerChainBuilder := ownerchain.NewBuilder(k8sManager.GetClient(), logger)

	// Initialize CustomLabels Rego engine (BR-SP-102, BR-SP-104, BR-SP-072)
	labelsPolicyFile, err := os.CreateTemp("", "labels-*.rego")
	Expect(err).NotTo(HaveOccurred())
	_, err = labelsPolicyFile.WriteString(`package signalprocessing.customlabels
import rego.v1
default labels := {}
`)
	Expect(err).NotTo(HaveOccurred())
	_ = labelsPolicyFile.Close()
	labelsPolicyFilePath = labelsPolicyFile.Name() // Store for hot-reload tests

	regoEngine := rego.NewEngine(logger, labelsPolicyFilePath)

	// Load initial policy
	policyContent, err := os.ReadFile(labelsPolicyFile.Name())
	Expect(err).NotTo(HaveOccurred())
	err = regoEngine.LoadPolicy(string(policyContent))
	Expect(err).NotTo(HaveOccurred())

	// BR-SP-072: Start hot-reload for CustomLabels Engine
	err = regoEngine.StartHotReload(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Initialize Metrics (DD-005: Observability)
	// Per AIAnalysis pattern: Use global prometheus.DefaultRegisterer
	sharedMetrics := spmetrics.NewMetrics() // No args = uses global prometheus.DefaultRegisterer

	// Initialize K8s Enricher (BR-SP-001)
	// Metrics are MANDATORY for observability (per k8s_enricher.go panic guard)
	k8sEnricher := enricher.NewK8sEnricher(
		k8sManager.GetClient(),
		logger,
		sharedMetrics, // Pass shared metrics to enricher
		5*time.Second, // Timeout for K8s API calls
	)

	// Initialize StatusManager (DD-PERF-001: Atomic Status Updates)
	// SP-CACHE-001: Pass APIReader to bypass cache for fresh refetches
	statusManager := spstatus.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader())

	// Initialize EventRecorder (K8s best practice)
	recorder := k8sManager.GetEventRecorderFor("signalprocessing-controller")

	// Initialize SignalProcessing controller with ALL dependencies
	err = (&signalprocessing.SignalProcessingReconciler{
		Client:             k8sManager.GetClient(),
		Scheme:             k8sManager.GetScheme(),
		AuditClient:        auditClient,   // Legacy audit client
		AuditManager:       auditManager,  // Phase 3 refactoring - MANDATORY per ADR-032
		Metrics:            sharedMetrics, // DD-005: Observability
		Recorder:           recorder,
		StatusManager:      statusManager, // DD-PERF-001 + SP-CACHE-001
		EnvClassifier:      envClassifier,
		PriorityAssigner:   priorityEngine, // PriorityEngine implements PriorityAssigner interface
		BusinessClassifier: businessClassifier,
		SeverityClassifier: severityClassifier, // BR-SP-105, DD-SEVERITY-001: Severity determination
		RegoEngine:         regoEngine,         // BR-SP-102, BR-SP-104: CustomLabels extraction
		K8sEnricher:        k8sEnricher,        // BR-SP-001: K8s context enrichment (interface)
		OwnerChainBuilder:    ownerChainBuilder,    // BR-SP-100: Owner chain analysis
		SignalModeClassifier: signalModeClassifier, // BR-SP-106: Predictive signal mode classification (ADR-054)
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	By("Starting the per-process controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ Phase 2 Complete: Per-process controller ready")
	GinkgoWriter.Println("  • envtest: Per-process in-memory K8s API server")
	GinkgoWriter.Println("  • k8sManager: Per-process controller-runtime manager")
	GinkgoWriter.Println("  • SignalProcessing controller: Running in this process")
	GinkgoWriter.Println("  • Audit infrastructure: Shared (PostgreSQL:15436, Redis:16382, DataStorage:18094)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// SP-BUG-005: Use SynchronizedAfterSuite to make Process 1-only cleanup explicit
// Function 1: Runs on ALL processes (per-process cleanup)
// Function 2: Runs ONLY on Process 1 (shared infrastructure cleanup)
var _ = SynchronizedAfterSuite(
	func() {
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// ALL PROCESSES: Per-process cleanup (DD-TEST-010 Multi-Controller)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		By("Tearing down per-process test environment")

		// Flush audit store before shutdown (BR-SP-090)
		if auditStore != nil {
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer flushCancel()
			if err := auditStore.Flush(flushCtx); err != nil {
				GinkgoWriter.Printf("⚠️  Failed to flush audit store during cleanup: %v\n", err)
			}
		}

		// Cancel context to stop controller manager
		if cancel != nil {
			cancel()
		}

		// Stop per-process envtest
		if testEnv != nil {
			By("Stopping per-process envtest")
			err := testEnv.Stop()
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop envtest: %v\n", err)
			}
		}

		GinkgoWriter.Println("✅ Per-process cleanup complete (envtest, controller, audit store)")

		// Stop testEnv if it was created (each process has its own)
		if testEnv != nil {
			err := testEnv.Stop()
			if err != nil {
				GinkgoWriter.Printf("⚠️ Warning: Failed to stop testEnv: %v\n", err)
			}
		}
	},
	func() {
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// PROCESS 1 ONLY: Shared infrastructure cleanup
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		By("Tearing down shared infrastructure (Process 1 only)")

		// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
		// ALWAYS collect logs - failures may have occurred on other parallel processes
		// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
		GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
		infrastructure.MustGatherContainerLogs("signalprocessing", []string{
			dsInfra.DataStorageContainer,
			dsInfra.PostgresContainer,
			dsInfra.RedisContainer,
		}, GinkgoWriter)

		// Clean up audit infrastructure (BR-SP-090)
		// SP-SHUTDOWN-001: Flush audit store BEFORE stopping DataStorage
		// This prevents "connection refused" errors during cleanup when the
		// background writer tries to flush buffered events after DataStorage is stopped.
		// Integration tests MUST always use real DataStorage (DD-TESTING-001)
		if auditStore != nil {
			GinkgoWriter.Println("🧹 Flushing audit store before infrastructure shutdown...")

			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer flushCancel()

			err := auditStore.Flush(flushCtx)
			if err != nil {
				GinkgoWriter.Printf("⚠️ Warning: Failed to flush audit store: %v\n", err)
			} else {
				GinkgoWriter.Println("✅ Audit store flushed (all buffered events written)")
			}

			err = auditStore.Close()
			if err != nil {
				GinkgoWriter.Printf("⚠️ Warning: Failed to close audit store: %v\n", err)
			} else {
				GinkgoWriter.Println("✅ Audit store closed")
			}
		}

		// Infrastructure cleanup handled by DeferCleanup (StopDSBootstrap)
		// SP-SHUTDOWN-001: Safe to stop now - audit events already flushed

		// DD-TEST-002 + DD-INTEGRATION-001 v2.0: Clean up composite-tagged images
		By("Cleaning up infrastructure images to prevent disk space issues")
		// Prune dangling images (composite tags from this test run)
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		if pruneErr := pruneCmd.Run(); pruneErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n", pruneErr)
		} else {
			GinkgoWriter.Println("✅ Infrastructure images pruned")
		}
	},
)

// ============================================================================
// TEST HELPER FUNCTIONS (for parallel execution isolation)
// ============================================================================

// createTestNamespace creates a managed test namespace for test isolation.
// Delegates to shared helpers.CreateTestNamespace with kubernaut.ai/managed=true.
// DD-TEST-002: UUID-based naming for parallel execution safety (handled by shared helper).
func createTestNamespace(prefix string) string {
	return helpers.CreateTestNamespace(ctx, k8sClient, prefix)
}

// createTestNamespaceWithLabels creates a managed test namespace with additional custom labels.
// The kubernaut.ai/managed=true label is applied by default; custom labels are merged.
// Used for testing environment classification from namespace labels (BR-SP-051).
func createTestNamespaceWithLabels(prefix string, labels map[string]string) string {
	return helpers.CreateTestNamespace(ctx, k8sClient, prefix, helpers.WithLabels(labels))
}

// deleteTestNamespace cleans up a test namespace.
// Delegates to shared helpers.DeleteTestNamespace.
func deleteTestNamespace(ns string) {
	helpers.DeleteTestNamespace(ctx, k8sClient, ns)
}

// waitForPhase waits for a SignalProcessing CR to reach a specific phase.
// Returns error if timeout is exceeded.
func waitForPhase(name, namespace string, expectedPhase signalprocessingv1alpha1.SignalProcessingPhase, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(pollCtx context.Context) (bool, error) {
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
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(pollCtx context.Context) (bool, error) {
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
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, timeout, true, func(pollCtx context.Context) (bool, error) {
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

// ADR-056: createTestPDB, createTestHPA, createTestNetworkPolicy removed
// — BR-SP-101 detection tests relocated to HAPI

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
