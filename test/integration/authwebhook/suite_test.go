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

package authwebhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/integration"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// sharedInfraData is passed from Phase 1 (Process #1) to Phase 2 (ALL processes)
// via Ginkgo's SynchronizedBeforeSuite []byte data passing mechanism
type sharedInfraData struct {
	ServiceAccountToken string `json:"serviceAccountToken"`
	DataStorageURL      string `json:"dataStorageURL"`
}

// Suite-level variables
var (
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager // Manager for webhook server lifecycle (follows production pattern)
	testEnv    *envtest.Environment
	ctx        context.Context
	cancel     context.CancelFunc
	auditStore audit.AuditStore   // REAL audit store for webhook handlers
	dsClient   *ogenclient.Client // DD-TESTING-001: Ogen OpenAPI-generated client
	infra      *testinfra.AuthWebhookInfrastructure
)

// auditStoreAdapter adapts audit.AuditStore to authwebhook.AuditManager interface
// This allows webhook handlers to use the real audit store
func TestAuthWebhookIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Integration Suite - BR-AUTH-001 SOC2 Attribution")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 1: Runs ONCE on parallel process #1
	// Setup shared infrastructure (envtest + PostgreSQL + Redis + Data Storage)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	GinkgoWriter.Printf("🔧 [Process %d] AuthWebhook Integration Test Suite - DD-TEST-002\n", GinkgoParallelProcess())
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	GinkgoWriter.Printf("Creating shared test infrastructure (Process #1 only)...\n")
	GinkgoWriter.Printf("  • envtest (for ServiceAccount authentication)\n")
	GinkgoWriter.Printf("  • PostgreSQL (port 15442)\n")
	GinkgoWriter.Printf("  • Redis (port 16386)\n")
	GinkgoWriter.Printf("  • Data Storage API (port 18099)\n")
	GinkgoWriter.Printf("  • Parallel Execution: 4 concurrent processors\n")
	GinkgoWriter.Printf("  • Pattern: DD-TEST-002 + DD-AUTH-014\n")
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// DD-AUTH-014: Create envtest FIRST for ServiceAccount authentication
	By("Creating envtest for DataStorage authentication (DD-AUTH-014)")
	sharedTestEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	sharedK8sConfig, err := sharedTestEnv.Start()
	if err != nil {
		Fail(fmt.Sprintf("Failed to start envtest: %v", err))
	}
	GinkgoWriter.Printf("✅ envtest started: %s\n", sharedK8sConfig.Host)

	// Write kubeconfig to temporary file for DataStorage container
	kubeconfigPath, err := testinfra.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "authwebhook-integration")
	if err != nil {
		Fail(fmt.Sprintf("Failed to write envtest kubeconfig: %v", err))
	}
	GinkgoWriter.Printf("✅ envtest kubeconfig written: %s\n", kubeconfigPath)

	// DD-AUTH-014: Create ServiceAccount with DataStorage access
	GinkgoWriter.Println("🔐 Creating ServiceAccount for DataStorage authentication...")
	authConfig, err := testinfra.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedK8sConfig,
		"authwebhook-integration-sa",
		"default",
		GinkgoWriter,
	)
	if err != nil {
		Fail(fmt.Sprintf("Failed to create ServiceAccount: %v", err))
	}
	GinkgoWriter.Println("✅ ServiceAccount created with Bearer token")

	By("Setting up Data Storage infrastructure (PostgreSQL + Redis + Data Storage service)")
	infra = testinfra.NewAuthWebhookInfrastructure()
	err = infra.SetupWithAuth(authConfig, GinkgoWriter)
	if err != nil {
		Fail(fmt.Sprintf("Failed to setup infrastructure: %v", err))
	}
	infra.SharedTestEnv = sharedTestEnv // Store for cleanup

	GinkgoWriter.Printf("✅ Shared infrastructure ready (Process #1)\n")
	// DD-AUTH-014: Pass ServiceAccount token AND DataStorage URL to all processes
	// Phase 1 runs only on Process #1, but Phase 2 runs on ALL processes
	// Package-level variables (like 'infra') are NOT shared across processes
	// We must marshal data into []byte for Ginkgo to pass to all processes
	// Note: DataStorage health check now includes auth readiness validation
	// StartDSBootstrap waits for /health to return 200, which includes auth middleware check
	sharedData := sharedInfraData{
		ServiceAccountToken: authConfig.Token,
		DataStorageURL:      infra.GetDataStorageURL(),
	}
	data, err := json.Marshal(sharedData)
	if err != nil {
		Fail(fmt.Sprintf("Failed to marshal shared data: %v", err))
	}
	return data

}, func(data []byte) {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: Runs on ALL parallel processes
	// Setup per-process resources (envtest + webhook server + clients)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	GinkgoWriter.Printf("🔧 [Process %d] Setting up per-process resources...\n", GinkgoParallelProcess())

	// DD-AUTH-014: Unmarshal shared data from Phase 1
	// Phase 1 (Process #1) marshaled ServiceAccount token + DataStorage URL
	// Phase 2 (ALL processes) unmarshal to access both values
	var sharedData sharedInfraData
	if err := json.Unmarshal(data, &sharedData); err != nil {
		Fail(fmt.Sprintf("Failed to unmarshal shared data: %v", err))
	}
	saToken := sharedData.ServiceAccountToken
	dataStorageURL := sharedData.DataStorageURL
	GinkgoWriter.Printf("[Process %d] Received ServiceAccount token and DataStorage URL from Phase 1\n", GinkgoParallelProcess())

	By("Initializing Data Storage OpenAPI client with authentication (DD-API-001 + DD-AUTH-014)")
	// DD-AUTH-014: Use authenticated client for audit queries
	// Pattern: test/shared/integration/datastorage_auth.go (validated in Gateway, AIAnalysis, HAPI, DataStorage)
	var err error
	dsClients := integration.NewAuthenticatedDataStorageClients(
		dataStorageURL,
		saToken,
		5*time.Second,
	)
	dsClient = dsClients.OpenAPIClient // Use authenticated client for test queries
	GinkgoWriter.Printf("[Process %d] ✅ Data Storage authenticated client initialized (DD-AUTH-014)\n", GinkgoParallelProcess())

	By("Creating REAL audit store with ServiceAccount authentication (DD-AUTH-014)")
	// Create OpenAPI DataStorage client adapter for audit writes
	// DD-AUTH-014: Integration tests use ServiceAccount Bearer token authentication
	authTransport := testauth.NewServiceAccountTransport(saToken)
	dsAuditClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		dataStorageURL,
		5*time.Second,
		authTransport, // ✅ Bearer token authentication (DD-AUTH-014)
	)
	if err != nil {
		Fail(fmt.Sprintf("Failed to create OpenAPI DataStorage audit client: %v", err))
	}

	// Create REAL buffered audit store (per ADR-038)
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Fast flush for tests
	auditStore, err = audit.NewBufferedStore(
		dsAuditClient,
		auditConfig,
		"authwebhook",
		logf.Log.WithName("audit"),
	)
	if err != nil {
		Fail(fmt.Sprintf("Failed to create audit store: %v", err))
	}
	GinkgoWriter.Printf("[Process %d] ✅ Real audit store created with authenticated access\n", GinkgoParallelProcess())

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STEP 2: Setup envtest + webhook server
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency
	By("Verifying KUBEBUILDER_ASSETS is set by Makefile")
	Expect(os.Getenv("KUBEBUILDER_ASSETS")).ToNot(BeEmpty(), "KUBEBUILDER_ASSETS must be set by Makefile (test-integration-% → setup-envtest)")

	By("Bootstrapping test environment with envtest + webhook")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Registering CRD schemes")
	err = workflowexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = remediationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = notificationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Creating controller-runtime Manager (follows production pattern)")
	// Pattern: cmd/authwebhook/main.go (same Manager setup as production)
	// DD-TEST-002: Manager provides built-in webhook server readiness handling
	// This eliminates the race condition where tests start before webhook server is ready
	webhookInstallOptions := &testEnv.WebhookInstallOptions

	// Load TLS certificate once at startup (bypass certwatcher for test stability)
	// In parallel execution, certwatcher can fail when other processes delete cert files
	// Production uses Kubernetes cert-manager with rotation; tests use static certs
	certPath := filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.crt")
	keyPath := filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.key")
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load webhook TLS certificate")
	GinkgoWriter.Printf("[Process %d] ✅ Loaded TLS certificate (bypassing certwatcher)\n", GinkgoParallelProcess())

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics for tests
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
			TLSOpts: []func(*tls.Config){
				// Provide certificate directly (bypasses certwatcher file monitoring)
				// This prevents "no such file or directory" errors in parallel execution
				func(config *tls.Config) {
					config.GetCertificate = func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
						return &cert, nil
					}
				},
			},
		}),
	})
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Printf("[Process %d] ✅ Manager created with webhook server (certwatcher disabled)\n", GinkgoParallelProcess())

	By("Getting K8s client from Manager")
	// Pattern: All other integration tests (RO, SP, AA, WE, NT)
	// DD-TEST-009: Use Manager's client to ensure field indexes work correctly
	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	By("Registering webhook handlers (follows production pattern)")
	// Pattern: cmd/authwebhook/main.go (identical handler registration)
	webhookServer := k8sManager.GetWebhookServer()
	decoder := admission.NewDecoder(scheme.Scheme)

	// Register WorkflowExecution mutating webhook (DD-WEBHOOK-003: Complete audit events)
	wfeHandler := authwebhook.NewWorkflowExecutionAuthHandler(auditStore)
	err = wfeHandler.InjectDecoder(decoder)
	Expect(err).ToNot(HaveOccurred())
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
	GinkgoWriter.Println("   ✅ Registered WorkflowExecution webhook handler")

	// Register RemediationApprovalRequest mutating webhook (DD-WEBHOOK-003: Complete audit events)
	rarHandler := authwebhook.NewRemediationApprovalRequestAuthHandler(auditStore)
	err = rarHandler.InjectDecoder(decoder)
	Expect(err).ToNot(HaveOccurred())
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
	GinkgoWriter.Println("   ✅ Registered RemediationApprovalRequest webhook handler")

	// Register NotificationRequest DELETE validator (DD-WEBHOOK-003: Complete audit events)
	nrValidator := authwebhook.NewNotificationRequestValidator(auditStore)
	webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{
		Handler: admission.WithCustomValidator(scheme.Scheme, &notificationv1.NotificationRequest{}, nrValidator),
	})
	GinkgoWriter.Println("   ✅ Registered NotificationRequest DELETE webhook handler")

	By("Starting Manager (webhook server lifecycle managed automatically)")
	// Pattern: All other integration tests (RO, SP, AA, WE, NT)
	// Manager.Start() blocks until webhook server is ready, then starts accepting requests
	// This ELIMINATES the race condition where tests started before webhook was ready
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
	GinkgoWriter.Printf("[Process %d] ✅ Manager started (webhook server ready for requests)\n", GinkgoParallelProcess())

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ AuthWebhook integration test environment ready")
	GinkgoWriter.Printf("   • Webhook server: %s:%d (via Manager)\n", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	GinkgoWriter.Printf("   • CertDir: %s\n", webhookInstallOptions.LocalServingCertDir)
	GinkgoWriter.Println("   • Manager client configured (field indexes supported)")
	GinkgoWriter.Println("   • Webhook configurations applied (Mutating + Validating)")
	GinkgoWriter.Println("   • Pattern: Matches production (cmd/authwebhook/main.go)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})
var _ = SynchronizedAfterSuite(func() {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 1: Runs on ALL parallel processes
	// Cleanup per-process resources (audit store + envtest + context)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	GinkgoWriter.Printf("🧹 [Process %d] Cleaning up per-process resources...\n", GinkgoParallelProcess())

	By("Flushing audit store before teardown")
	if auditStore != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer flushCancel()
		err := auditStore.Flush(flushCtx)
		if err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Warning: Failed to flush audit store: %v\n", GinkgoParallelProcess(), err)
		} else {
			GinkgoWriter.Printf("[Process %d] ✅ Audit store flushed\n", GinkgoParallelProcess())
		}
	}

	By("Tearing down the test environment (envtest)")
	if cancel != nil {
		cancel()
	}
	if testEnv != nil {
		err := testEnv.Stop()
		if err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Warning: Failed to stop envtest: %v\n", GinkgoParallelProcess(), err)
		} else {
			GinkgoWriter.Printf("[Process %d] ✅ envtest stopped\n", GinkgoParallelProcess())
		}
	}

	GinkgoWriter.Printf("[Process %d] ✅ Per-process cleanup complete\n", GinkgoParallelProcess())

}, func() {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: Runs ONCE on process #1 AFTER all processes finish
	// Teardown shared infrastructure (PostgreSQL + Redis + Data Storage)
	//
	// NOTE: SynchronizedAfterSuite guarantees Phase 2 runs AFTER all processes
	// complete Phase 1. No time.Sleep() needed - Ginkgo handles synchronization.
	// Per TESTING_GUIDELINES.md: time.Sleep() for async waits is forbidden.
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Printf("🧹 [Process %d] Tearing down shared infrastructure (Process #1 only)...\n", GinkgoParallelProcess())
	GinkgoWriter.Println("   (Ginkgo guarantees all processes finished Phase 1 cleanup)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	if infra != nil && infra.DSBootstrapInfra != nil {
		GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
		testinfra.MustGatherContainerLogs("authwebhook", []string{
			infra.DataStorageContainer,
			infra.PostgresContainer,
			infra.RedisContainer,
		}, GinkgoWriter)
	}

	By("Tearing down Data Storage infrastructure")
	if infra != nil {
		_ = infra.Teardown(GinkgoWriter) // Ignore errors during cleanup
		GinkgoWriter.Println("✅ Shared infrastructure torn down (PostgreSQL + Redis + Data Storage)")

		// DD-AUTH-014: Stop shared envtest
		if infra.SharedTestEnv != nil {
			if sharedEnv, ok := infra.SharedTestEnv.(*envtest.Environment); ok {
				if err := sharedEnv.Stop(); err != nil {
					GinkgoWriter.Printf("⚠️  Failed to stop shared envtest: %v\n", err)
				} else {
					GinkgoWriter.Println("✅ Shared envtest stopped")
				}
			}
		}
	}

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ AuthWebhook Integration Test Suite - Teardown Complete")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})
