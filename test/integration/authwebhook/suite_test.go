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
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/webhooks"

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
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAuthWebhookIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Integration Suite - BR-AUTH-001 SOC2 Attribution")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Bootstrapping test environment with envtest + webhook")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		// WebhookInstallOptions not needed - we register handlers programmatically below
	}

	var err error
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
	// Note: Must be mutating (not validating) because we need to add annotations
	nrHandler := webhooks.NewNotificationRequestDeleteHandler()
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
	// Read CA cert from webhook server
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(webhookOpts.LocalServingCertDir, "tls.crt"),
		filepath.Join(webhookOpts.LocalServingCertDir, "tls.key"),
	)
	if err != nil {
		return fmt.Errorf("failed to load webhook certificates: %w", err)
	}

	// Get CA bundle (the cert itself in self-signed case)
	caBundle := cert.Certificate[0]

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
							APIGroups:   []string{"workflowexecution.kubernaut.ai"},
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
							APIGroups:   []string{"remediation.kubernaut.ai"},
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
							APIGroups:   []string{"notification.kubernaut.ai"},
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
	By("Tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

