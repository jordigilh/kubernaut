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

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Suite-level variables
var (
	cfg        *rest.Config
	k8sClient  client.Client
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
	// Package-level variables are NOT shared across processes, so we must serialize data
	// Note: DataStorage health check now includes auth readiness validation
	// StartDSBootstrap waits for /health to return 200, which includes auth middleware check
	sharedData := fmt.Sprintf("%s|%s", authConfig.Token, infra.GetDataStorageURL())
	return []byte(sharedData)

}, func(data []byte) {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: Runs on ALL parallel processes
	// Setup per-process resources (envtest + webhook server + clients)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	GinkgoWriter.Printf("🔧 [Process %d] Setting up per-process resources...\n", GinkgoParallelProcess())

	// DD-AUTH-014: Extract ServiceAccount token AND DataStorage URL from Phase 1
	// Phase 1 serializes: "token|url"
	sharedData := string(data)
	parts := []string{"", ""}
	if idx := len(sharedData) - 1; idx >= 0 {
		// Find the LAST '|' separator (token may contain '|' but URL won't)
		for i := len(sharedData) - 1; i >= 0; i-- {
			if sharedData[i] == '|' {
				parts[0] = sharedData[:i]   // Token
				parts[1] = sharedData[i+1:] // URL
				break
			}
		}
	}
	saToken := parts[0]
	dataStorageURL := parts[1]
	GinkgoWriter.Printf("[Process %d] Received ServiceAccount token and DataStorage URL from Phase 1\n", GinkgoParallelProcess())

	By("Initializing Data Storage OpenAPI client (DD-API-001)")
	var err error
	dsClient, err = ogenclient.NewClient(dataStorageURL)
	if err != nil {
		Fail(fmt.Sprintf("DD-API-001 violation: Cannot proceed without DataStorage client: %v", err))
	}
	GinkgoWriter.Printf("[Process %d] ✅ Data Storage Ogen client initialized\n", GinkgoParallelProcess())

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

	By("Creating K8s client for CRD operations")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Setting up webhook server with envtest")
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	webhookServer := webhook.NewServer(webhook.Options{
		Host:    webhookInstallOptions.LocalServingHost,
		Port:    webhookInstallOptions.LocalServingPort,
		CertDir: webhookInstallOptions.LocalServingCertDir,
	})

	By("Registering webhook handlers (GREEN phase)")
	// Create decoder for webhook handlers
	decoder := admission.NewDecoder(scheme.Scheme)

	// Register WorkflowExecution mutating webhook (DD-WEBHOOK-003: Complete audit events)
	wfeHandler := authwebhook.NewWorkflowExecutionAuthHandler(auditStore)
	_ = wfeHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})

	// Register RemediationApprovalRequest mutating webhook (DD-WEBHOOK-003: Complete audit events)
	rarHandler := authwebhook.NewRemediationApprovalRequestAuthHandler(auditStore)
	_ = rarHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})

	// Register NotificationRequest DELETE validator (DD-WEBHOOK-003: Complete audit events)
	// Uses Kubebuilder CustomValidator interface for envtest compatibility
	// Reference: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation
	nrValidator := authwebhook.NewNotificationRequestValidator(auditStore)
	webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{
		Handler: admission.WithCustomValidator(scheme.Scheme, &notificationv1.NotificationRequest{}, nrValidator),
	})

	By("Starting webhook server")
	go func() {
		defer GinkgoRecover()
		err := webhookServer.Start(ctx)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Webhook server error: %v\n", err)
		}
	}()

	By("Webhook server ready")
	// envtest automatically installs webhook configurations from WebhookInstallOptions.Paths
	// and ensures webhook server is ready before proceeding
	GinkgoWriter.Printf("[Process %d] ✅ Webhook server ready (envtest handles configuration automatically)\n", GinkgoParallelProcess())

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ envtest environment ready")
	GinkgoWriter.Printf("   • Webhook server: %s:%d\n", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	GinkgoWriter.Printf("   • CertDir: %s\n", webhookInstallOptions.LocalServingCertDir)
	GinkgoWriter.Println("   • K8s client configured for CRD operations")
	GinkgoWriter.Println("   • Webhook configurations applied (Mutating + Validating)")
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
