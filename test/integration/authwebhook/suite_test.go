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
	"encoding/json"
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
	"github.com/jordigilh/kubernaut/pkg/webhooks"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
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
	dsClient         *dsgen.ClientWithResponses // DD-TESTING-001: OpenAPI-generated client
	infra            *testinfra.AuthWebhookInfrastructure
)

// auditStoreAdapter adapts audit.AuditStore to webhooks.AuditManager interface
// This allows webhook handlers to use the real audit store
type auditStoreAdapter struct {
	store audit.AuditStore
}

func (a *auditStoreAdapter) RecordEvent(ctx context.Context, event audit.AuditEvent) error {
	// Convert audit.AuditEvent to dsgen.AuditEventRequest
	// Per DD-AUDIT-002 V2.0: Use OpenAPI types directly
	req := &dsgen.AuditEventRequest{
		Version:        event.EventVersion,
		EventTimestamp: event.EventTimestamp,
		EventType:      event.EventType,
		EventCategory:  dsgen.AuditEventRequestEventCategory(event.EventCategory),
		EventAction:    event.EventAction,
		EventOutcome:   dsgen.AuditEventRequestEventOutcome(event.EventOutcome),
		ActorType:      &event.ActorType,
		ActorId:        &event.ActorID,
		ResourceType:   &event.ResourceType,
		ResourceId:     &event.ResourceID,
		CorrelationId:  event.CorrelationID,
	}

	// Optional fields that exist in dsgen.AuditEventRequest
	if event.Namespace != nil {
		req.Namespace = event.Namespace
	}
	if event.ClusterName != nil {
		req.ClusterName = event.ClusterName
	}
	if event.Severity != nil {
		req.Severity = event.Severity
	}
	if event.DurationMs != nil {
		req.DurationMs = event.DurationMs
	}

	// Convert EventData from []byte to interface{} (will be JSON marshaled by OpenAPI client)
	if len(event.EventData) > 0 {
		var eventData interface{}
		if err := json.Unmarshal(event.EventData, &eventData); err != nil {
			return fmt.Errorf("failed to unmarshal event_data: %w", err)
		}
		req.EventData = eventData
	}

	return a.store.StoreAudit(ctx, req)
}

func TestAuthWebhookIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Integration Suite - BR-AUTH-001 SOC2 Attribution")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STEP 1: Setup Real Data Storage Infrastructure (DD-TESTING-001)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Setting up Data Storage infrastructure (PostgreSQL + Redis + Data Storage service)")
	infra = testinfra.NewAuthWebhookInfrastructure()
	infra.Setup()

	By("Initializing Data Storage OpenAPI client (DD-API-001)")
	var err error
	dsClient, err = dsgen.NewClientWithResponses(infra.GetDataStorageURL())
	if err != nil {
		Fail(fmt.Sprintf("DD-API-001 violation: Cannot proceed without DataStorage client: %v", err))
	}
	GinkgoWriter.Println("✅ Data Storage OpenAPI client initialized")

	By("Creating REAL audit store for webhook handlers")
	// Create OpenAPI DataStorage client adapter for audit writes
	dsAuditClient, err := audit.NewOpenAPIClientAdapter(infra.GetDataStorageURL(), 5*time.Second)
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
	GinkgoWriter.Println("✅ Real audit store created (connected to DataStorage)")

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STEP 2: Setup envtest + webhook server
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Bootstrapping test environment with envtest + webhook")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		// WebhookInstallOptions not needed - we register handlers programmatically below
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

	// Register WorkflowExecution mutating webhook
	wfeHandler := webhooks.NewWorkflowExecutionAuthHandler()
	_ = wfeHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})

	// Register RemediationApprovalRequest mutating webhook
	rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler()
	_ = rarHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})

	// Register NotificationRequest mutating webhook for DELETE
	// Note: Writes audit traces for DELETE attribution (K8s prevents object mutation during DELETE)
	// Uses REAL audit store to write to Data Storage (DD-TESTING-001 compliance)
	auditAdapter := &auditStoreAdapter{store: auditStore}
	nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditAdapter)
	_ = nrHandler.InjectDecoder(decoder) // InjectDecoder always returns nil
	webhookServer.Register("/mutate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})

	By("Starting webhook server")
	go func() {
		defer GinkgoRecover()
		err := webhookServer.Start(ctx)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Webhook server error: %v\n", err)
		}
	}()

	By("Waiting for webhook server to be ready")
	// Give webhook server time to start
	time.Sleep(2 * time.Second)

	By("Configuring webhook configurations in K8s API server")
	err = configureWebhooks(ctx, k8sClient, *webhookInstallOptions)
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ envtest environment ready")
	GinkgoWriter.Printf("   • Webhook server: %s:%d\n", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	GinkgoWriter.Printf("   • CertDir: %s\n", webhookInstallOptions.LocalServingCertDir)
	GinkgoWriter.Println("   • K8s client configured for CRD operations")
	GinkgoWriter.Println("   • Webhook configurations applied")
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
			{
				Name: "notificationrequest.mutate.kubernaut.ai",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL:      ptr.To(webhookURL + "/mutate-notificationrequest-delete"),
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
				SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
				TimeoutSeconds:          ptr.To(int32(10)),
			},
		},
	}

	if err := k8sClient.Create(ctx, mutatingWebhook); err != nil {
		return fmt.Errorf("failed to create MutatingWebhookConfiguration: %w", err)
	}

	// Wait for webhook configurations to be ready
	time.Sleep(1 * time.Second)

	return nil
}

var _ = AfterSuite(func() {
	By("Flushing audit store before teardown")
	if auditStore != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer flushCancel()
		err := auditStore.Flush(flushCtx)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Warning: Failed to flush audit store: %v\n", err)
		} else {
			GinkgoWriter.Println("✅ Audit store flushed")
		}
	}

	By("Tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	By("Tearing down Data Storage infrastructure")
	if infra != nil {
		infra.Teardown()
	}
})

