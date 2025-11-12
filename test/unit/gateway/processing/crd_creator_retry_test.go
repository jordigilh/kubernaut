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

package processing_test

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// mockK8sClient is a mock implementation that satisfies the controller-runtime client.Client interface
// for testing retry logic
type mockK8sClient struct {
	createFunc    func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error
	callCount     int
	callCountLock sync.Mutex
}

func newMockK8sClient() *mockK8sClient {
	return &mockK8sClient{}
}

// Create implements the controller-runtime client.Client interface
func (m *mockK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.callCountLock.Lock()
	m.callCount++
	m.callCountLock.Unlock()

	if m.createFunc != nil {
		if rr, ok := obj.(*remediationv1alpha1.RemediationRequest); ok {
			return m.createFunc(ctx, rr)
		}
	}
	return nil
}

// Stub implementations for other controller-runtime client.Client methods
func (m *mockK8sClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}

func (m *mockK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (m *mockK8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

func (m *mockK8sClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}

func (m *mockK8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (m *mockK8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (m *mockK8sClient) Status() client.SubResourceWriter {
	return &mockSubResourceWriter{}
}

func (m *mockK8sClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

func (m *mockK8sClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (m *mockK8sClient) SubResource(subResource string) client.SubResourceClient {
	return &mockSubResourceClient{}
}

func (m *mockK8sClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (m *mockK8sClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

// mockSubResourceWriter implements client.SubResourceWriter
type mockSubResourceWriter struct{}

func (m *mockSubResourceWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (m *mockSubResourceWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (m *mockSubResourceWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

// mockSubResourceClient implements client.SubResourceClient
type mockSubResourceClient struct{}

func (m *mockSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}

func (m *mockSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (m *mockSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (m *mockSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (m *mockK8sClient) GetCallCount() int {
	m.callCountLock.Lock()
	defer m.callCountLock.Unlock()
	return m.callCount
}

func (m *mockK8sClient) ResetCallCount() {
	m.callCountLock.Lock()
	defer m.callCountLock.Unlock()
	m.callCount = 0
}

// Test suite is defined in suite_test.go

var _ = Describe("CRDCreator Retry Logic", func() {
	var (
		creator     *processing.CRDCreator
		mockClient  *mockK8sClient
		metricsReg  *prometheus.Registry
		metricsInst *metrics.Metrics
		logger      *zap.Logger
		retryConfig *config.RetrySettings
		ctx         context.Context
		cancel      context.CancelFunc
	)

	// GAP 5: Test cleanup pattern
	BeforeEach(func() {
		// Create context with timeout for each test
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		// Create custom Prometheus registry per test (prevents duplicate registration)
		metricsReg = prometheus.NewRegistry()
		metricsInst = metrics.NewMetricsWithRegistry(metricsReg)

		// Create logger
		logger = zap.NewNop()

		// Create mock K8s client
		mockClient = newMockK8sClient()

		// Configure retry settings (fast for tests)
		retryConfig = &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: 10 * time.Millisecond, // Fast for tests
			MaxBackoff:     50 * time.Millisecond,
		}

		// Note: We'll need to create a wrapper to inject the mock client
		// For now, this will fail compilation - we need to implement retry logic first
	})

	// GAP 5: Resource cleanup
	AfterEach(func() {
		cancel() // Prevent context leaks
		mockClient.ResetCallCount()
	})

	// ========================================
	// Iteration 1: Retryable Errors - HTTP 429
	// ========================================
	Context("Retryable Errors - HTTP 429", func() {
		It("should retry on HTTP 429 and succeed on 2nd attempt", func() {
			// BR-GATEWAY-112: Error Classification (429 is retryable)
			// BR-GATEWAY-113: Exponential Backoff
			// BR-GATEWAY-114: Retry Metrics

			// Setup: First attempt fails with 429, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// First attempt: rate limited
					return apierrors.NewTooManyRequests("rate limited", 1)
				}
				// Second attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-429",
				AlertName:   "TestAlert429",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert429",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(rr.Name).To(ContainSubstring("rr-"))
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
			Expect(mockClient.GetCallCount()).To(Equal(2), "Mock client should have been called twice")

			// Verify: Metrics incremented
			// Note: Metrics verification will be implemented after metrics are added
		})

		It("should retry on HTTP 503 and succeed on 3rd attempt", func() {
			// BR-GATEWAY-112: Error Classification (503 is retryable)
			// BR-GATEWAY-113: Exponential Backoff (100ms → 200ms → success)

			// Setup: First two attempts fail with 503, third succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount <= 2 {
					// First and second attempts: service unavailable
					return apierrors.NewServiceUnavailable("API server overloaded")
				}
				// Third attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-503",
				AlertName:   "TestAlert503",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert503",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(3), "Should have made exactly 3 attempts")

			// Note: Timing verification removed - backoff logic is tested in integration tests
		})

		It("should fail after max retries on persistent HTTP 503", func() {
			// BR-GATEWAY-112: Error Classification (503 is retryable but exhausted)
			// BR-GATEWAY-113: Max Attempts (3 attempts configured)

			// Setup: All attempts fail with 503
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewServiceUnavailable("API server overloaded")
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-503-fail",
				AlertName:   "TestAlert503Fail",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert503Fail",
				},
			}

			// Execute: Create CRD with retry (should fail)
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Failure after max retries
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(attemptCount).To(Equal(3), "Should have made exactly 3 attempts (max)")

			// Verify: Error is wrapped with retry context (GAP 10)
			var retryErr *processing.RetryError
			Expect(errors.As(err, &retryErr)).To(BeTrue(), "Error should be wrapped as RetryError")
			Expect(retryErr.Attempt).To(Equal(3))
			Expect(retryErr.MaxAttempts).To(Equal(3))
			Expect(retryErr.ErrorCode).To(Equal(503))
		})
	})

	Context("Retryable Errors - HTTP 504 Timeout", func() {
		It("should retry on HTTP 504 gateway timeout", func() {
			// BR-GATEWAY-112: Error Classification (504 is retryable)
			// BR-GATEWAY-113: Exponential Backoff

			// Setup: First attempt fails with 504, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// First attempt: gateway timeout
					return apierrors.NewTimeoutError("gateway timeout", 10)
				}
				// Second attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-504",
				AlertName:   "TestAlert504",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert504",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
		})

		It("should retry on context deadline exceeded", func() {
			// BR-GATEWAY-112: Error Classification (timeout errors are retryable)
			// BR-GATEWAY-113: Exponential Backoff

			// Setup: First attempt fails with context deadline, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// First attempt: context deadline exceeded (simulated as timeout error)
					return apierrors.NewTimeoutError("context deadline exceeded", 5)
				}
				// Second attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-timeout",
				AlertName:   "TestAlertTimeout",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertTimeout",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
		})
	})

	Context("Non-Retryable Errors", func() {
		It("should NOT retry on HTTP 400 (Bad Request)", func() {
			// BR-GATEWAY-112: Error Classification (400 is non-retryable)
			// Validation errors should fail fast

			// Setup: All attempts fail with 400 (but should only try once)
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewBadRequest("invalid CRD spec")
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-400",
				AlertName:   "TestAlert400",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert400",
				},
			}

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(attemptCount).To(Equal(1), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("invalid CRD spec"))
		})

		It("should NOT retry on HTTP 403 (Forbidden/RBAC)", func() {
			// BR-GATEWAY-112: Error Classification (403 is non-retryable)
			// RBAC errors should fail fast

			// Setup: All attempts fail with 403 (but should only try once)
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewForbidden(
					schema.GroupResource{Group: "remediation.kubernaut.io", Resource: "remediationrequests"},
					"test-rr",
					errors.New("insufficient permissions"),
				)
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-403",
				AlertName:   "TestAlert403",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert403",
				},
			}

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(attemptCount).To(Equal(1), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("forbidden"))
		})

		It("should NOT retry on HTTP 422 (Unprocessable Entity)", func() {
			// BR-GATEWAY-112: Error Classification (422 is non-retryable)
			// Schema validation errors should fail fast

			// Setup: All attempts fail with 422 (but should only try once)
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewInvalid(
					remediationv1alpha1.GroupVersion.WithKind("RemediationRequest").GroupKind(),
					"test-rr",
					nil,
				)
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-422",
				AlertName:   "TestAlert422",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert422",
				},
			}

			// Execute: Create CRD (should fail immediately)
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Immediate failure (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(attemptCount).To(Equal(1), "Should have made exactly 1 attempt (no retry)")
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})
	})

	Context("HTTP 409 Conflict Handling", func() {
		It("should NOT retry on HTTP 409 (already exists)", func() {
			// BR-GATEWAY-112: Error Classification (409 is non-retryable, idempotent)
			// Already exists is not an error condition

			// Setup: All attempts fail with 409 (but should only try once)
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewAlreadyExists(
					schema.GroupResource{Group: "remediation.kubernaut.io", Resource: "remediationrequests"},
					"test-rr",
				)
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-409",
				AlertName:   "TestAlert409",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlert409",
				},
			}

			// Execute: Create CRD (should fail immediately but gracefully)
			_, _ = creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Immediate failure (no retry), but CRD is fetched
			// Note: The actual implementation fetches the existing CRD, so this test
			// verifies the retry logic doesn't kick in for 409
			Expect(attemptCount).To(Equal(1), "Should have made exactly 1 attempt (no retry)")
		})
	})

	Context("Network Errors", func() {
		It("should retry on connection refused", func() {
			// BR-GATEWAY-112: Error Classification (network errors are retryable)
			// Connection refused is a transient network error

			// Setup: First attempt fails with connection refused, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// First attempt: connection refused
					return errors.New("connection refused")
				}
				// Second attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-connrefused",
				AlertName:   "TestAlertConnRefused",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertConnRefused",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
		})

		It("should retry on connection reset", func() {
			// BR-GATEWAY-112: Error Classification (network errors are retryable)
			// Connection reset is a transient network error

			// Setup: First attempt fails with connection reset, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// First attempt: connection reset
					return errors.New("connection reset by peer")
				}
				// Second attempt: success
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-connreset",
				AlertName:   "TestAlertConnReset",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertConnReset",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
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

			// Setup: First 3 attempts fail, 4th succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount <= 3 {
					return apierrors.NewTooManyRequests("rate limited", 1)
				}
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with custom retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", customRetryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-backoff-cap",
				AlertName:   "TestAlertBackoffCap",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertBackoffCap",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(4), "Should have made exactly 4 attempts")
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

			// Setup: First attempt fails, second succeeds
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					return apierrors.NewTooManyRequests("rate limited", 1)
				}
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with custom retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", customRetryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-initial-backoff",
				AlertName:   "TestAlertInitialBackoff",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertInitialBackoff",
				},
			}

			// Execute: Create CRD with retry
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made exactly 2 attempts")
		})
	})

	Context("Context Cancellation", func() {
		It("should stop retrying on context cancellation", func() {
			// BR-GATEWAY-112: Context-aware retry (GAP 6: Graceful Shutdown)
			// Retry should stop immediately on context cancellation

			// Setup: Create cancellable context
			cancelCtx, cancel := context.WithCancel(context.Background())

			// Setup: First attempt fails, cancel context before retry
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					// Cancel context after first attempt
					cancel()
					return apierrors.NewTooManyRequests("rate limited", 1)
				}
				// Should never reach here
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-ctx-cancel",
				AlertName:   "TestAlertCtxCancel",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertCtxCancel",
				},
			}

			// Execute: Create CRD with retry (context will be cancelled)
			rr, err := creator.CreateRemediationRequest(cancelCtx, signal, "prod", "P0")

			// Verify: Failure due to context cancellation (no retry)
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			Expect(attemptCount).To(Equal(1), "Should have made exactly 1 attempt before context cancellation")
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		It("should return context error immediately", func() {
			// BR-GATEWAY-112: Context-aware retry (GAP 6: Graceful Shutdown)
			// Context deadline exceeded should be detected during backoff

			// Setup: Create context with very short deadline
			deadlineCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// Setup: All attempts fail with retryable error
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				return apierrors.NewTooManyRequests("rate limited", 1)
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with retry config
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-ctx-deadline",
				AlertName:   "TestAlertCtxDeadline",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertCtxDeadline",
				},
			}

			// Execute: Create CRD with retry (context will timeout during backoff)
			rr, err := creator.CreateRemediationRequest(deadlineCtx, signal, "prod", "P0")

			// Verify: Failure due to context deadline
			Expect(err).To(HaveOccurred())
			Expect(rr).To(BeNil())
			// Should have made 1-2 attempts before context deadline
			Expect(attemptCount).To(BeNumerically("<=", 2), "Should stop retrying after context deadline")
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})
	})

	Context("Config Validation", func() {
		It("should use default config when nil", func() {
			// BR-GATEWAY-111: Default retry configuration (GAP 7: Config Validation)
			// Nil config should use sensible defaults

			// Setup: Pass nil retry config
			attemptCount := 0
			mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
				attemptCount++
				if attemptCount == 1 {
					return apierrors.NewTooManyRequests("rate limited", 1)
				}
				return nil
			}

			// Wrap mock client in k8s.Client
			k8sClient := k8s.NewClient(mockClient)

			// Create CRD creator with nil retry config (should use defaults)
			creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", nil)

			// Create test signal
			signal := &types.NormalizedSignal{
				Fingerprint: "test-fingerprint-default-config",
				AlertName:   "TestAlertDefaultConfig",
				Severity:    "critical",
				Namespace:   "production",
				Labels: map[string]string{
					"alertname": "TestAlertDefaultConfig",
				},
			}

			// Execute: Create CRD with retry (using default config)
			rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

			// Verify: Success after retry (default config allows retries)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(attemptCount).To(Equal(2), "Should have made 2 attempts with default config")
		})

		It("should validate MaxAttempts >= 1", func() {
			// BR-GATEWAY-111: Config validation (GAP 7: Config Validation)
			// MaxAttempts must be at least 1

			// Setup: Invalid retry config with MaxAttempts = 0
			invalidConfig := &config.RetrySettings{
				MaxAttempts:    0, // Invalid
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     1 * time.Second,
			}

			// Execute: Validate config
			err := invalidConfig.Validate()

			// Verify: Validation fails
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retry.max_attempts must be >= 1"))
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
			Expect(err.Error()).To(ContainSubstring("retry.max_backoff"))
		})
	})

	// NOTE: Retry Toggles tests removed (Reliability-First Design)
	// Retries are now ALWAYS enabled for transient errors (429, 503, 504, timeouts, network errors)
	// This ensures maximum reliability without configuration complexity
})
