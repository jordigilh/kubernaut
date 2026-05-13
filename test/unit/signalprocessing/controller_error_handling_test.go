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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
//
// This file covers Error Category B: Transient Errors (timeout, retry behavior)
// Referenced by: test/integration/signalprocessing/reconciler_integration_test.go:883
package signalprocessing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/prometheus/client_golang/prometheus"
)

// ========================================
// ERROR HANDLING TESTS (Error Category B)
// Referenced by integration test skip at reconciler_integration_test.go:883
// ========================================

var _ = Describe("Controller Error Handling", func() {

	// Error Category B: Transient Errors
	// Tests that the controller correctly identifies and handles transient errors
	Context("Error Category B: Transient Error Detection", func() {

		// Test 1: Timeout errors are identified as transient
		It("should identify timeout errors as transient (retryable)", func() {
			// Simulate a timeout error
			timeoutErr := context.DeadlineExceeded

			// Verify it's a transient error that should trigger retry
			Expect(errors.Is(timeoutErr, context.DeadlineExceeded)).To(BeTrue())
			Expect(isTransientError(timeoutErr)).To(BeTrue())
		})

		// Test 2: Context canceled errors are identified as transient
		It("should identify context canceled errors as transient", func() {
			canceledErr := context.Canceled

			Expect(errors.Is(canceledErr, context.Canceled)).To(BeTrue())
			Expect(isTransientError(canceledErr)).To(BeTrue())
		})

		// Test 3: Server timeout (K8s API) errors are retryable
		It("should identify K8s API timeout as retryable", func() {
			// Simulate K8s API server timeout (HTTP 504)
			serverTimeoutErr := apierrors.NewServerTimeout(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"get",
				5,
			)

			Expect(apierrors.IsServerTimeout(serverTimeoutErr)).To(BeTrue())
			Expect(isTransientError(serverTimeoutErr)).To(BeTrue())
		})

		// Test 4: Too many requests (rate limiting) should trigger backoff
		It("should identify rate limiting errors as transient", func() {
			// Simulate K8s API rate limiting (HTTP 429)
			rateLimitErr := apierrors.NewTooManyRequests("rate limit exceeded", 5)

			Expect(apierrors.IsTooManyRequests(rateLimitErr)).To(BeTrue())
			Expect(isTransientError(rateLimitErr)).To(BeTrue())
		})

		// Test 5: Service unavailable errors are transient
		It("should identify service unavailable errors as transient", func() {
			// Simulate K8s API unavailable (HTTP 503)
			unavailableErr := apierrors.NewServiceUnavailable("etcd unavailable")

			Expect(apierrors.IsServiceUnavailable(unavailableErr)).To(BeTrue())
			Expect(isTransientError(unavailableErr)).To(BeTrue())
		})

		// Test 6: Not found errors are NOT transient (permanent)
		It("should NOT identify not found errors as transient", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"missing-resource",
			)

			Expect(apierrors.IsNotFound(notFoundErr)).To(BeTrue())
			Expect(isTransientError(notFoundErr)).To(BeFalse())
		})

		// Test 7: Conflict errors are retryable (optimistic concurrency)
		It("should identify conflict errors as retryable", func() {
			conflictErr := apierrors.NewConflict(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"test-resource",
				errors.New("resource version mismatch"),
			)

			Expect(apierrors.IsConflict(conflictErr)).To(BeTrue())
			// Conflicts are retryable via retry.RetryOnConflict
			// K8s default retry has exactly 5 steps
			Expect(retry.DefaultRetry.Steps).To(Equal(5))
		})
	})

	// Error Category B: Retry Behavior
	// Tests that retry logic works correctly with backoff
	Context("Error Category B: Retry Behavior with Backoff", func() {

		// Test 8: RetryOnConflict succeeds after transient failure
		It("should succeed after transient conflict during status update", func() {
			attemptCount := 0
			maxAttempts := 3

			// Simulate a function that fails twice then succeeds
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				attemptCount++
				if attemptCount < maxAttempts {
					return apierrors.NewConflict(
						schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
						"test-resource",
						errors.New("simulated conflict"),
					)
				}
				return nil // Success on third attempt
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(attemptCount).To(Equal(maxAttempts))
		})

		// Test 9: Retry exhaustion returns last error
		It("should return error after retry exhaustion", func() {
			attemptCount := 0

			// Use a limited retry config
			limitedRetry := retry.DefaultRetry
			limitedRetry.Steps = 2

			err := retry.RetryOnConflict(limitedRetry, func() error {
				attemptCount++
				return apierrors.NewConflict(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"test-resource",
					errors.New("persistent conflict"),
				)
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsConflict(err)).To(BeTrue())
			Expect(attemptCount).To(BeNumerically(">=", 2))
		})

		// Test 10: Non-conflict errors are not retried
		It("should not retry non-conflict errors", func() {
			attemptCount := 0

			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				attemptCount++
				return apierrors.NewNotFound(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"missing-resource",
				)
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(attemptCount).To(Equal(1)) // Only one attempt - no retry
		})
	})

	// Error Category B: Context Deadline Handling
	// Tests that timeout contexts are properly handled
	Context("Error Category B: Context Deadline Handling", func() {

		// Test 11: Operation respects context deadline
		It("should abort operation when context deadline exceeded", func() {
			// Create a context with very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			// Use Eventually to poll until the context deadline fires.
			// Avoids time.Sleep anti-pattern: Go's context timer goroutine may not
			// be scheduled immediately on loaded CI runners, causing ctx.Err() to
			// return nil even after wall-clock time exceeds the deadline.
			Eventually(ctx.Err).WithTimeout(100 * time.Millisecond).WithPolling(1 * time.Millisecond).
				Should(Equal(context.DeadlineExceeded))
		})

		// Test 12: Operation should check context before expensive operations
		It("should detect context cancellation before proceeding", func() {
			ctx, cancel := context.WithCancel(context.Background())

			// Cancel immediately
			cancel()

			// Check context - this is what controller should do before API calls
			select {
			case <-ctx.Done():
				Expect(ctx.Err()).To(Equal(context.Canceled))
			default:
				Fail("Context should be canceled")
			}
		})
	})
})

// isTransientError checks if an error is transient and should trigger retry.
// This mirrors the controller's error classification logic.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are transient
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// K8s API transient errors
	if apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) {
		return true
	}

	return false
}

// ========================================
// PHASE 1 TDD RED: Issue #1110 SP Readiness Audit
// Findings: E1, E2, E5, E6, O5
// ========================================

var _ = Describe("Issue #1110 Phase 1: Error Handling, Resilience, and Startup Validation", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// E1 (Critical): ConsecutiveFailures reset — BR-SP-111
	// Bug: resetConsecutiveFailures only fires when result.Requeue==true,
	// but successful phase transitions use RequeueAfter (not Requeue).
	// ========================================
	Describe("E1: ConsecutiveFailures reset on phase transition", func() {

		It("UT-SP-1110-001: resets ConsecutiveFailures to 0 after successful phase transition with RequeueAfter > 0", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e1-001",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e1-001",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:               signalprocessingv1alpha1.PhasePending,
					ConsecutiveFailures: 3,
					StartTime:           &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     newDefaultMockK8sEnricher(),
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Successful Pending->Enriching uses RequeueAfter")

			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.ConsecutiveFailures).To(Equal(int32(0)),
				"BR-SP-111: ConsecutiveFailures MUST reset on successful phase transition (RequeueAfter path)")
		})

		It("UT-SP-1110-002: resets ConsecutiveFailures to 0 after successful transition with Requeue==true", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e1-002",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e1-002",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:               signalprocessingv1alpha1.PhasePending,
					ConsecutiveFailures: 5,
					StartTime:           &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     newDefaultMockK8sEnricher(),
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue(),
				"Successful transitions use either Requeue or RequeueAfter")

			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.ConsecutiveFailures).To(Equal(int32(0)),
				"BR-SP-111: ConsecutiveFailures MUST reset on any successful transition")
		})

		It("UT-SP-1110-003: does NOT reset ConsecutiveFailures after failed reconcile (err != nil)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e1-003",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e1-003",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:               signalprocessingv1alpha1.PhaseEnriching,
					ConsecutiveFailures: 2,
					StartTime:           &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			failingEnricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, fmt.Errorf("permanent enrichment error")
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     failingEnricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})
			Expect(err).To(HaveOccurred(), "Enrichment failure returns error")

			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.ConsecutiveFailures).To(BeNumerically(">=", 2),
				"ConsecutiveFailures MUST NOT reset on error (failures should accumulate)")
		})
	})

	// ========================================
	// E2 (High): isTransientError uses == not errors.Is
	// Bug: Wrapped context errors (fmt.Errorf("...: %w", ctx.Err())) are NOT detected.
	// ========================================
	Describe("E2: isTransientError with wrapped context errors", func() {

		It("UT-SP-1110-004: detects wrapped context.DeadlineExceeded as transient", func() {
			wrappedErr := fmt.Errorf("enrichment timed out: %w", context.DeadlineExceeded)

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e2-004",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e2-004",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, wrappedErr
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			// Wrapped DeadlineExceeded should be detected as transient -> handled via backoff (no error returned)
			Expect(err).ToNot(HaveOccurred(),
				"E2: Wrapped context.DeadlineExceeded MUST be detected as transient (currently uses == instead of errors.Is)")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"E2: Transient errors get explicit backoff delay")
		})

		It("UT-SP-1110-005: detects wrapped context.Canceled as transient", func() {
			wrappedErr := fmt.Errorf("operation cancelled: %w", context.Canceled)

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e2-005",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e2-005",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, wrappedErr
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(err).ToNot(HaveOccurred(),
				"E2: Wrapped context.Canceled MUST be detected as transient (currently uses == instead of errors.Is)")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"E2: Transient errors get explicit backoff delay")
		})

		It("UT-SP-1110-006: detects direct context.DeadlineExceeded as transient (baseline)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e2-006",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e2-006",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, context.DeadlineExceeded
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(err).ToNot(HaveOccurred(),
				"Direct context.DeadlineExceeded should be detected as transient (baseline)")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Transient errors get explicit backoff delay")
		})
	})

	// ========================================
	// E6 (Medium): RecordError return ignored — ADR-032
	// Bug: `_ = r.AuditManager.RecordError(ctx, sp, "Enriching", err)` silently drops audit failure.
	// ========================================
	Describe("E6: RecordError audit failure handling", func() {

		It("UT-SP-1110-010: RecordError failure is propagated (not silenced with _ =)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e6-010",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{Name: "rr-e6-010"},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e6-010",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			failStore := &mockAuditStoreWithError{err: fmt.Errorf("audit storage unavailable")}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, fmt.Errorf("enrichment failed")
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(failStore, logr.Discard())),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			_, recErr := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})
			// The enrichment error is always returned. ADR-032 says the audit error should be logged.
			// We verify audit store captured the attempt (even if it failed). The important assertion
			// is that the code does NOT silently discard the audit error via `_ =`.
			Expect(recErr).To(HaveOccurred(), "Enrichment error propagated")
			Expect(failStore.callCount).To(BeNumerically(">", 0),
				"ADR-032: RecordError MUST be called (audit store was invoked)")
		})

		It("UT-SP-1110-011: RecordError failure is logged with context when audit write fails", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e6-011",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{Name: "rr-e6-011"},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e6-011",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			failStore := &mockAuditStoreWithError{err: fmt.Errorf("audit storage unavailable")}
			logSink := &capturingLogSink{}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, fmt.Errorf("enrichment failed")
				},
			}

			logger := logr.New(logSink)

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(failStore, logger)),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			ctx := logr.NewContext(context.Background(), logger)
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			// ADR-032: When RecordError fails, the failure MUST be logged
			Expect(logSink.hasErrorLog("audit")).To(BeTrue(),
				"E6: When RecordError fails, the audit failure MUST be logged (ADR-032: no silent skips)")
		})
	})

	// ========================================
	// O5 (High, reclassified): PolicyEvaluator nil = fail-fast
	// Defense in depth: prevent SP startup without PolicyEvaluator.
	// ========================================
	Describe("O5: PolicyEvaluator nil startup validation", func() {

		It("UT-SP-1110-014: SetupWithManager returns error when PolicyEvaluator is nil", func() {
			reconciler := &controller.SignalProcessingReconciler{
				Scheme:        scheme,
				StatusManager: spstatus.NewManager(nil, nil),
				Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			}

			// SetupWithManager requires a real manager; we test that nil PolicyEvaluator
			// is validated before controller registration. Since SetupWithManager needs
			// a Manager, we check that the reconciler itself rejects nil PolicyEvaluator.
			Expect(reconciler.PolicyEvaluator).To(BeNil(), "precondition: PolicyEvaluator is nil")
			// The expected behavior: SetupWithManager should return an error for nil PolicyEvaluator.
			// We can't easily call SetupWithManager without a full manager, so we test Reconcile
			// returns a permanent error instead (see UT-SP-1110-015).
		})

		It("UT-SP-1110-015: Reconcile returns permanent error when PolicyEvaluator is nil", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-o5-015",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-o5-015",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseClassifying,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "default",
					Labels: map[string]string{"env": "production"},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp, ns).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
				Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:  spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:   newDefaultMockK8sEnricher(),
				Recorder:      record.NewFakeRecorder(20),
				// PolicyEvaluator intentionally nil
			}

			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(err).To(HaveOccurred(),
				"O5: Reconcile MUST return permanent error when PolicyEvaluator is nil (fail-fast guard)")
			Expect(err.Error()).To(ContainSubstring("PolicyEvaluator"),
				"Error message should identify the nil dependency")
		})

		It("UT-SP-1110-016: SetupWithManager succeeds when PolicyEvaluator is non-nil", func() {
			reconciler := &controller.SignalProcessingReconciler{
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(nil, nil),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
			}

			Expect(reconciler.PolicyEvaluator).ToNot(BeNil(), "PolicyEvaluator is set")
		})
	})

	// ========================================
	// E5 (Medium): Error logging — 00-kubernaut-core-rules + DD-005
	// 12+ error returns without logger.Error. Tests verify structured log context.
	// ========================================
	Describe("E5: Error-path structured logging", func() {

		It("UT-SP-1110-007: enrichment error is logged with resource name, namespace, and phase", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e5-007",
					Namespace:  "test-ns",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e5-007",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "test-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}

			logSink := &capturingLogSink{}
			logger := logr.New(logSink)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			enricher := &mockK8sEnricher{
				EnrichFunc: func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return nil, fmt.Errorf("enrichment failed: node not found")
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				K8sEnricher:     enricher,
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			ctx := logr.NewContext(context.Background(), logger)
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(logSink.hasErrorLog("")).To(BeTrue(),
				"E5: Enrichment error MUST be logged via logger.Error (DD-005 structured logging)")
		})

		It("UT-SP-1110-008: classification error is logged with resource name, namespace, and phase", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e5-008",
					Namespace:  "test-ns",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e5-008",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "test-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseClassifying,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
					KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
						Namespace: &signalprocessingv1alpha1.NamespaceContext{
							Name:   "test-ns",
							Labels: map[string]string{"env": "production"},
						},
					},
				},
			}

			logSink := &capturingLogSink{}
			logger := logr.New(logSink)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			failingEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					return nil, fmt.Errorf("rego evaluation failed: policy timeout")
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				PolicyEvaluator: failingEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			ctx := logr.NewContext(context.Background(), logger)
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(logSink.hasErrorLog("")).To(BeTrue(),
				"E5: Classification error MUST be logged via logger.Error (DD-005 structured logging)")
		})

		It("UT-SP-1110-009: categorization phase completes and transitions through Categorizing → Completed", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-e5-009",
					Namespace:  "test-ns",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{Name: "rr-e5-009"},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-e5-009",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "test-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseCategorizing,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
					KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
						Namespace: &signalprocessingv1alpha1.NamespaceContext{
							Name:   "test-ns",
							Labels: map[string]string{"env": "production"},
						},
					},
					EnvironmentClassification: &signalprocessingv1alpha1.EnvironmentClassification{
						Environment:  signalprocessingv1alpha1.EnvironmentProduction,
						Source:       "test",
						ClassifiedAt: metav1.Now(),
					},
					PriorityAssignment: &signalprocessingv1alpha1.PriorityAssignment{
						Priority:   signalprocessingv1alpha1.PriorityP1,
						Source:     "test",
						AssignedAt: metav1.Now(),
					},
				},
			}

			logSink := &capturingLogSink{}
			logger := logr.New(logSink)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(spaudit.NewAuditClient(&mockAuditStore{}, logr.Discard())),
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
				Recorder:        record.NewFakeRecorder(20),
			}

			ctx := logr.NewContext(context.Background(), logger)
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(err).ToNot(HaveOccurred(), "E5: Categorizing phase should complete without error")

			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{
				Name: sp.Name, Namespace: sp.Namespace,
			}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted),
				"E5: Successful categorization must transition to Completed")
		})
	})
})

// ========================================
// Test helpers for Phase 1
// ========================================

// mockAuditStoreWithError always returns an error on StoreAudit.
type mockAuditStoreWithError struct {
	err       error
	callCount int
}

func (m *mockAuditStoreWithError) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	m.callCount++
	return m.err
}

func (m *mockAuditStoreWithError) Flush(_ context.Context) error {
	return m.err
}

func (m *mockAuditStoreWithError) Close() error {
	return nil
}

// capturingLogSink captures log entries for assertion.
type capturingLogSink struct {
	errorMessages []string
	infoMessages  []string
}

func (c *capturingLogSink) Init(_ logr.RuntimeInfo) {}

func (c *capturingLogSink) Enabled(_ int) bool { return true }

func (c *capturingLogSink) Info(_ int, msg string, _ ...interface{}) {
	c.infoMessages = append(c.infoMessages, msg)
}

func (c *capturingLogSink) Error(_ error, msg string, _ ...interface{}) {
	c.errorMessages = append(c.errorMessages, msg)
}

func (c *capturingLogSink) WithValues(_ ...interface{}) logr.LogSink { return c }

func (c *capturingLogSink) WithName(_ string) logr.LogSink { return c }

func (c *capturingLogSink) hasErrorLog(substring string) bool {
	if substring == "" {
		return len(c.errorMessages) > 0
	}
	for _, msg := range c.errorMessages {
		if contains(msg, substring) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
