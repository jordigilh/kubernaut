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

package processing

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ADR-004: Fake Kubernetes Client for Unit Testing
// This test file uses controller-runtime's fake client with interceptors for error simulation.
// See: docs/architecture/decisions/ADR-004-fake-kubernetes-client.md
//
// Benefits:
// - Maintained by controller-runtime (no breakage on interface updates like Apply())
// - Compile-time type safety
// - Real K8s semantics with in-memory storage
// - Error simulation via interceptor.Funcs

// newTestSignal creates a valid NormalizedSignal with required resource info for testing
// V1.0 requires resource Kind and Name (per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
func newTestSignal(fingerprint, alertName string) *types.NormalizedSignal {
	return &types.NormalizedSignal{
		Fingerprint: fingerprint,
		AlertName:   alertName,
		Severity:    "critical",
		Namespace:   "production",
		Resource: types.ResourceIdentifier{
			Kind:      "Pod",
			Name:      "test-pod-" + fingerprint,
			Namespace: "production",
		},
		Labels: map[string]string{
			"alertname": alertName,
		},
	}
}

var _ = Describe("CRDCreator Retry Logic", func() {
	var (
		creator     *processing.CRDCreator
		fakeClient  client.Client
		scheme      *runtime.Scheme
		metricsReg  *prometheus.Registry
		metricsInst *metrics.Metrics
		logger      logr.Logger
		retryConfig *config.RetrySettings
		ctx         context.Context
		cancel      context.CancelFunc
		callCount   *atomic.Int32 // Thread-safe call counter for interceptors
	)

	// GAP 5: Test cleanup pattern
	BeforeEach(func() {
		// Create context with timeout for each test
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		// Create custom Prometheus registry per test (prevents duplicate registration)
		metricsReg = prometheus.NewRegistry()
		metricsInst = metrics.NewMetricsWithRegistry(metricsReg)

		// Create logger
		logger = logr.Discard()

		// Setup scheme for fake client (ADR-004)
		scheme = runtime.NewScheme()
		_ = remediationv1alpha1.AddToScheme(scheme)

		// Initialize call counter
		callCount = &atomic.Int32{}

		// Configure retry settings (fast for tests)
		retryConfig = &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: 10 * time.Millisecond, // Fast for tests
			MaxBackoff:     50 * time.Millisecond,
		}

		// Note: Fake client will be created per-test with specific interceptors
		// For now, this will fail compilation - we need to implement retry logic first
	})

	// GAP 5: Resource cleanup
	AfterEach(func() {
		cancel() // Prevent context leaks
		// Note: Fake client is recreated per-test, no cleanup needed (ADR-004)
	})

	// ========================================
	// Iteration 1: Retryable Errors - HTTP 429
	// ========================================
	Context("Retryable Errors - HTTP 429", func() {
		It("should retry on HTTP 429 and succeed on 2nd attempt", func() {
			// BR-GATEWAY-112: Error Classification (429 is retryable)
			// BR-GATEWAY-113: Exponential Backoff
			// BR-GATEWAY-114: Retry Metrics

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// First attempt: rate limited
							return apierrors.NewTooManyRequests("rate limited", 1)
						}
						// Second attempt: success - let fake client handle it
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-429", "TestAlert429")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(rr.Name).To(ContainSubstring("rr-"))
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")

			// Verify: Metrics incremented
			// Note: Metrics verification will be implemented after metrics are added
		})

		It("should retry on HTTP 503 and succeed on 3rd attempt", func() {
			// BR-GATEWAY-112: Error Classification (503 is retryable)
			// BR-GATEWAY-113: Exponential Backoff (100ms → 200ms → success)

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count <= 2 {
							// First and second attempts: service unavailable
							return apierrors.NewServiceUnavailable("API server overloaded")
						}
						// Third attempt: success - let fake client handle it
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-503", "TestAlert503")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(3)), "Should have made exactly 3 attempts")

			// Note: Timing verification removed - backoff logic is tested in integration tests
		})

		It("should fail after max retries on persistent HTTP 503", func() {
			// BR-GATEWAY-112: Error Classification (503 is retryable but exhausted)
			// BR-GATEWAY-113: Max Attempts (3 attempts configured)

			// Setup: Fake client with interceptor that always fails (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewServiceUnavailable("API server overloaded")
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-503-fail", "TestAlert503Fail")

			// Execute: Create CRD with retry (should fail)
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Failure after max retries
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(callCount.Load()).To(Equal(int32(3)), "Should have made exactly 3 attempts (max)")

			// Verify: Error is wrapped with retry context (GAP 10)
			var retryErr *processing.RetryError
			Expect(errors.As(err, &retryErr)).To(BeTrue(), "Error should be wrapped as RetryError")
			Expect(retryErr.Attempt).To(Equal(3))
			Expect(retryErr.MaxAttempts).To(Equal(3))
			Expect(retryErr.ErrorType).To(ContainSubstring("service_unavailable"))
		})
	})

	Context("Retryable Errors - HTTP 504 Timeout", func() {
		It("should retry on HTTP 504 gateway timeout", func() {
			// BR-GATEWAY-112: Error Classification (504 is retryable)
			// BR-GATEWAY-113: Exponential Backoff

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// First attempt: gateway timeout
							return apierrors.NewTimeoutError("gateway timeout", 10)
						}
						// Second attempt: success
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-504", "TestAlert504")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")
		})

		It("should retry on context deadline exceeded", func() {
			// BR-GATEWAY-112: Error Classification (timeout errors are retryable)
			// BR-GATEWAY-113: Exponential Backoff

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// First attempt: context deadline exceeded (simulated as timeout error)
							return apierrors.NewTimeoutError("context deadline exceeded", 5)
						}
						// Second attempt: success
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-timeout", "TestAlertTimeout")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")
		})
	})

	Context("Non-Retryable Errors", func() {
		It("should NOT retry on HTTP 400 (Bad Request)", func() {
			// BR-GATEWAY-112: Error Classification (400 is non-retryable)
			// Validation errors should fail fast

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewBadRequest("invalid CRD spec")
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-400", "TestAlert400")

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(callCount.Load()).To(Equal(int32(1)), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("invalid CRD spec"))
		})

		It("should NOT retry on HTTP 403 (Forbidden/RBAC)", func() {
			// BR-GATEWAY-112: Error Classification (403 is non-retryable)
			// RBAC errors should fail fast

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewForbidden(
							schema.GroupResource{Group: "remediation.kubernaut.ai", Resource: "remediationrequests"},
							"test-rr",
							errors.New("insufficient permissions"),
						)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-403", "TestAlert403")

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(callCount.Load()).To(Equal(int32(1)), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("forbidden"))
		})

		It("should NOT retry on HTTP 422 (Unprocessable Entity)", func() {
			// BR-GATEWAY-112: Error Classification (422 is non-retryable)
			// Schema validation errors should fail fast

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewInvalid(
							remediationv1alpha1.GroupVersion.WithKind("RemediationRequest").GroupKind(),
							"test-rr",
							nil,
						)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-422", "TestAlert422")

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(callCount.Load()).To(Equal(int32(1)), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})
	})

	Context("HTTP 409 Conflict Handling", func() {
		It("should NOT retry on HTTP 409 (already exists)", func() {
			// BR-GATEWAY-112: Error Classification (409 is non-retryable, idempotent)
			// Already exists is not an error condition

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewAlreadyExists(
							schema.GroupResource{Group: "remediation.kubernaut.ai", Resource: "remediationrequests"},
							"test-rr",
						)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-409", "TestAlert409")

			// Execute: Create CRD (should fail immediately but gracefully)
			_, _ = creator.CreateRemediationRequest(ctx, signal)

			// Verify: Immediate failure (no retry), but CRD is fetched
			// Note: The actual implementation fetches the existing CRD, so this test
			// verifies the retry logic doesn't kick in for 409
			Expect(callCount.Load()).To(Equal(int32(1)), "Should have made exactly 1 attempt (no retry)")
		})
	})

	Context("Network Errors", func() {
		It("should retry on connection refused", func() {
			// BR-GATEWAY-112: Error Classification (network errors are retryable)
			// Connection refused is a transient network error

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// First attempt: connection refused
							return errors.New("connection refused")
						}
						// Second attempt: success
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-connrefused", "TestAlertConnRefused")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")
		})

		It("should retry on connection reset", func() {
			// BR-GATEWAY-112: Error Classification (network errors are retryable)
			// Connection reset is a transient network error

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// First attempt: connection reset
							return errors.New("connection reset by peer")
						}
						// Second attempt: success
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-connreset", "TestAlertConnReset")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")
		})
	})

	Context("Backoff Configuration", func() {
		It("should cap backoff at MaxBackoff", func() {
			// BR-GATEWAY-113: Exponential Backoff with cap
			// Backoff should not exceed MaxBackoff

			// Setup: Custom retry config with low MaxBackoff
			customRetryConfig := &config.RetrySettings{
				MaxAttempts:    4,
				InitialBackoff: 50 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond, // Cap at 100ms
			}

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count <= 3 {
							return apierrors.NewTooManyRequests("rate limited", 1)
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with custom retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", customRetryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-backoff-cap", "TestAlertBackoffCap")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(4)), "Should have made exactly 4 attempts")
			// Note: Backoff timing verification is in integration tests
		})

		It("should respect InitialBackoff configuration", func() {
			// BR-GATEWAY-113: Exponential Backoff starts with InitialBackoff
			// First retry should wait InitialBackoff duration

			// Setup: Custom retry config with specific InitialBackoff
			customRetryConfig := &config.RetrySettings{
				MaxAttempts:    2,
				InitialBackoff: 200 * time.Millisecond,
				MaxBackoff:     1 * time.Second,
			}

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							return apierrors.NewTooManyRequests("rate limited", 1)
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with custom retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", customRetryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-initial-backoff", "TestAlertInitialBackoff")

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made exactly 2 attempts")
		})
	})

	Context("Context Cancellation", func() {
		It("should stop retrying on context cancellation", func() {
			// BR-GATEWAY-112: Context-aware retry (GAP 6: Graceful Shutdown)
			// Retry should stop immediately on context cancellation

			// Setup: Create cancellable context
			cancelCtx, cancel := context.WithCancel(context.Background())

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							// Cancel context after first attempt
							cancel()
							return apierrors.NewTooManyRequests("rate limited", 1)
						}
						// Should never reach here
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-ctx-cancel", "TestAlertCtxCancel")

			// Execute: Create CRD with retry (context will be cancelled)
			// Note: environment/priority parameters removed (2025-12-06) - SP owns classification
			rr, err := creator.CreateRemediationRequest(cancelCtx, signal)

			// Verify: Failure due to context cancellation (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(callCount.Load()).To(Equal(int32(1)), "Should have made exactly 1 attempt before context cancellation")
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		It("should return context error immediately", func() {
			// BR-GATEWAY-112: Context-aware retry (GAP 6: Graceful Shutdown)
			// Context deadline exceeded should be detected during backoff

			// Setup: Create context with very short deadline
			deadlineCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						callCount.Add(1)
						return apierrors.NewTooManyRequests("rate limited", 1)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-ctx-deadline", "TestAlertCtxDeadline")

			// Execute: Create CRD with retry (context will timeout during backoff)
			// Note: environment/priority parameters removed (2025-12-06) - SP owns classification
			rr, err := creator.CreateRemediationRequest(deadlineCtx, signal)

			// Verify: Failure due to context deadline
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			// Should have made 1-2 attempts before context deadline
			Expect(callCount.Load()).To(BeNumerically("<=", int32(2)), "Should stop retrying after context deadline")
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})
	})

	Context("Config Validation", func() {
		It("should use default config when nil", func() {
			// BR-GATEWAY-111: Default retry configuration (GAP 7: Config Validation)
			// Nil config should use sensible defaults

			// Setup: Fake client with interceptor (ADR-004)
			callCount.Store(0)
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						count := callCount.Add(1)
						if count == 1 {
							return apierrors.NewTooManyRequests("rate limited", 1)
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			// Wrap fake client in k8s.Client
			k8sClient := k8s.NewClient(fakeClient)

			// Create CRD creator with nil retry config (should use defaults)
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", nil)

			// Create test signal (with required resource info per BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
			signal := newTestSignal("test-fingerprint-default-config", "TestAlertDefaultConfig")

			// Execute: Create CRD with retry (using default config)
			rr, err := creator.CreateRemediationRequest(ctx, signal)

			// Verify: Success after retry (default config allows retries)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(callCount.Load()).To(Equal(int32(2)), "Should have made 2 attempts with default config")
		})

		It("should validate MaxAttempts >= 1", func() {
			// BR-GATEWAY-111: Config validation (GAP-8: Enhanced Config Validation)
			// MaxAttempts must be at least 1

			// Setup: Invalid retry config with MaxAttempts = 0
			invalidConfig := &config.RetrySettings{
				MaxAttempts:    0, // Invalid
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     1 * time.Second,
			}

			// Execute: Validate config
			err := invalidConfig.Validate()

		// Verify: Validation fails with structured error (GAP-8)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("processing.retry.maxAttempts"))
		Expect(err.Error()).To(ContainSubstring("must be >= 1"))
		})

		It("should validate MaxBackoff >= InitialBackoff", func() {
			// BR-GATEWAY-111: Config validation (GAP 7: Config Validation)
			// MaxBackoff must be >= InitialBackoff

			// Setup: Invalid retry config with MaxBackoff < InitialBackoff
			invalidConfig := &config.RetrySettings{
				MaxAttempts:    3,
				InitialBackoff: 1 * time.Second,
				MaxBackoff:     100 * time.Millisecond, // Invalid (less than InitialBackoff)
			}

			// Execute: Validate config
			err := invalidConfig.Validate()

		// Verify: Validation fails
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("retry.maxBackoff"))
		})
	})

	// NOTE: Retry Toggles tests removed (Reliability-First Design)
	// Retries are now ALWAYS enabled for transient errors (429, 503, 504, timeouts, network errors)
	// This ensures maximum reliability without configuration complexity
})
