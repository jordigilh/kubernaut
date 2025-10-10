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

package gateway_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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
		logger        *logrus.Logger
		fakeK8sClient *FakeK8sClient // We'll use a fake since this is unit test
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(GinkgoWriter)
		logger.SetLevel(logrus.PanicLevel) // Suppress logs during tests
		ctx = context.Background()

		// Create fake K8s client for unit testing
		fakeClient := NewFakeControllerRuntimeClient()
		fakeK8sClient = NewFakeK8sClientWrapper(fakeClient)
		crdCreator = processing.NewCRDCreator(fakeK8sClient, logger)
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
	})
})

// FakeControllerRuntimeClient for unit testing (no real Kubernetes API calls)
type FakeControllerRuntimeClient struct {
	createdCRDs []*remediationv1alpha1.RemediationRequest
}

func NewFakeControllerRuntimeClient() *FakeControllerRuntimeClient {
	return &FakeControllerRuntimeClient{
		createdCRDs: make([]*remediationv1alpha1.RemediationRequest, 0),
	}
}

// Implement controller-runtime client.Client interface (minimal subset)
func (f *FakeControllerRuntimeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if rr, ok := obj.(*remediationv1alpha1.RemediationRequest); ok {
		f.createdCRDs = append(f.createdCRDs, rr)
	}
	return nil
}

// Stubs for other client.Client methods (not used in unit tests)
func (f *FakeControllerRuntimeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}
func (f *FakeControllerRuntimeClient) Status() client.SubResourceWriter {
	return nil
}
func (f *FakeControllerRuntimeClient) SubResource(subResource string) client.SubResourceClient {
	return nil
}
func (f *FakeControllerRuntimeClient) Scheme() *runtime.Scheme {
	return nil
}
func (f *FakeControllerRuntimeClient) RESTMapper() meta.RESTMapper {
	return nil
}
func (f *FakeControllerRuntimeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (f *FakeControllerRuntimeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return false, nil
}
func (f *FakeControllerRuntimeClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return nil
}

// FakeK8sClient wraps FakeControllerRuntimeClient (mimics pkg/gateway/k8s/Client)
type FakeK8sClient = k8s.Client

func NewFakeK8sClientWrapper(fakeClient *FakeControllerRuntimeClient) *FakeK8sClient {
	return k8s.NewClient(fakeClient)
}
