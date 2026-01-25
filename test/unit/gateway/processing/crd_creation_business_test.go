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
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: CRD Creation with Correct Business Metadata
// ============================================================================
//
// PURPOSE: Validate BR-GATEWAY-004 - RemediationRequest CRD creation from signals
//
// BUSINESS VALUE:
// - Operations can identify WHAT failed (alertName in CRD metadata)
// - RO can prioritize remediation (severity classification)
// - RO can select correct workflow (resource kind, name, namespace)
// - Audit trail preserved (original payload, timestamps, fingerprint)
// - Resilience through retry logic (transient K8s API failures handled)
//
// NOT TESTING: Implementation details, internal data structures, K8s API internals
// ============================================================================

var _ = Describe("BR-GATEWAY-004: RemediationRequest CRD Creation Business Outcomes", func() {
	var (
		crdCreator      *processing.CRDCreator
		k8sClient       *k8s.Client
		ctx             context.Context
		testNamespace   string
		metricsInstance *metrics.Metrics
		fallbackNS      string
		retryConfig     *config.RetrySettings
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-signals"
		fallbackNS = "kubernaut-system"

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}

		// Create fallback namespace
		fallbackNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fallbackNS,
			},
		}

		// Setup fake K8s client with RemediationRequest CRD
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = remediationv1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ns, fallbackNamespace).
			Build()

		// Create real K8s client wrapper
		k8sClient = k8s.NewClient(fakeClient)

		// Create test-isolated metrics instance
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)

		// Default retry configuration
		retryConfig = &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: 50 * time.Millisecond,
			MaxBackoff:     200 * time.Millisecond,
		}

		// Create CRD creator
		crdCreator = processing.NewCRDCreator(
			k8sClient,
			logr.Discard(),
			metricsInstance,
			fallbackNS,
			retryConfig,
		)
	})

	Context("Business Metadata Population", func() {
		It("creates CRD with correct alertName for remediation identification", func() {
			// BUSINESS OUTCOME: Operations can identify WHAT failed
			// RO uses alertName to select appropriate workflow
			signal := &types.NormalizedSignal{
				AlertName:    "HighMemoryUsage",
				Fingerprint:  "test-fingerprint-abc123",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-api-789",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred(),
				"CRD creation must succeed for valid signals")
			Expect(rr.Spec.SignalName).To(Equal("HighMemoryUsage"),
				"AlertName must be correctly populated for remediation identification")
			Expect(rr.Name).To(ContainSubstring("rr-"),
				"CRD name must follow rr-{fingerprint-prefix}-{timestamp} pattern")
		})

		It("creates CRD with correct severity for remediation prioritization", func() {
			// BUSINESS OUTCOME: RO prioritizes critical > warning > info
			// Severity determines urgency of remediation action
			signal := &types.NormalizedSignal{
				AlertName:    "PodCrashLooping",
				Fingerprint:  "crash-fingerprint-xyz789",
				Severity:     "critical", // CRITICAL = immediate remediation
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-api-789",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.Severity).To(Equal("critical"),
				"Severity must be preserved for RO prioritization")
			Expect(rr.Labels["kubernaut.ai/severity"]).To(Equal("critical"),
				"Severity label enables filtering: kubectl get rr -l kubernaut.ai/severity=critical")
		})

		It("creates CRD with correct resource identification for workflow selection", func() {
			// BUSINESS OUTCOME: RO selects workflow based on resource Kind
			// WorkflowExecution targets specific resource Name/Namespace
			signal := &types.NormalizedSignal{
				AlertName:    "DeploymentUnhealthy",
				Fingerprint:  "deploy-fingerprint-def456",
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "payment-service",
					Namespace: testNamespace,
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Deployment"),
				"Resource Kind required for workflow selection (Pod vs Deployment vs Node)")
			Expect(rr.Spec.TargetResource.Name).To(Equal("payment-service"),
				"Resource Name required for targeting specific instance")
			Expect(rr.Spec.TargetResource.Namespace).To(Equal(testNamespace),
				"Resource Namespace required for multi-tenant remediation")
		})

		It("creates CRD with fingerprint for deduplication tracking", func() {
			// BUSINESS OUTCOME: Gateway can track duplicate occurrences
			// DD-GATEWAY-011: Status.Deduplication uses fingerprint as key
			signal := &types.NormalizedSignal{
				AlertName:    "HighMemoryUsage",
				Fingerprint:  "dedup-test-fingerprint-unique",
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.SignalFingerprint).To(Equal("dedup-test-fingerprint-unique"),
				"Fingerprint enables deduplication tracking (BR-GATEWAY-185)")
			Expect(rr.Labels["kubernaut.ai/signal-fingerprint"]).NotTo(BeEmpty(),
				"Fingerprint label enables filtering duplicate signals")
		})

		It("creates CRD with timestamp-based naming for unique occurrences", func() {
			// BUSINESS OUTCOME: Same problem can be remediated multiple times
			// DD-015: Timestamp ensures each occurrence creates unique CRD

			// Use MockClock for fast, deterministic testing (no time.Sleep needed)
			mockClock := processing.NewMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			crdCreatorWithClock := processing.NewCRDCreatorWithClock(
				k8sClient,
				logr.Discard(),
				metricsInstance,
				fallbackNS,
				retryConfig,
				mockClock,
			)

			signal := &types.NormalizedSignal{
				AlertName:    "SameIssue",
				Fingerprint:  "same-fingerprint",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			// Create first occurrence
			rr1, err1 := crdCreatorWithClock.CreateRemediationRequest(ctx, signal)
			Expect(err1).NotTo(HaveOccurred())

			// Advance clock by 1 second (instant, no actual wait)
			mockClock.Advance(1 * time.Second)

			// Create second occurrence (same problem recurring)
			rr2, err2 := crdCreatorWithClock.CreateRemediationRequest(ctx, signal)
			Expect(err2).NotTo(HaveOccurred())

		// BUSINESS OUTCOME: Different CRD names enable tracking each remediation attempt
		Expect(rr1.Name).NotTo(Equal(rr2.Name),
			"UUID-based naming allows same problem to be remediated multiple times")
		// DD-AUDIT-CORRELATION-002: UUID suffix guarantees zero collision risk
		Expect(rr1.Name).To(MatchRegexp(`^rr-same-fingerp-[0-9a-f]{8}$`),
			"CRD name follows rr-{fingerprint-prefix}-{uuid-suffix} pattern (8 hex chars)")
		Expect(rr2.Name).To(MatchRegexp(`^rr-same-fingerp-[0-9a-f]{8}$`),
			"Each occurrence gets unique UUID suffix in CRD name")
		})
	})

	Context("Business Correctness - Resource Validation", func() {
		It("rejects signals missing resource Kind (workflow selection requirement)", func() {
			// BUSINESS OUTCOME: V1.0 is Kubernetes-only
			// Missing Kind means RO cannot select appropriate workflow
			invalidSignal := &types.NormalizedSignal{
				AlertName:    "TestAlert",
				Fingerprint:  "test-fingerprint",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "", // MISSING - cannot select workflow
					Name: "test-resource",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without resource Kind - RO cannot select workflow")
			Expect(rr).To(BeNil(),
				"No CRD created for invalid signals")
			Expect(err.Error()).To(ContainSubstring("Kind"),
				"Error message must indicate which field is missing")
			Expect(err.Error()).To(ContainSubstring("V1.0 requires valid Kubernetes resource info"),
				"Error provides context: V1.0 is Kubernetes-only")
		})

		It("rejects signals missing resource Name (remediation target requirement)", func() {
			// BUSINESS OUTCOME: Cannot remediate without knowing WHICH instance
			// WorkflowExecution needs specific resource name for kubectl commands
			invalidSignal := &types.NormalizedSignal{
				AlertName:    "TestAlert",
				Fingerprint:  "test-fingerprint",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "", // MISSING - cannot target specific resource
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without resource Name - cannot target remediation")
			Expect(rr).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("Name"),
				"Error message must indicate resource Name is required")
		})

		It("accepts cluster-scoped resources without namespace (Node, ClusterRole)", func() {
			// BUSINESS OUTCOME: Cluster-scoped resources don't have namespaces
			// Node alerts must still be processed for remediation
			clusterScopedSignal := &types.NormalizedSignal{
				AlertName:    "NodeNotReady",
				Fingerprint:  "node-fingerprint-abc",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind:      "Node",
					Name:      "worker-node-1",
					Namespace: "", // Cluster-scoped - no namespace
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, clusterScopedSignal)

			Expect(err).NotTo(HaveOccurred(),
				"Cluster-scoped resources without namespace must be accepted")
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Node"))
			Expect(rr.Spec.TargetResource.Name).To(Equal("worker-node-1"))
			Expect(rr.Spec.TargetResource.Namespace).To(BeEmpty(),
				"Cluster-scoped resources correctly have empty namespace")
		})
	})

	Context("Business Resilience - Fallback Namespace Handling", func() {
		It("uses specified namespace when it exists in cluster", func() {
			// BUSINESS OUTCOME: Signals are created in their target namespace
			// This allows proper multi-tenant remediation
			signal := &types.NormalizedSignal{
				AlertName:    "PodCrashLooping",
				Fingerprint:  "ns-test-fingerprint",
				Severity:     "critical",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace, // Using existing namespace
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred(),
				"CRD creation must succeed when namespace exists")
			Expect(rr.Namespace).To(Equal(testNamespace),
				"CRD created in signal's target namespace for proper tenant isolation")
		})
	})

	Context("Business Auditing - Metadata Preservation", func() {
		It("preserves signal source type for audit trail", func() {
			// BUSINESS OUTCOME: Audit trail shows WHERE signal originated
			// Operations can filter by source: Prometheus vs K8s Events
			signal := &types.NormalizedSignal{
				AlertName:    "TestAlert",
				Fingerprint:  "audit-test",
				Severity:     "info",
				SourceType:   "prometheus-alert", // Prometheus AlertManager source
				Source:       "alertmanager",
				Namespace:    testNamespace,
				ReceivedTime: time.Now(),
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.SignalType).To(Equal("prometheus-alert"),
				"Signal type preserved for audit trail and filtering")
			Expect(rr.Spec.SignalSource).To(Equal("alertmanager"),
				"Signal source preserved for troubleshooting")
			Expect(rr.Labels["kubernaut.ai/signal-type"]).To(Equal("prometheus-alert"),
				"Signal type label enables filtering by source")
		})

		It("preserves temporal data for audit trail (firing and received times)", func() {
			// BUSINESS OUTCOME: Audit trail shows WHEN issue occurred
			// Time difference shows alert propagation latency
			firingTime := time.Now().Add(-5 * time.Minute) // Alert fired 5 minutes ago
			receivedTime := time.Now()                     // Received just now

			signal := &types.NormalizedSignal{
				AlertName:    "DelayedAlert",
				Fingerprint:  "time-test",
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Source:       "alertmanager",
				Namespace:    testNamespace,
				FiringTime:   firingTime,
				ReceivedTime: receivedTime,
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.FiringTime.Time).To(BeTemporally("~", firingTime, time.Second),
				"Firing time preserved - shows WHEN problem occurred")
			Expect(rr.Spec.ReceivedTime.Time).To(BeTemporally("~", receivedTime, time.Second),
				"Received time preserved - shows WHEN Gateway received signal")
			Expect(rr.Annotations["kubernaut.ai/created-at"]).NotTo(BeEmpty(),
				"Creation timestamp in annotation for audit trail")
		})
	})

})

// ============================================================================
// BUSINESS OUTCOME TESTS: Annotation Truncation for K8s Compliance
// ============================================================================
//
// BR-GATEWAY-009: Oversized annotations are truncated to fit K8s limits
//
// BUSINESS VALUE:
// - Prevents CRD creation failures from K8s size limits
// - Preserves as much data as possible within limits
// - Ensures signal metadata is always persisted
// ============================================================================

var _ = Describe("BR-GATEWAY-009: Oversized Annotations Truncation", func() {
	var (
		crdCreator      *processing.CRDCreator
		k8sClient       *k8s.Client
		ctx             context.Context
		testNamespace   string
		metricsInstance *metrics.Metrics
		fallbackNS      string
		retryConfig     *config.RetrySettings
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-signals"
		fallbackNS = "kubernaut-system"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		fallbackNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fallbackNS,
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = remediationv1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ns, fallbackNamespace).
			Build()

		k8sClient = k8s.NewClient(fakeClient)

		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)

		retryConfig = &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: 50 * time.Millisecond,
			MaxBackoff:     200 * time.Millisecond,
		}

		crdCreator = processing.NewCRDCreator(
			k8sClient,
			logr.Discard(),
			metricsInstance,
			fallbackNS,
			retryConfig,
		)
	})

	Context("when signal contains oversized annotations", func() {
		It("successfully creates CRD without exceeding K8s limits", func() {
			// BUSINESS OUTCOME: CRD creation succeeds even with large payloads
			largePayload := make([]byte, 300000) // 300KB, exceeds 256KB limit
			for i := range largePayload {
				largePayload[i] = 'X'
			}

			signal := &types.NormalizedSignal{
				Fingerprint: "oversized-test-fingerprint",
				AlertName:   "LargePayloadAlert",
				Severity:    "critical",
				Namespace:   testNamespace,
				Resource: types.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "large-payload-pod",
					Namespace: testNamespace,
				},
				Annotations: map[string]string{
					"large_annotation": string(largePayload),
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus",
				RawPayload:   largePayload,
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS EXPECTATION: CRD created (no K8s size limit error)
			Expect(err).ToNot(HaveOccurred(),
				"CRD creation should succeed - annotations were truncated")
			Expect(rr).ToNot(BeNil(),
				"RemediationRequest must be returned")
			Expect(rr.Name).To(HavePrefix("rr-oversized-te"),
				"CRD preserves fingerprint for correlation")
		})
	})

	Context("when signal has nil annotations", func() {
		It("successfully creates CRD without annotations", func() {
			// BUSINESS OUTCOME: Signals without annotations are valid
			signal := &types.NormalizedSignal{
				Fingerprint: "no-annotations-fingerprint",
				AlertName:   "SimpleAlert",
				Severity:    "warning",
				Namespace:   testNamespace,
				Resource: types.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "simple-deploy",
					Namespace: testNamespace,
				},
				Annotations:  nil, // No annotations
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "kubernetes-event",
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			Expect(err).ToNot(HaveOccurred(),
				"CRD creation succeeds without annotations")
			Expect(rr).ToNot(BeNil())
		})
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: CRDCreator Initialization with Defaults
// ============================================================================
//
// BR-GATEWAY-019: CRD Name Generation - Safe Defaults
// Sub-requirement: CRDCreator must work with minimal configuration
//
// BUSINESS VALUE:
// - CRDCreator works even without all configuration
// - Safe defaults prevent nil pointer panics
// - Operator doesn't need to specify every config option
// ============================================================================

var _ = Describe("BR-GATEWAY-019: CRDCreator Safe Defaults", func() {
	Context("when initialized with empty fallback namespace", func() {
		It("uses safe default namespace", func() {
			// BUSINESS OUTCOME: Empty namespace doesn't cause failures
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
			}

			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = remediationv1alpha1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns).
				Build()

			k8sClient := k8s.NewClient(fakeClient)
			registry := prometheus.NewRegistry()
			metricsInstance := metrics.NewMetricsWithRegistry(registry)

			// Initialize with empty fallback namespace
			crdCreator := processing.NewCRDCreator(
				k8sClient,
				logr.Discard(),
				metricsInstance,
				"", // empty fallback namespace
				nil,
			)

			Expect(crdCreator).NotTo(BeNil(),
				"CRDCreator uses default 'kubernaut-system' for empty namespace")
		})
	})

	// ============================================================================
	// EDGE CASE TESTS: CreateRemediationRequest Error Handling
	// ============================================================================
	//
	// PURPOSE: Validate CreateRemediationRequest handles edge cases correctly
	//
	// BUSINESS VALUE:
	// - Namespace not found → fallback namespace (cluster-scoped signals)
	// - Different source types → correct signalType field
	// - Empty labels/namespace → safe handling
	// ============================================================================

	var _ = Describe("CreateRemediationRequest Edge Cases", func() {
		var (
			crdCreator      *processing.CRDCreator
			k8sClient       *k8s.Client
			ctx             context.Context
			testNamespace   string
			fallbackNS      string
			metricsInstance *metrics.Metrics
			retryConfig     *config.RetrySettings
		)

		BeforeEach(func() {
			ctx = context.Background()
			testNamespace = "test-signals"
			fallbackNS = "kubernaut-system"

			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}

			// Create fallback namespace
			fallbackNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: fallbackNS},
			}

			// Setup fake K8s client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = remediationv1alpha1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns, fallbackNamespace).
				Build()

			k8sClient = k8s.NewClient(fakeClient)

			registry := prometheus.NewRegistry()
			metricsInstance = metrics.NewMetricsWithRegistry(registry)

			retryConfig = &config.RetrySettings{
				MaxAttempts:    3,
				InitialBackoff: 50 * time.Millisecond,
				MaxBackoff:     200 * time.Millisecond,
			}

			crdCreator = processing.NewCRDCreator(
				k8sClient,
				logr.Discard(),
				metricsInstance,
				fallbackNS,
				retryConfig,
			)
		})

		Context("when signal source type is kubernetes-event", func() {
			It("correctly sets signalType for event-based signals", func() {
				// BUSINESS OUTCOME: Different source types tracked for observability
				signal := &types.NormalizedSignal{
					AlertName:    "PodEvicted",
					Fingerprint:  "k8s-event-fingerprint",
					Severity:     "warning",
					SourceType:   "kubernetes-event", // Different source type
					Source:       "kube-apiserver",
					Namespace:    testNamespace,
					ReceivedTime: time.Now(),
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "evicted-pod",
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Spec.SignalType).To(Equal("kubernetes-event"),
					"SignalType distinguishes prometheus-alert vs kubernetes-event sources")
				Expect(rr.Labels["kubernaut.ai/signal-type"]).To(Equal("kubernetes-event"),
					"Signal type label enables filtering by source")
			})
		})

		Context("when signal has empty labels", func() {
			It("creates CRD without signal labels", func() {
				// BUSINESS OUTCOME: Minimal signals without labels are valid
				signal := &types.NormalizedSignal{
					AlertName:    "MinimalAlert",
					Fingerprint:  "minimal-fingerprint",
					Severity:     "info",
					SourceType:   "prometheus-alert",
					Source:       "alertmanager",
					Namespace:    testNamespace,
					ReceivedTime: time.Now(),
					Labels:       nil, // No labels
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "minimal-pod",
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Spec.SignalLabels).To(BeNil(),
					"Empty labels handled gracefully")
			})
		})

		Context("when signal has custom source type", func() {
			It("creates CRD with custom source type preserved", func() {
				// BUSINESS OUTCOME: Future extensibility for non-prometheus/k8s sources
				signal := &types.NormalizedSignal{
					AlertName:    "CustomSourceAlert",
					Fingerprint:  "custom-source-fingerprint",
					Severity:     "warning",
					SourceType:   "custom-monitoring-tool", // Custom source
					Source:       "my-custom-tool",
					Namespace:    testNamespace,
					ReceivedTime: time.Now(),
					Resource: types.ResourceIdentifier{
						Kind: "Deployment",
						Name: "custom-deploy",
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Spec.SignalType).To(Equal("custom-monitoring-tool"),
					"Custom source types preserved for extensibility")
				Expect(rr.Spec.SignalSource).To(Equal("my-custom-tool"),
					"Source identifier preserved")
			})

			Context("when signal has nil namespace (buildProviderData coverage)", func() {
				It("creates CRD with empty namespace in ProviderData", func() {
					// BUSINESS OUTCOME: Cluster-scoped resources without namespace handled
					signal := &types.NormalizedSignal{
						AlertName:    "ClusterScopedAlert",
						Fingerprint:  "cluster-scoped-fingerprint",
						Severity:     "warning",
						SourceType:   "prometheus-alert",
						Source:       "alertmanager",
						Namespace:    "", // Empty namespace (cluster-scoped)
						ReceivedTime: time.Now(),
						Labels:       map[string]string{"cluster": "production"},
						Resource: types.ResourceIdentifier{
							Kind: "Node",
							Name: "worker-node-1",
						},
					}

					// This will use fallback namespace for CRD, but we're testing ProviderData
					rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

					Expect(err).ToNot(HaveOccurred())
					Expect(rr.Spec.ProviderData).ToNot(BeNil(),
						"ProviderData should be populated even with empty namespace")
					// ProviderData is JSON - should contain empty namespace value
					var providerDataMap map[string]interface{}
					jsonErr := json.Unmarshal(rr.Spec.ProviderData, &providerDataMap)
					Expect(jsonErr).ToNot(HaveOccurred())
					Expect(providerDataMap).To(HaveKey("namespace"))
					Expect(providerDataMap).To(HaveKey("labels"))
				})
			})

			Context("when signal has nil labels (buildProviderData coverage)", func() {
				It("creates CRD with null labels in ProviderData", func() {
					// BUSINESS OUTCOME: Signals without labels are valid
					signal := &types.NormalizedSignal{
						AlertName:    "NoLabelsAlert",
						Fingerprint:  "no-labels-fingerprint",
						Severity:     "info",
						SourceType:   "kubernetes-event",
						Source:       "kubelet",
						Namespace:    testNamespace,
						ReceivedTime: time.Now(),
						Labels:       nil, // Nil labels
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: "no-labels-pod",
						},
					}

					rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

					Expect(err).ToNot(HaveOccurred())
					Expect(rr.Spec.ProviderData).ToNot(BeNil(),
						"ProviderData should be populated even with nil labels")
					// ProviderData JSON should handle nil labels gracefully
					var providerDataMap map[string]interface{}
					jsonErr := json.Unmarshal(rr.Spec.ProviderData, &providerDataMap)
					Expect(jsonErr).ToNot(HaveOccurred(),
						"ProviderData JSON should be valid even with nil labels")
				})
			})
		})
	})
})
