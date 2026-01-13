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

	"github.com/google/uuid"
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
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/detection"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
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
	auditStore audit.AuditStore   // Audit store for BR-SP-090 (write operations)
	dsClient   *ogenclient.Client // DataStorage HTTP API client (query operations - correct service boundary)
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

	By("Starting SignalProcessing integration infrastructure (DD-TEST-002)")
	// This starts: PostgreSQL, Redis, Immudb, DataStorage (with migrations)
	// Per DD-TEST-001 v2.2: PostgreSQL=15436, Redis=16382, Immudb=13324, DS=18094
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "signalprocessing",
		PostgresPort:    15436, // DD-TEST-001 v2.2
		RedisPort:       16382, // DD-TEST-001 v2.2
		DataStoragePort: 18094, // DD-TEST-001 v2.2 (OFFICIAL SP allocation)
		MetricsPort:     19094,
		ConfigDir:       "test/integration/signalprocessing/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All services started and healthy (PostgreSQL, Redis, Immudb, DataStorage)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
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

	// DD-TEST-010: Multi-Controller Pattern - No config serialization needed
	// Each process will create its own envtest + controller in Phase 2
	return []byte{}
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

	// Create DataStorage client adapter for audit store (per-process)
	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	mockTransport := testauth.NewMockUserTransport("test-signalprocessing@integration.test")
	dsAuditClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
		5*time.Second,
		mockTransport,
	)
	Expect(err).NotTo(HaveOccurred(), "Failed to create OpenAPI client adapter")

	// SP-CACHE-001: Create audit store per-process (uses shared DataStorage infrastructure)
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	logger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsAuditClient, auditConfig, "signalprocessing", logger)
	Expect(err).NotTo(HaveOccurred(), "Audit store creation must succeed for BR-SP-090")
	GinkgoWriter.Println("✅ Per-process audit store configured")

	// Create DataStorage ogen client for audit event queries in tests
	// Per service boundary rules: SignalProcessing queries DataStorage via HTTP API
	dsClient, err = ogenclient.NewClient(
		fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
	)
	Expect(err).NotTo(HaveOccurred(), "DataStorage ogen client creation must succeed")
	GinkgoWriter.Printf("✅ DataStorage ogen client ready for test queries\n")

	By("Setting up the per-process controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // DD-TEST-010: Random port per process to avoid conflicts
		},
	})
	Expect(err).NotTo(HaveOccurred())

	By("Setting up the SignalProcessing controller with audit client")
	// Create audit client for BR-SP-090 compliance
	auditClient := spaudit.NewAuditClient(auditStore, logger)

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
# DD-SEVERITY-001: Strategy B - Policy-Defined Fallback
determine_severity := "critical" if {
	input.signal.severity == "Sev1"
} else := "critical" if {
	input.signal.severity == "P0"
} else := "critical" if {
	input.signal.severity == "P1"
} else := "warning" if {
	input.signal.severity == "Sev2"
} else := "warning" if {
	input.signal.severity == "P2"
} else := "info" if {
	input.signal.severity == "Sev3"
} else := "info" if {
	input.signal.severity == "P3"
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

	// Schedule cleanup of business and severity policy files
	DeferCleanup(func() {
		_ = os.Remove(businessPolicyFile.Name())
		_ = os.Remove(severityPolicyFile.Name())
	})

	// Initialize owner chain builder (Day 7 integration)
	ownerChainBuilder := ownerchain.NewBuilder(k8sManager.GetClient(), logger)

	// Initialize CustomLabels Rego engine (BR-SP-102, BR-SP-104, BR-SP-072)
	labelsPolicyFile, err := os.CreateTemp("", "labels-*.rego")
	Expect(err).NotTo(HaveOccurred())
	_, err = labelsPolicyFile.WriteString(`package signalprocessing.labels
import rego.v1
result := {}
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

	// Initialize Label Detector (BR-SP-101)
	labelDetector := detection.NewLabelDetector(k8sManager.GetClient(), logger)

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
		AuditClient:        auditClient,
		Metrics:            sharedMetrics, // DD-005: Observability
		Recorder:           recorder,
		StatusManager:      statusManager, // DD-PERF-001 + SP-CACHE-001
		EnvClassifier:      envClassifier,
		PriorityAssigner:   priorityEngine, // PriorityEngine implements PriorityAssigner interface
		BusinessClassifier: businessClassifier,
		SeverityClassifier: severityClassifier, // BR-SP-105, DD-SEVERITY-001: Severity determination
		RegoEngine:         regoEngine,         // BR-SP-102, BR-SP-104: CustomLabels extraction
		LabelDetector:      labelDetector,      // BR-SP-101: Detected labels
		K8sEnricher:        k8sEnricher,        // BR-SP-001: K8s context enrichment (interface)
		OwnerChainBuilder:  ownerChainBuilder,  // BR-SP-100: Owner chain analysis
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
// DD-TEST-002: Uses UUID-based namespace generation for parallel execution safety.
// MANDATORY per 03-testing-strategy.mdc: Each test must use unique identifiers.
func createTestNamespace(prefix string) string {
	// DD-TEST-002: UUID-based naming prevents collisions in parallel execution
	ns := fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
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
// DD-TEST-002: Uses UUID-based namespace generation for parallel execution safety.
// Used for testing environment classification from namespace labels (BR-SP-051).
func createTestNamespaceWithLabels(prefix string, labels map[string]string) string {
	// DD-TEST-002: UUID-based naming prevents collisions in parallel execution
	ns := fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
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
