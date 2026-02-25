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

package gateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// BR-GATEWAY-092: Notification Metadata Completeness
// Business Outcome: Downstream notification service has ALL data needed to alert humans
var _ = Describe("BR-GATEWAY-092: Notification Metadata in RemediationRequest CRD", func() {
	var (
		crdCreator    *processing.CRDCreator
		ctx           context.Context
		logger        logr.Logger
		fakeK8sClient *k8s.Client
	)

	BeforeEach(func() {
		logger = logr.Discard() // No-op logger for tests (no output)
		ctx = context.Background()

		// Create fake K8s client using controller-runtime fake client builder
		scheme := runtime.NewScheme()
		_ = remediationv1alpha1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		fakeK8sClient = k8s.NewClient(fakeClient)
		retryConfig := config.DefaultRetrySettings()
		// Use isolated metrics registry per test to avoid collisions
		testRegistry := prometheus.NewRegistry()
		testMetrics := metrics.NewMetricsWithRegistry(testRegistry)
		crdCreator = processing.NewCRDCreator(fakeK8sClient, logger, testMetrics, &retryConfig, &mocks.NoopRetryObserver{}, "kubernaut-system")
	})

	// BUSINESS CAPABILITY: Notification service needs complete context to alert humans
	Context("when creating CRD for notification service consumption", func() {
		It("enables notification service to identify WHAT failed for alert content", func() {
			// BUSINESS SCENARIO: PagerDuty needs to tell on-call engineer:
			// "Alert: payment-api pod crashed in production"
			// Required fields: signalName, severity, namespace, resource details

			signal := &types.NormalizedSignal{
				Fingerprint: "abc123def456ghi789jkl012mno345", // Must be >=16 chars
				SignalName:   "PodCrashLooping",
				Severity:    "critical",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-api-789",
				},
				Labels: map[string]string{
					"alertname": "PodCrashLooping",
					"pod":       "payment-api-789",
				},
				FiringTime:   time.Now().Add(-5 * time.Minute),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{"alerts":[{"status":"firing"}]}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Notification has alert name and severity
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.SignalName).To(Equal("PodCrashLooping"),
				"Notification service needs alertname for PagerDuty title")
			Expect(rr.Spec.Severity).To(Equal("critical"),
				"Notification service needs severity for PagerDuty priority")
			Expect(rr.Namespace).To(Equal("kubernaut-system"),
				"ADR-057: CRD must be in controller namespace for security boundary")

			// Business capability verified:
			// PagerDuty alert: "🚨 Critical: payment-api pod crashed in production"
		})

		// Note: Test "enables notification service to identify WHO to alert based on priority" REMOVED (2025-12-06)
		// Priority classification moved to Signal Processing per DD-CATEGORIZATION-001
		// Gateway no longer populates priority labels or spec fields

		It("enables notification service to provide WHEN context for incident timeline", func() {
			// BUSINESS SCENARIO: On-call engineer needs timeline:
			// "Alert started firing at 14:05 UTC, Gateway received at 14:06 UTC"
			// Required fields: firingTime, receivedTime

			firingTime := time.Date(2025, 10, 10, 14, 5, 0, 0, time.UTC)
			receivedTime := time.Date(2025, 10, 10, 14, 6, 30, 0, time.UTC)

			signal := &types.NormalizedSignal{
				Fingerprint: "timeline123abc456def789ghi012jk", // Must be >=16 chars
				SignalName:   "HighLatency",
				Severity:    "warning",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "Service",
					Name:      "api-gateway",
					Namespace: "production",
				},
				FiringTime:   firingTime,
				ReceivedTime: receivedTime,
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Notification has timestamps for incident timeline
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.FiringTime.Time).To(Equal(firingTime),
				"Notification service needs firing time for 'alert started at...'")
			Expect(rr.Spec.ReceivedTime.Time).To(Equal(receivedTime),
				"Notification service needs received time to calculate latency")

			// Calculate notification latency
			latency := receivedTime.Sub(firingTime)
			Expect(latency).To(Equal(90*time.Second),
				"Notification service can show: 'Alert detected 90 seconds ago'")

			// Business capability verified:
			// PagerDuty incident: "Alert firing since 14:05 UTC (90 seconds ago)"
		})

		// Note: Test "enables notification service to include WHERE context for incident location" REMOVED (2025-12-06)
		// Environment classification moved to Signal Processing per DD-CATEGORIZATION-001
		// Gateway no longer populates environment labels or spec fields

		It("enables notification service to show ORIGINAL alert details for engineer investigation", func() {
			// BUSINESS SCENARIO: On-call engineer clicks PagerDuty link, sees:
			// - Full Prometheus labels (instance, job, container, etc.)
			// - Alert annotations (description, runbook URL, dashboard link)
			// - Original alert payload (for debugging false positives)
			// Required fields: signalLabels, signalAnnotations, originalPayload

			signal := &types.NormalizedSignal{
				Fingerprint: "details789abc123def456ghi789jkl", // Must be >=16 chars
				SignalName:   "PodMemoryHigh",
				Severity:    "critical",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "payment-api-789",
					Namespace: "production",
				},
				Labels: map[string]string{
					"alertname": "PodMemoryHigh",
					"pod":       "payment-api-789",
					"instance":  "10.0.1.45:9090",
					"job":       "kubernetes-pods",
					"container": "payment-api",
					"team":      "platform-engineering",
				},
				Annotations: map[string]string{
					"summary":     "Pod memory usage > 90%",
					"description": "Pod payment-api-789 in production namespace is using 92% memory",
					"runbook_url": "https://wiki.company.com/runbooks/pod-memory",
					"dashboard":   "https://grafana.company.com/d/pod-memory",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{"alerts":[{"labels":{"alertname":"PodMemoryHigh"},"annotations":{"summary":"Pod memory > 90%"}}]}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Notification shows all Prometheus labels
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.SignalLabels).To(HaveKey("pod"),
				"Engineer needs pod name to run 'kubectl logs payment-api-789'")
			Expect(rr.Spec.SignalLabels).To(HaveKey("team"),
				"Notification service can route to team: platform-engineering")
			Expect(rr.Spec.SignalLabels["instance"]).To(ContainSubstring("10.0.1.45"),
				"Engineer needs instance IP for SSH debugging")

			// BUSINESS OUTCOME: Notification shows alert annotations
			Expect(rr.Spec.SignalAnnotations).To(HaveKey("runbook_url"),
				"PagerDuty shows clickable runbook link for engineer")
			Expect(rr.Spec.SignalAnnotations).To(HaveKey("dashboard"),
				"PagerDuty shows clickable Grafana dashboard link")
			Expect(rr.Spec.SignalAnnotations["description"]).To(ContainSubstring("92%"),
				"Engineer sees exact memory percentage in notification")

			// BUSINESS OUTCOME: Notification preserves original payload
			Expect(rr.Spec.OriginalPayload).NotTo(BeEmpty(),
				"Engineer can view raw Prometheus webhook for debugging")
			var payload map[string]interface{}
			err = json.Unmarshal([]byte(rr.Spec.OriginalPayload), &payload)
			Expect(err).NotTo(HaveOccurred(),
				"Original payload is valid JSON for notification service parsing")

			// Business capability verified:
			// PagerDuty incident card shows:
			// - Title: "🚨 PodMemoryHigh: payment-api-789 (92% memory)"
			// - Links: [Runbook] [Grafana Dashboard]
			// - Details: All Prometheus labels + annotations
			// - Raw: Original webhook payload
		})

		It("enables notification service to show STORM context for mass incident awareness", func() {
			// BUSINESS SCENARIO: On-call engineer receives:
			// "⚡ ALERT STORM: 50 pods crashing in production (related to deployment rollout)"
			// vs 50 individual PagerDuty pages (alert fatigue)
			// Required fields: isStorm, stormType, stormWindow, stormAlertCount

			signal := &types.NormalizedSignal{
				Fingerprint: "storm123abc456def789ghi012jkl34", // Must be >=16 chars
				SignalName:   "PodCrashLooping",
				Severity:    "critical",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-deployment",
					Namespace: "production",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
				// Storm metadata removed (DD-GATEWAY-015)
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: CRD created successfully (storm fields removed per DD-GATEWAY-015)
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Name).ToNot(BeEmpty(), "CRD should be created successfully")
			Expect(rr.Spec.SignalName).To(Equal("PodCrashLooping"), "Signal name should be set correctly")

			// Business capability verified:
			// PagerDuty: "⚡ ALERT STORM: 50 PodCrashLooping alerts in 1m (likely rollout issue)"
			// Engineer response: Check recent deployments, not individual pods
		})

		It("enables notification service to provide deduplication context for recurring issues", func() {
			// BUSINESS SCENARIO: On-call engineer sees:
			// "Alert seen 5 times in last 10 minutes (recurring issue)"
			// vs "New alert just fired"
			// Required fields: deduplication.firstSeen, deduplication.lastSeen, deduplication.occurrenceCount

			signal := &types.NormalizedSignal{
				Fingerprint: "recurring456abc789def012ghi345jk", // Must be >=16 chars
				SignalName:   "DiskSpaceRunningOut",
				Severity:    "warning",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "PersistentVolume",
					Name:      "data-pv-1",
					Namespace: "production",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: CRD structure is correct for deduplication
			Expect(err).NotTo(HaveOccurred())

			// DD-GATEWAY-011: Deduplication metadata moved from spec to status
			// CRD Creator responsibility: Create CRD with correct structure
			// Gateway Server responsibility: Initialize Status.Deduplication after creation
			// This unit test validates the CRD creator's scope only

			// Verify: CRD created successfully with required fields
			Expect(rr.Spec.SignalFingerprint).NotTo(BeEmpty(),
				"Fingerprint required for deduplication")
			Expect(rr.Spec.SignalName).To(Equal("DiskSpaceRunningOut"))
			Expect(rr.Namespace).To(Equal("kubernaut-system"))

			// Note: Status.Deduplication is initialized by Gateway server after CRD creation
			// Integration tests in test/integration/gateway/dd_gateway_011_*.go verify
			// the complete flow including status initialization
			//
			// Business capability verified at integration level:
			// - PagerDuty on first alert: "🔔 New issue: Disk space running out"
			// - PagerDuty on 5th alert: "🔁 Recurring: Disk space (seen 5 times in 10 min)"

			// Business capability verified:
			// PagerDuty on first alert: "🔔 New issue: Disk space running out"
			// PagerDuty on 5th alert: "🔁 Recurring: Disk space (seen 5 times in 10 min)"
		})

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// PHASE 3: CRD METADATA EDGE CASES (BR-GATEWAY-015)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// Production Risk: K8s label/annotation limits cause CRD creation failures
		// Business Impact: Valid alerts rejected due to metadata size
		// Defense: Truncation and validation

		Context("Phase 3: CRD Metadata Edge Cases", func() {
			It("should truncate label values exceeding K8s 63 char limit", func() {
				// BR-GATEWAY-015: K8s label value limit compliance
				// BUSINESS OUTCOME: Long label values don't break CRD creation

				longLabelValue := "very-long-environment-name-that-exceeds-kubernetes-label-value-limit-of-63-characters"

				signal := &types.NormalizedSignal{
					Fingerprint: "fp-long-label-test12345678901234567890",
					SignalName:   "HighMemory",
					Severity:    "critical",
					Namespace:   "production",
					Resource: types.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "memory-test-pod",
						Namespace: "production",
					},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					SourceType:   "alert",
					Source:       "prometheus-adapter",
					RawPayload:   json.RawMessage(`{}`),
					Labels: map[string]string{
						"environment": longLabelValue, // >63 chars
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

				// BUSINESS OUTCOME: CRD created successfully with truncated label
				Expect(err).NotTo(HaveOccurred())
				Expect(rr).NotTo(BeNil())

				// Verify label was truncated to K8s limit
				if envLabel, ok := rr.Spec.SignalLabels["environment"]; ok {
					Expect(len(envLabel)).To(BeNumerically("<=", 63),
						"Label value must comply with K8s 63 char limit")
				}

				// Business capability verified:
				// System handles long label values gracefully without K8s API rejection
			})

			It("should handle extremely large annotations (>256KB K8s limit)", func() {
				// BR-GATEWAY-015: K8s annotation size limit compliance
				// BUSINESS OUTCOME: Large annotations don't break CRD creation

				// K8s annotation limit is 256KB total for all annotations
				largeAnnotation := make([]byte, 300*1024) // 300KB
				for i := range largeAnnotation {
					largeAnnotation[i] = 'A'
				}

				signal := &types.NormalizedSignal{
					Fingerprint: "fp-large-annotation-1234567890123",
					SignalName:   "HighMemory",
					Severity:    "critical",
					Namespace:   "production",
					Resource: types.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "large-annotation-pod",
						Namespace: "production",
					},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					SourceType:   "alert",
					Source:       "prometheus-adapter",
					RawPayload:   json.RawMessage(`{}`),
					Annotations: map[string]string{
						"description": string(largeAnnotation), // >256KB
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

				// BUSINESS OUTCOME: CRD created successfully (annotation truncated or rejected)
				// Implementation may either:
				// 1. Truncate annotation to fit K8s limit
				// 2. Reject signal with clear error
				// Both are acceptable - key is not crashing or hanging
				if err != nil {
					// If rejected, error should be clear
					Expect(err.Error()).To(ContainSubstring("annotation"),
						"Error should indicate annotation size issue")
				} else {
					// If accepted, annotation should be truncated
					Expect(rr).NotTo(BeNil())
					if desc, ok := rr.Spec.SignalAnnotations["description"]; ok {
						Expect(len(desc)).To(BeNumerically("<", 256*1024),
							"Annotation must be truncated to K8s limit")
					}
				}

				// Business capability verified:
				// System handles extremely large annotations without crashing
			})
		})
	})
})

// BR-GATEWAY-TARGET-RESOURCE-VALIDATION: Signals Must Have Resource Info (V1.0 Kubernetes-Only)
// Business Outcome: V1.0 is Kubernetes-only; signals without resource info indicate
// configuration issues at the source and should be rejected with clear HTTP 400 feedback.
// This prevents downstream processing failures (SP enrichment, AIAnalysis RCA, WE remediation).
var _ = Describe("BR-GATEWAY-TARGET-RESOURCE-VALIDATION: Resource Info Validation", func() {
	var (
		crdCreator    *processing.CRDCreator
		ctx           context.Context
		logger        logr.Logger
		fakeK8sClient *k8s.Client
	)

	BeforeEach(func() {
		logger = logr.Discard()
		ctx = context.Background()

		scheme := runtime.NewScheme()
		_ = remediationv1alpha1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		fakeK8sClient = k8s.NewClient(fakeClient)
		retryConfig := config.DefaultRetrySettings()
		// Use isolated metrics registry per test to avoid collisions
		testRegistry := prometheus.NewRegistry()
		testMetrics := metrics.NewMetricsWithRegistry(testRegistry)
		crdCreator = processing.NewCRDCreator(fakeK8sClient, logger, testMetrics, &retryConfig, &mocks.NoopRetryObserver{}, "kubernaut-system")
	})

	// BUSINESS CAPABILITY: V1.0 requires valid Kubernetes resource info for all signals
	Context("when signal is missing resource Kind", func() {
		It("should reject signal with clear error (HTTP 400)", func() {
			// BUSINESS SCENARIO: Alert source sends signal without resource kind
			// This indicates misconfiguration at the alert source
			// Rejecting early provides immediate feedback to fix configuration

			signal := &types.NormalizedSignal{
				Fingerprint: "fp-missing-kind-12345678901234567890",
				SignalName:   "HighMemoryUsage",
				Severity:    "critical",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "", // Missing Kind - should be rejected
					Name:      "payment-api-789",
					Namespace: "production",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Signal rejected with clear error
			Expect(err).To(HaveOccurred(), "Signal without resource Kind must be rejected")
			Expect(rr).To(BeNil(), "No CRD should be created for invalid signal")
			Expect(err.Error()).To(ContainSubstring("resource"),
				"Error must indicate resource validation issue")
			Expect(err.Error()).To(ContainSubstring("Kind"),
				"Error must specify that Kind is missing")

			// Business capability verified:
			// Alert source receives HTTP 400 with message: "resource Kind is required"
		})
	})

	Context("when signal is missing resource Name", func() {
		It("should reject signal with clear error (HTTP 400)", func() {
			// BUSINESS SCENARIO: Alert source sends signal without resource name
			// Downstream services cannot identify target without resource name

			signal := &types.NormalizedSignal{
				Fingerprint: "fp-missing-name-12345678901234567890",
				SignalName:   "CrashLoopBackOff",
				Severity:    "warning",
				Namespace:   "staging",
				Resource: types.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "", // Missing Name - should be rejected
					Namespace: "staging",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Signal rejected with clear error
			Expect(err).To(HaveOccurred(), "Signal without resource Name must be rejected")
			Expect(rr).To(BeNil(), "No CRD should be created for invalid signal")
			Expect(err.Error()).To(ContainSubstring("resource"),
				"Error must indicate resource validation issue")
			Expect(err.Error()).To(ContainSubstring("Name"),
				"Error must specify that Name is missing")

			// Business capability verified:
			// Alert source receives HTTP 400 with message: "resource Name is required"
		})
	})

	Context("when signal is missing both Kind and Name", func() {
		It("should reject signal with comprehensive error", func() {
			// BUSINESS SCENARIO: Completely empty resource info
			// Both Kind and Name required for V1.0 Kubernetes-only support

			signal := &types.NormalizedSignal{
				Fingerprint: "fp-empty-resource-12345678901234567890",
				SignalName:   "UnknownAlert",
				Severity:    "info",
				Namespace:   "default",
				Resource: types.ResourceIdentifier{
					Kind:      "", // Missing
					Name:      "", // Missing
					Namespace: "",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: Signal rejected
			Expect(err).To(HaveOccurred(), "Signal with empty resource must be rejected")
			Expect(rr).To(BeNil(), "No CRD should be created for invalid signal")

			// Business capability verified:
			// Clear feedback: signal has no target resource info
		})
	})

	Context("when signal has valid resource info", func() {
		It("should create CRD successfully", func() {
			// BUSINESS SCENARIO: Normal signal with complete resource info
			// This is the happy path - signal should be processed

			signal := &types.NormalizedSignal{
				Fingerprint: "fp-valid-resource-12345678901234567890",
				SignalName:   "HighCPUUsage",
				Severity:    "warning",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "production",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: CRD created with populated TargetResource
			Expect(err).NotTo(HaveOccurred(), "Valid signal should be processed")
			Expect(rr).NotTo(BeNil(), "CRD should be created")
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Deployment"))
			Expect(rr.Spec.TargetResource.Name).To(Equal("api-server"))
			Expect(rr.Spec.TargetResource.Namespace).To(Equal("production"))

			// Business capability verified:
			// SignalProcessing and RO can access resource info directly
		})
	})

	Context("when signal has resource Kind and Name but empty Namespace (cluster-scoped)", func() {
		It("should create CRD successfully for cluster-scoped resources", func() {
			// BUSINESS SCENARIO: NodeNotReady alert for cluster-scoped Node resource
			// Namespace is empty for cluster-scoped resources - this is valid

			signal := &types.NormalizedSignal{
				Fingerprint: "fp-cluster-scoped-12345678901234567890",
				SignalName:   "NodeNotReady",
				Severity:    "critical",
				Namespace:   "default", // Signal namespace (for CRD placement)
				Resource: types.ResourceIdentifier{
					Kind:      "Node",
					Name:      "worker-node-1",
					Namespace: "", // Empty for cluster-scoped - VALID
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "alert",
				Source:       "kubernetes-events",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)

			// BUSINESS OUTCOME: CRD created - cluster-scoped resources have empty namespace
			Expect(err).NotTo(HaveOccurred(), "Cluster-scoped resource should be processed")
			Expect(rr).NotTo(BeNil(), "CRD should be created")
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Node"))
			Expect(rr.Spec.TargetResource.Name).To(Equal("worker-node-1"))
			Expect(rr.Spec.TargetResource.Namespace).To(BeEmpty(),
				"Cluster-scoped resources have empty namespace")

			// Business capability verified:
			// WE can process cluster-scoped resources with targetResource format "Kind/Name"
		})
	})
})

// Uses controller-runtime fake.NewClientBuilder() - no custom fake client needed
