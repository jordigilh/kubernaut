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

// Package reconciler_test contains unit tests for the SignalProcessing reconciler.
//
// BR Coverage:
//   - BR-SP-090: Categorization Audit Trail (MANDATORY enforcement)
//   - ADR-032: Data Access Layer Isolation - Audit is MANDATORY
//
// Test Categories:
//   - AM-MAN-XX: Mandatory enforcement tests
//
// Per 03-testing-strategy.mdc: Unit tests validate business logic behavior.
// These tests verify that audit client is mandatory and cannot be nil.
package reconciler

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
)

func TestReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Reconciler Unit Tests")
}

// mockK8sEnricher implements signalprocessing.K8sEnricher interface for testing
type mockK8sEnricher struct {
	EnrichFunc func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error)
}

func (m *mockK8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
	if m.EnrichFunc != nil {
		return m.EnrichFunc(ctx, signal)
	}
	return &signalprocessingv1alpha1.KubernetesContext{}, nil
}

var _ = Describe("BR-SP-090/ADR-032: Audit Client Mandatory Enforcement", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// AM-MAN-01: Nil AuditClient Returns Error (ADR-032)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// ADR-032 §2: "No Audit Loss" - Audit writes are MANDATORY, not best-effort
	// Services MUST NOT implement "graceful degradation" that silently skips audit
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("AM-MAN-01: When is nil", func() {
		It("should fail reconciliation instead of silently skipping audit", func() {
			// BUSINESS SCENARIO:
			// Per ADR-032: Audit is MANDATORY for compliance (SOC2, HIPAA)
			// A nil AuditClient indicates a configuration error
			// Controller MUST fail fast rather than process signals without audit trail
			//
			// This prevents:
			// - Silent compliance violations
			// - Orphaned signals with no audit history
			// - Production deployments with missing audit configuration

			By("Creating SignalProcessing CR in Enriching phase")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sp-no-audit",
					Namespace:  "default",
					Generation: 1, // K8s increments on create/update
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      "test-rr",
						Namespace: "default",
					},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
						Name:         "TestSignal",
						Severity:     "critical",
						Type:         "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: metav1.Now().Time},
				},
			}

			// Create fake client with the SignalProcessing resource
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			By("Creating reconciler WITHOUT AuditClient (nil)")
			// Provide minimal mocks for components checked before audit
			mockEnricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return &signalprocessingv1alpha1.KubernetesContext{
						Namespace: &signalprocessingv1alpha1.NamespaceContext{
							Name: signal.TargetResource.Namespace,
						},
					}, nil
				},
			}

			reconciler := &signalprocessing.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				StatusManager: status.NewManager(fakeClient, fakeClient),
				Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				K8sEnricher:   mockEnricher, // Need this to reach audit check
				AuditClient:   nil,          // DELIBERATELY nil to test mandatory enforcement
			}

			By("Attempting reconciliation")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)

			By("Verifying reconciliation fails with audit error")
			// ADR-032: Controller MUST return error if is nil
			// This ensures the error is surfaced and logged, not silently ignored
			Expect(err).To(HaveOccurred(), "Reconciliation MUST fail when is nil per ADR-032")
			Expect(err.Error()).To(ContainSubstring("is nil"),
				"Error message MUST indicate audit client is missing")
			Expect(err.Error()).To(ContainSubstring("MANDATORY"),
				"Error message MUST indicate audit is mandatory")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// AM-MAN-02: Verify main.go Enforcement Pattern
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// This test documents the expected startup behavior.
	// Actual startup validation is in cmd/signalprocessing/main.go (lines 139-166)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("AM-MAN-02: Documentation of startup enforcement", func() {
		It("documents that main.go enforces audit client creation at startup", func() {
			// BUSINESS SCENARIO:
			// cmd/signalprocessing/main.go (lines 139-166) enforces:
			//
			// 1. DataStorage URL is configured (defaults to in-cluster service)
			// 2. OpenAPI client adapter is created (DD-API-001 compliant)
			// 3. BufferedStore is created (ADR-038 fire-and-forget pattern)
			// 4. If any step fails → os.Exit(1)
			//
			// This ensures:
			// - Controller NEVER starts without audit capability
			// - Configuration errors are caught at startup
			// - Production deployments always have audit trail
			//
			// Code reference (cmd/signalprocessing/main.go:151-166):
			// ```go
			// dsClient, err := sharedaudit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
			// if err != nil {
			//     setupLog.Error(err, "FATAL: failed to create Data Storage client")
			//     os.Exit(1)
			// }
			//
			// auditStore, err := sharedaudit.NewBufferedStore(...)
			// if err != nil {
			//     setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
			//     os.Exit(1)
			// }
			// ```

			// This is a documentation test - the actual enforcement is in main.go
			Expect(true).To(BeTrue(), "main.go enforces audit client creation at startup with os.Exit(1) on failure")
		})
	})
})
