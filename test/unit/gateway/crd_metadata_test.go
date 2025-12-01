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
	"github.com/go-logr/logr"
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
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

		// Create fake K8s client per ADR-004: Fake Kubernetes Client for Unit Testing
		// Uses controller-runtime fake client for compile-time safety and maintained interface
		scheme := runtime.NewScheme()
		_ = remediationv1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		fakeK8sClient = k8s.NewClient(fakeClient)
		retryConfig := config.DefaultRetrySettings()
		crdCreator = processing.NewCRDCreator(fakeK8sClient, logger, nil, "default", &retryConfig)
	})

	// BUSINESS CAPABILITY: Notification service needs complete context to alert humans
	Context("when creating CRD for notification service consumption", func() {
		It("enables notification service to identify WHAT failed for alert content", func() {
			// BUSINESS SCENARIO: PagerDuty needs to tell on-call engineer:
			// "Alert: payment-api pod crashed in production"
			// Required fields: signalName, severity, namespace, resource details

			signal := &types.NormalizedSignal{
				Fingerprint: "abc123def456ghi789jkl012mno345", // Must be >=16 chars
				AlertName:   "PodCrashLooping",
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
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{"alerts":[{"status":"firing"}]}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Notification has alert name and severity
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.SignalName).To(Equal("PodCrashLooping"),
				"Notification service needs alertname for PagerDuty title")
			Expect(rr.Spec.Severity).To(Equal("critical"),
				"Notification service needs severity for PagerDuty priority")
			Expect(rr.Namespace).To(Equal("production"),
				"Notification service needs namespace for incident context")

			// Business capability verified:
			// PagerDuty alert: "🚨 Critical: payment-api pod crashed in production"
		})

		It("enables notification service to identify WHO to alert based on priority", func() {
			// BUSINESS SCENARIO: Notification routing based on priority
			// P0 → Page on-call engineer immediately (phone call)
			// P1 → Slack alert to team channel
			// P2 → Email digest (batched)
			// Required field: priority

			signal := &types.NormalizedSignal{
				Fingerprint:  "xyz789abc123def456ghi789jkl012", // Must be >=16 chars
				AlertName:    "DatabaseConnectionPoolExhausted",
				Severity:     "critical",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: CRD has priority for notification routing
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.Priority).To(Equal("P0"),
				"Notification service needs priority to decide: phone call vs Slack vs email")
			Expect(rr.Labels["kubernaut.io/priority"]).To(Equal("P0"),
				"Label enables notification service to filter/query by priority")

			// Business capability verified:
			// Notification service: P0 label → Phone call to on-call engineer
		})

		It("enables notification service to provide WHEN context for incident timeline", func() {
			// BUSINESS SCENARIO: On-call engineer needs timeline:
			// "Alert started firing at 14:05 UTC, Gateway received at 14:06 UTC"
			// Required fields: firingTime, receivedTime

			firingTime := time.Date(2025, 10, 10, 14, 5, 0, 0, time.UTC)
			receivedTime := time.Date(2025, 10, 10, 14, 6, 30, 0, time.UTC)

			signal := &types.NormalizedSignal{
				Fingerprint:  "timeline123abc456def789ghi012jk", // Must be >=16 chars
				AlertName:    "HighLatency",
				Severity:     "warning",
				Namespace:    "production",
				FiringTime:   firingTime,
				ReceivedTime: receivedTime,
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P1", "production")

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

		It("enables notification service to include WHERE context for incident location", func() {
			// BUSINESS SCENARIO: On-call engineer needs to know:
			// "Issue in production environment (not staging/dev)"
			// Required field: environment

			signal := &types.NormalizedSignal{
				Fingerprint:  "location456abc789def012ghi345jkl", // Must be >=16 chars
				AlertName:    "HighMemoryUsage",
				Severity:     "critical",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Notification has environment for incident context
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.Environment).To(Equal("production"),
				"Notification service needs environment to emphasize urgency")
			Expect(rr.Labels["kubernaut.io/environment"]).To(Equal("production"),
				"Label enables notification service to filter production vs staging alerts")

			// Business capability verified:
			// PagerDuty: "🔴 PRODUCTION: High memory usage detected"
			// (vs staging: "🟡 STAGING: High memory usage detected")
		})

		It("enables notification service to show ORIGINAL alert details for engineer investigation", func() {
			// BUSINESS SCENARIO: On-call engineer clicks PagerDuty link, sees:
			// - Full Prometheus labels (instance, job, container, etc.)
			// - Alert annotations (description, runbook URL, dashboard link)
			// - Original alert payload (for debugging false positives)
			// Required fields: signalLabels, signalAnnotations, originalPayload

			signal := &types.NormalizedSignal{
				Fingerprint: "details789abc123def456ghi789jkl", // Must be >=16 chars
				AlertName:   "PodMemoryHigh",
				Severity:    "critical",
				Namespace:   "production",
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
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{"alerts":[{"labels":{"alertname":"PodMemoryHigh"},"annotations":{"summary":"Pod memory > 90%"}}]}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

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
			err = json.Unmarshal(rr.Spec.OriginalPayload, &payload)
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
				Fingerprint:  "storm123abc456def789ghi012jkl34", // Must be >=16 chars
				AlertName:    "PodCrashLooping",
				Severity:     "critical",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
				// Storm metadata
				IsStorm:     true,
				StormType:   "rate-based-same-alertname",
				StormWindow: "1m",
				AlertCount:  50,
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Notification shows storm aggregation
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.IsStorm).To(BeTrue(),
				"Notification service knows to aggregate: '1 storm' not '50 alerts'")
			Expect(rr.Spec.StormAlertCount).To(Equal(50),
				"PagerDuty shows: '⚡ 50 related alerts aggregated'")
			Expect(rr.Spec.StormType).To(ContainSubstring("rate-based"),
				"Notification explains WHY aggregated: 'rate-based storm detected'")
			Expect(rr.Spec.StormWindow).To(Equal("1m"),
				"PagerDuty shows: '50 alerts in 1 minute window'")

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
				Fingerprint:  "recurring456abc789def012ghi345jk", // Must be >=16 chars
				AlertName:    "DiskSpaceRunningOut",
				Severity:     "warning",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P1", "production")

			// BUSINESS OUTCOME: CRD has deduplication metadata
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.Deduplication.FirstSeen.Time).NotTo(BeZero(),
				"Notification service can show: 'Issue first detected at 14:00 UTC'")
			Expect(rr.Spec.Deduplication.LastSeen.Time).NotTo(BeZero(),
				"Notification service can show: 'Last occurred at 14:10 UTC'")
			Expect(rr.Spec.Deduplication.OccurrenceCount).To(Equal(1),
				"Initial occurrence count = 1, incremented on subsequent alerts")

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
					Fingerprint:  "fp-long-label",
					AlertName:    "HighMemory",
					Severity:     "critical",
					Namespace:    "production",
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					SourceType:   "prometheus-alert",
					Source:       "prometheus-adapter",
					RawPayload:   json.RawMessage(`{}`),
					Labels: map[string]string{
						"environment": longLabelValue, // >63 chars
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

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
					Fingerprint:  "fp-large-annotation",
					AlertName:    "HighMemory",
					Severity:     "critical",
					Namespace:    "production",
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					SourceType:   "prometheus-alert",
					Source:       "prometheus-adapter",
					RawPayload:   json.RawMessage(`{}`),
					Annotations: map[string]string{
						"description": string(largeAnnotation), // >256KB
					},
				}

				rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

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

// BR-GATEWAY-TARGET-RESOURCE: Target Resource Accessibility
// Business Outcome: SignalProcessing and RO can access resource info WITHOUT JSON parsing
// Reference: RESPONSE_TARGET_RESOURCE_SCHEMA.md - Option A approved
var _ = Describe("BR-GATEWAY-TARGET-RESOURCE: Target Resource in RemediationRequest CRD", func() {
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

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		fakeK8sClient = k8s.NewClient(fakeClient)
		retryConfig := config.DefaultRetrySettings()
		crdCreator = processing.NewCRDCreator(fakeK8sClient, logger, nil, "default", &retryConfig)
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BUSINESS CAPABILITY: Direct resource access for downstream services
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when creating CRD for SignalProcessing and RO consumption", func() {

		It("enables SignalProcessing to access resource Kind directly for context enrichment", func() {
			// BUSINESS SCENARIO: SignalProcessing needs resource Kind to:
			// - Query K8s API for resource-specific context (Pod logs, Deployment status)
			// - Choose appropriate enrichment strategy per resource type
			// Required: spec.targetResource.kind directly accessible (no JSON parsing)

			signal := &types.NormalizedSignal{
				Fingerprint: "target-resource-kind-test-123456",
				AlertName:   "PodCrashLooping",
				Severity:    "critical",
				Namespace:   "production",
				Resource: types.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "payment-api-789",
					Namespace: "production",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Resource Kind is directly accessible
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource).NotTo(BeNil(),
				"SignalProcessing MUST be able to access targetResource without nil check failures")
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Pod"),
				"SignalProcessing can access rr.Spec.TargetResource.Kind directly - no JSON parsing needed")

			// Business capability verified:
			// SignalProcessing: rr.Spec.TargetResource.Kind == "Pod" → query Pod logs
		})

		It("enables RO to access resource Name directly for workflow routing", func() {
			// BUSINESS SCENARIO: RO needs resource Name to:
			// - Include in workflow execution context
			// - Route to resource-specific remediation workflows
			// Required: spec.targetResource.name directly accessible (no JSON parsing)

			signal := &types.NormalizedSignal{
				Fingerprint: "target-resource-name-test-123456",
				AlertName:   "HighMemoryUsage",
				Severity:    "warning",
				Namespace:   "staging",
				Resource: types.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "checkout-service",
					Namespace: "staging",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P1", "staging")

			// BUSINESS OUTCOME: Resource Name is directly accessible
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource).NotTo(BeNil(),
				"RO MUST be able to access targetResource without nil check failures")
			Expect(rr.Spec.TargetResource.Name).To(Equal("checkout-service"),
				"RO can access rr.Spec.TargetResource.Name directly - no JSON parsing needed")

			// Business capability verified:
			// RO: rr.Spec.TargetResource.Name == "checkout-service" → include in workflow context
		})

		It("enables SignalProcessing to access resource Namespace for K8s API queries", func() {
			// BUSINESS SCENARIO: SignalProcessing needs resource Namespace to:
			// - Scope K8s API queries (GetPod, GetDeployment)
			// - Enrich with namespace-level context (labels, annotations)
			// Required: spec.targetResource.namespace directly accessible (no JSON parsing)

			signal := &types.NormalizedSignal{
				Fingerprint: "target-resource-ns-test-1234567",
				AlertName:   "ReplicaSetUnavailable",
				Severity:    "critical",
				Namespace:   "prod-payments",
				Resource: types.ResourceIdentifier{
					Kind:      "ReplicaSet",
					Name:      "payment-api-v2-abc123",
					Namespace: "prod-payments",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Resource Namespace is directly accessible
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource).NotTo(BeNil(),
				"SignalProcessing MUST be able to access targetResource without nil check failures")
			Expect(rr.Spec.TargetResource.Namespace).To(Equal("prod-payments"),
				"SignalProcessing can access rr.Spec.TargetResource.Namespace directly - no JSON parsing needed")

			// Business capability verified:
			// SignalProcessing: client.Get(ctx, types.NamespacedName{
			//   Name: rr.Spec.TargetResource.Name,
			//   Namespace: rr.Spec.TargetResource.Namespace,
			// }, &pod)
		})

		It("provides complete ResourceIdentifier for downstream workflow execution", func() {
			// BUSINESS SCENARIO: Complete resource identification enables:
			// - SignalProcessing context enrichment
			// - RO workflow routing decisions
			// - WorkflowExecution targeting
			// Required: ALL fields (Kind, Name, Namespace) populated together

			signal := &types.NormalizedSignal{
				Fingerprint: "complete-resource-id-test-12345",
				AlertName:   "StatefulSetNotReady",
				Severity:    "critical",
				Namespace:   "database-tier",
				Resource: types.ResourceIdentifier{
					Kind:      "StatefulSet",
					Name:      "postgresql-primary",
					Namespace: "database-tier",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// BUSINESS OUTCOME: Complete ResourceIdentifier available
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource).NotTo(BeNil(),
				"Downstream services MUST have targetResource populated")

			// Verify complete identification
			Expect(rr.Spec.TargetResource.Kind).To(Equal("StatefulSet"),
				"Kind enables resource-type-specific workflows")
			Expect(rr.Spec.TargetResource.Name).To(Equal("postgresql-primary"),
				"Name enables specific resource targeting")
			Expect(rr.Spec.TargetResource.Namespace).To(Equal("database-tier"),
				"Namespace enables K8s API scoping")

			// Business capability verified:
			// WorkflowExecution can target: StatefulSet/postgresql-primary in database-tier
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// CORRECTNESS: No duplicate resource data in ProviderData
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when verifying ProviderData structure (no duplication)", func() {

		It("does NOT duplicate resource info in ProviderData (per RESPONSE_TARGET_RESOURCE_SCHEMA.md)", func() {
			// BUSINESS SCENARIO: Pre-release cleanup decision
			// Resource info is now in spec.targetResource (top-level field)
			// ProviderData should NOT contain redundant resource{} object
			// This reduces CRD size and prevents data inconsistency

			signal := &types.NormalizedSignal{
				Fingerprint: "no-duplicate-resource-test-1234",
				AlertName:   "NodeNotReady",
				Severity:    "critical",
				Namespace:   "kube-system",
				Resource: types.ResourceIdentifier{
					Kind:      "Node",
					Name:      "worker-node-3",
					Namespace: "", // Cluster-scoped resource
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P0", "production")

			// CORRECTNESS: ProviderData does NOT contain resource duplication
			Expect(err).NotTo(HaveOccurred())

			// Parse ProviderData to verify no resource{} object
			var providerData map[string]interface{}
			err = json.Unmarshal(rr.Spec.ProviderData, &providerData)
			Expect(err).NotTo(HaveOccurred(), "ProviderData should be valid JSON")

			// Verify NO resource{} duplication
			Expect(providerData).NotTo(HaveKey("resource"),
				"ProviderData should NOT contain resource{} - it's now in spec.targetResource")

			// Correctness verified:
			// - spec.targetResource has the data (single source of truth)
			// - ProviderData is lean (no duplication)
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASES: Handle missing/empty resource info gracefully
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when handling edge cases for resource identification", func() {

		It("handles cluster-scoped resources (empty namespace) correctly", func() {
			// BUSINESS SCENARIO: Node alerts don't have namespace
			// SignalProcessing must handle empty namespace gracefully

			signal := &types.NormalizedSignal{
				Fingerprint: "cluster-scoped-resource-test-12",
				AlertName:   "NodeDiskPressure",
				Severity:    "warning",
				Namespace:   "default", // CRD namespace, not resource namespace
				Resource: types.ResourceIdentifier{
					Kind:      "Node",
					Name:      "worker-node-5",
					Namespace: "", // Cluster-scoped - no namespace
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P1", "production")

			// BUSINESS OUTCOME: Cluster-scoped resource handled correctly
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.TargetResource).NotTo(BeNil(),
				"Cluster-scoped resources should still have targetResource populated")
			Expect(rr.Spec.TargetResource.Kind).To(Equal("Node"),
				"Kind is populated for cluster-scoped resources")
			Expect(rr.Spec.TargetResource.Name).To(Equal("worker-node-5"),
				"Name is populated for cluster-scoped resources")
			Expect(rr.Spec.TargetResource.Namespace).To(BeEmpty(),
				"Namespace is empty for cluster-scoped resources (Node, PV, etc.)")

			// Business capability verified:
			// SignalProcessing: if rr.Spec.TargetResource.Namespace == "" → cluster-scoped query
		})

		It("handles signals without resource info (nil TargetResource)", func() {
			// BUSINESS SCENARIO: Some signals may not have specific resource targets
			// (e.g., cluster-wide alerts, external system alerts)
			// SignalProcessing should handle nil TargetResource gracefully

			signal := &types.NormalizedSignal{
				Fingerprint: "no-resource-signal-test-123456",
				AlertName:   "ClusterCPUHigh",
				Severity:    "warning",
				Namespace:   "monitoring",
				Resource: types.ResourceIdentifier{
					// All fields empty - no specific resource target
					Kind:      "",
					Name:      "",
					Namespace: "",
				},
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
				SourceType:   "prometheus-alert",
				Source:       "prometheus-adapter",
				RawPayload:   json.RawMessage(`{}`),
			}

			rr, err := crdCreator.CreateRemediationRequest(ctx, signal, "P2", "production")

			// BUSINESS OUTCOME: Signal without resource info handled gracefully
			Expect(err).NotTo(HaveOccurred())

			// TargetResource should be nil when no resource info is available
			Expect(rr.Spec.TargetResource).To(BeNil(),
				"Signals without resource info should have nil TargetResource (not empty struct)")

			// Business capability verified:
			// SignalProcessing: if rr.Spec.TargetResource == nil → skip resource-specific enrichment
		})
	})
})
