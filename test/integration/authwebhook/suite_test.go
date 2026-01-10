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
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/pkg/webhooks"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testinfra "github.com/jordigilh/kubernaut/test/infrastructure"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Suite-level variables
var (
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
	auditStore       audit.AuditStore // REAL audit store for webhook handlers
	dsClient         *ogenclient.Client // DD-TESTING-001: Ogen OpenAPI-generated client
	infra            *testinfra.AuthWebhookInfrastructure
)

// auditStoreAdapter adapts audit.AuditStore to webhooks.AuditManager interface
// This allows webhook handlers to use the real audit store
func TestAuthWebhookIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Integration Suite - BR-AUTH-001 SOC2 Attribution")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 1: Runs ONCE on parallel process #1
	// Setup shared infrastructure (PostgreSQL + Redis + Data Storage)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	GinkgoWriter.Printf("🔧 [Process %d] AuthWebhook Integration Test Suite - DD-TEST-002\n", GinkgoParallelProcess())
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	GinkgoWriter.Printf("Creating shared test infrastructure (Process #1 only)...\n")
	GinkgoWriter.Printf("  • PostgreSQL (port 15442)\n")
	GinkgoWriter.Printf("  • Redis (port 16386)\n")
	GinkgoWriter.Printf("  • Data Storage API (port 18099)\n")
	GinkgoWriter.Printf("  • Parallel Execution: 4 concurrent processors\n")
	GinkgoWriter.Printf("  • Pattern: DD-TEST-002 (Synchronized Suite Setup)\n")
	GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	By("Setting up Data Storage infrastructure (PostgreSQL + Redis + Data Storage service)")
	infra = testinfra.NewAuthWebhookInfrastructure()
	err := infra.Setup(GinkgoWriter)
	if err != nil {
		Fail(fmt.Sprintf("Failed to setup infrastructure: %v", err))
	}

	// Share Data Storage URL with all processes via byte slice
	dataStorageURL := infra.GetDataStorageURL()
	GinkgoWriter.Printf("✅ Shared infrastructure ready (Process #1) - Data Storage: %s\n", dataStorageURL)
	return []byte(dataStorageURL) // Pass URL to all processes

}, func(data []byte) {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: Runs on ALL parallel processes
	// Setup per-process resources (envtest + webhook server + clients)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	GinkgoWriter.Printf("🔧 [Process %d] Setting up per-process resources...\n", GinkgoParallelProcess())

	// Receive Data Storage URL from Phase 1
	dataStorageURL := string(data)
	GinkgoWriter.Printf("[Process %d] Received Data Storage URL: %s\n", GinkgoParallelProcess(), dataStorageURL)

	By("Initializing Data Storage OpenAPI client (DD-API-001)")
	var err error
	dsClient, err = ogenclient.NewClient(dataStorageURL)
	if err != nil {
		Fail(fmt.Sprintf("DD-API-001 violation: Cannot proceed without DataStorage client: %v", err))
	}
	GinkgoWriter.Printf("[Process %d] ✅ Data Storage Ogen client initialized\n", GinkgoParallelProcess())

	By("Creating REAL audit store for webhook handlers")
	// Create OpenAPI DataStorage client adapter for audit writes
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	mockTransport := testutil.NewMockUserTransport("test-authwebhook@integration.test")
	dsAuditClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		dataStorageURL,
		5*time.Second,
		mockTransport, // ← Mock user header injection (simulates oauth-proxy)
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
	GinkgoWriter.Printf("[Process %d] ✅ Real audit store created (connected to DataStorage)\n", GinkgoParallelProcess())

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STEP 2: Setup envtest + webhook server
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
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
	wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
	_ = wfeHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})

	// Register RemediationApprovalRequest mutating webhook (DD-WEBHOOK-003: Complete audit events)
	rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(auditStore)
	_ = rarHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})

	// Register NotificationRequest DELETE validator (DD-WEBHOOK-003: Complete audit events)
	// Uses Kubebuilder CustomValidator interface for envtest compatibility
	// Reference: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation
	nrValidator := webhooks.NewNotificationRequestValidator(auditStore)
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

// configureWebhooks creates MutatingWebhookConfiguration and ValidatingWebhookConfiguration
// resources in the envtest API server, pointing to the webhook server we started.
func configureWebhooks(ctx context.Context, k8sClient client.Client, webhookOpts envtest.WebhookInstallOptions) error {
	// Read CA cert PEM file directly (envtest uses self-signed certs)
	caBundle, err := os.ReadFile(filepath.Join(webhookOpts.LocalServingCertDir, "tls.crt"))
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	// Construct webhook URL
	webhookHost := webhookOpts.LocalServingHost
	if webhookHost == "" {
		webhookHost = "127.0.0.1"
	}
	webhookPort := webhookOpts.LocalServingPort
	webhookURL := fmt.Sprintf("https://%s", net.JoinHostPort(webhookHost, fmt.Sprintf("%d", webhookPort)))

	// Create MutatingWebhookConfiguration
	mutatingWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-authwebhook-mutating",
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "workflowexecution.mutate.kubernaut.ai",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL:      ptr.To(webhookURL + "/mutate-workflowexecution"),
					CABundle: caBundle,
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"kubernaut.ai"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"workflowexecutions/status"},
							Scope:       ptr.To(admissionregistrationv1.NamespacedScope),
						},
					},
				},
				FailurePolicy:           ptr.To(admissionregistrationv1.Fail),
				SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
				TimeoutSeconds:          ptr.To(int32(10)),
			},
			{
				Name: "remediationapprovalrequest.mutate.kubernaut.ai",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL:      ptr.To(webhookURL + "/mutate-remediationapprovalrequest"),
					CABundle: caBundle,
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"kubernaut.ai"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"remediationapprovalrequests/status"},
							Scope:       ptr.To(admissionregistrationv1.NamespacedScope),
						},
					},
				},
				FailurePolicy:           ptr.To(admissionregistrationv1.Fail),
				SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
				TimeoutSeconds:          ptr.To(int32(10)),
			},
			// Note: NotificationRequest DELETE webhook moved to ValidatingWebhookConfiguration
			// (K8s doesn't invoke mutating webhooks for DELETE operations)
		},
	}

	if err := k8sClient.Create(ctx, mutatingWebhook); err != nil {
		return fmt.Errorf("failed to create MutatingWebhookConfiguration: %w", err)
	}

	// Create ValidatingWebhookConfiguration for DELETE operations
	// Note: Kubernetes doesn't invoke mutating webhooks for DELETE
	// (nothing to mutate), so we use validating webhook for audit capture
	validatingWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-authwebhook-validating",
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "notificationrequest.validate.kubernaut.ai",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL:      ptr.To(webhookURL + "/validate-notificationrequest-delete"),
					CABundle: caBundle,
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Delete,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"kubernaut.ai"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"notificationrequests"},
							Scope:       ptr.To(admissionregistrationv1.NamespacedScope),
						},
					},
				},
				FailurePolicy:           ptr.To(admissionregistrationv1.Fail),
				SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNoneOnDryRun),
				AdmissionReviewVersions: []string{"v1"},
				TimeoutSeconds:          ptr.To(int32(10)),
			},
		},
	}

	if err := k8sClient.Create(ctx, validatingWebhook); err != nil {
		return fmt.Errorf("failed to create ValidatingWebhookConfiguration: %w", err)
	}

	// Wait for webhook configurations to be ready
	time.Sleep(1 * time.Second)

	return nil
}

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

	By("Tearing down Data Storage infrastructure")
	if infra != nil {
		_ = infra.Teardown(GinkgoWriter) // Ignore errors during cleanup
		GinkgoWriter.Println("✅ Shared infrastructure torn down (PostgreSQL + Redis + Data Storage)")
	}

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ AuthWebhook Integration Test Suite - Teardown Complete")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

