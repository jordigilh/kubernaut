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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// Business Outcome Testing: Test WHAT the K8s Event Adapter enables, not HOW it parses
//
// ❌ WRONG: "should extract reason field from JSON" (tests implementation)
// ✅ RIGHT: "identifies Pod OOM failures for AI remediation" (tests business outcome)

var _ = Describe("BR-GATEWAY-005: Kubernetes Event Adapter", func() {
	var (
		adapter *adapters.KubernetesEventAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewKubernetesEventAdapter()
		ctx = context.Background()
	})

	// BUSINESS OUTCOME: AI needs to identify which resource failed for targeted remediation
	// Without this capability, AI cannot determine WHERE to apply fixes
	Describe("Resource identification for remediation targeting", func() {
		It("identifies Pod failures for remediation (OOMKilled scenario)", func() {
			// Business scenario: Pod killed due to memory limit
			// Expected: AI can restart pod, adjust memory limits, or scale down
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"message": "Container killed due to memory limit",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "production",
					"name": "payment-api-789"
				}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)

			// BUSINESS OUTCOME: Gateway extracts resource identity for AI targeting
			Expect(err).NotTo(HaveOccurred(),
				"Valid K8s events must be parseable for remediation")

			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"AI needs resource KIND to choose remediation strategy (restart vs scale)")
			Expect(signal.Resource.Name).To(Equal("payment-api-789"),
				"AI needs resource NAME for kubectl targeting: 'kubectl delete pod payment-api-789'")
			Expect(signal.Namespace).To(Equal("production"),
				"AI needs NAMESPACE for kubectl context: 'kubectl -n production'")
			Expect(signal.SignalName).To(Equal("OOMKilled"),
				"AI needs alert name to understand failure type")

			// Business capability verified:
			// K8s Event → Gateway → AI can identify WHAT resource needs remediation
		})

		It("identifies Node failures for cluster-level remediation", func() {
			// Business scenario: Node running out of disk space
			// Expected: AI can cordon node, drain pods, or alert ops team
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "DiskPressure",
				"message": "Node has insufficient disk space",
				"involvedObject": {
					"kind": "Node",
					"name": "worker-node-3"
				}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)

			// BUSINESS OUTCOME: AI handles cluster-scoped resources (not just namespaced)
			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Node"),
				"AI can remediate cluster-level resources")
			Expect(signal.Resource.Name).To(Equal("worker-node-3"))
			Expect(signal.Namespace).To(BeEmpty(),
				"Nodes are cluster-scoped, no namespace required")

			// Business capability verified:
			// K8s Event → Gateway → AI can handle both namespaced and cluster-scoped resources
		})

		It("identifies Deployment failures for rollback remediation", func() {
			// Business scenario: Deployment rollout stuck due to image pull failure
			// Expected: AI can trigger rollback to previous working version
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "FailedCreate",
				"message": "Failed to create pod for deployment: ImagePullBackOff",
				"involvedObject": {
					"kind": "Deployment",
					"namespace": "staging",
					"name": "api-service"
				}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)

			// BUSINESS OUTCOME: AI can trigger deployment rollbacks
			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"AI chooses rollback strategy for Deployments")
			Expect(signal.SignalName).To(Equal("FailedCreate"),
				"AI needs failure type to determine root cause")

			// Business capability verified:
			// K8s Event → Gateway → AI can trigger 'kubectl rollout undo deployment/api-service'
		})
	})

	// BUSINESS OUTCOME: Filter noise to prevent overwhelming AI with normal operations
	// K8s generates ~1000 events/minute in production, most are informational
	Describe("Event type filtering to reduce AI analysis costs", func() {
		It("processes Warning events for remediation workflow", func() {
			// Business scenario: Warning events indicate issues needing attention
			// Expected: Gateway passes through event type → SignalProcessing normalizes
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			warningEvent := []byte(`{
				"type": "Warning",
				"reason": "BackOff",
				"message": "Back-off restarting failed container",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "production",
					"name": "crashloop-pod"
				}
			}`)

			signal, err := adapter.Parse(ctx, warningEvent)

			// BUSINESS OUTCOME: K8s event type passed through as-is
			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event type 'Warning' passed through (not normalized to 'warning')")

			// Business capability verified:
			// Warning event → Gateway (pass-through) → CRD → SignalProcessing (normalize) → AI analyzes
		})

		It("skips Normal events to avoid creating CRDs for routine operations", func() {
			// Business scenario: Normal events = routine operations (Pod started, config updated)
			// Expected: Gateway filters out to prevent 100+ CRDs/minute for normal ops
			normalEvent := []byte(`{
				"type": "Normal",
				"reason": "Started",
				"message": "Container started successfully",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "production",
					"name": "healthy-pod"
				}
			}`)

			signal, err := adapter.Parse(ctx, normalEvent)

			// BUSINESS OUTCOME: Normal events don't waste AI analysis resources
			Expect(err).To(HaveOccurred(),
				"Normal events should be rejected to avoid noise")
			Expect(err.Error()).To(ContainSubstring("normal events not processed"),
				"Error message helps debugging")
			Expect(signal).To(BeNil(),
				"No signal created = no CRD = AI not invoked")

			// Business capability verified:
			// Normal event → Gateway filters → No CRD → AI resources saved
		})
	})

	// BUSINESS OUTCOME: Validation prevents incomplete remediation requests
	// Missing fields → AI cannot determine what to fix → Failed remediation
	DescribeTable("Event validation prevents incomplete remediation requests",
		func(eventJSON string, expectedError string, businessReason string) {
			signal, err := adapter.Parse(ctx, []byte(eventJSON))

			// BUSINESS OUTCOME: Invalid events rejected before expensive AI analysis
			Expect(err).To(HaveOccurred(), businessReason)
			Expect(err.Error()).To(ContainSubstring(expectedError),
				"Error messages guide troubleshooting")
			Expect(signal).To(BeNil(),
				"Invalid event should not create signal")

			// Business capability verified:
			// Invalid event → Gateway validation fails → No CRD → AI not wasted
		},

		// Scenario 1: Missing resource identification
		Entry("missing involvedObject → AI cannot target remediation",
			`{"type": "Warning", "reason": "Failed", "message": "Something failed"}`,
			"missing involvedObject",
			"AI needs resource to know WHERE to apply remediation"),

		// Scenario 2: Missing failure reason
		Entry("missing reason → AI cannot understand failure type",
			`{"type": "Warning", "involvedObject": {"kind": "Pod", "name": "test"}}`,
			"missing reason",
			"AI needs failure reason for root cause analysis"),

		// Scenario 3: Malformed JSON
		Entry("malformed JSON → cannot parse event",
			`{invalid json structure`,
			"invalid JSON",
			"Malformed data should not crash Gateway"),

		// Scenario 4: Missing resource kind
		Entry("missing kind → AI cannot choose remediation strategy",
			`{"type": "Warning", "reason": "Failed", "involvedObject": {"name": "test"}}`,
			"missing involvedObject.kind field",
			"AI needs resource type to select appropriate remediation (Pod restart vs Deployment rollback)"),

		// Scenario 5: Missing resource name
		Entry("missing name → AI cannot target kubectl commands",
			`{"type": "Warning", "reason": "Failed", "involvedObject": {"kind": "Pod"}}`,
			"missing involvedObject.name field",
			"AI needs resource name for kubectl targeting"),
	)

	// BUSINESS OUTCOME: Signal normalization enables uniform downstream processing
	// Different sources (Prometheus, K8s Events) → Unified format → AI doesn't need source-specific logic
	Describe("Signal normalization for uniform AI processing", func() {
		It("converts K8s Event to NormalizedSignal format for downstream compatibility", func() {
			// Business scenario: Gateway passes through K8s event type
			// SignalProcessing normalizes severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"message": "Container killed due to memory limit",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "production",
					"name": "payment-api-789"
				}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)

			// BUSINESS OUTCOME: Gateway extracts and preserves (not transforms)
			Expect(err).NotTo(HaveOccurred())

			// All adapters must populate these fields
			Expect(signal.SignalName).NotTo(BeEmpty(),
				"AlertName required for deduplication fingerprint")
			Expect(signal.Fingerprint).NotTo(BeEmpty(),
				"Fingerprint required for Redis deduplication")
			Expect(signal.Severity).NotTo(BeEmpty(),
				"BR-GATEWAY-181: Severity passed through (normalization happens in SignalProcessing)")
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: K8s event type passed through as-is")
			Expect(signal.SourceType).To(Equal("alert"),
				"Source type enables adapter-specific metrics")

			// Business capability verified:
			// K8s Event → Gateway (pass-through) → SignalProcessing (normalize) → AI processes
		})

		It("preserves original event payload for audit trail", func() {
			// Business scenario: Compliance requires 90-day audit logs
			// Expected: Original K8s Event stored in RemediationRequest CRD
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "BackOff",
				"involvedObject": {"kind": "Pod", "name": "test"}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)

			// BUSINESS OUTCOME: Audit trail enables compliance and debugging
			Expect(err).NotTo(HaveOccurred())
			Expect(signal.RawPayload).NotTo(BeEmpty(),
				"Original payload required for audit compliance")

			// Business capability verified:
			// K8s Event → Gateway preserves → Audit service can access original event
		})
	})

	// BUSINESS OUTCOME: Deduplication fingerprint prevents redundant remediation
	// Same event fired 5 times → 1 CRD created → AI analyzes once (not 5 times)
	Describe("Deduplication fingerprinting to prevent redundant workflows", func() {
		It("generates consistent fingerprints for identical events", func() {
			// Business scenario: Same pod OOM event fires twice in 1 minute
			// Expected: Same fingerprint → Redis deduplication → 1 CRD created
			event1 := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"involvedObject": {"kind": "Pod", "namespace": "prod", "name": "api"}
			}`)
			event2 := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"involvedObject": {"kind": "Pod", "namespace": "prod", "name": "api"}
			}`)

			signal1, _ := adapter.Parse(ctx, event1)
			signal2, _ := adapter.Parse(ctx, event2)

			// BUSINESS OUTCOME: Identical events produce same fingerprint for deduplication
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Same fingerprint enables Redis deduplication")

			// Business capability verified:
			// Duplicate event → Same fingerprint → Redis detects → No duplicate CRD
		})

		It("generates different fingerprints for different resources", func() {
			// Business scenario: Two different pods OOM at same time
			// Expected: Different fingerprints → 2 separate CRDs → AI analyzes both
			eventPod1 := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"involvedObject": {"kind": "Pod", "namespace": "prod", "name": "api-1"}
			}`)
			eventPod2 := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"involvedObject": {"kind": "Pod", "namespace": "prod", "name": "api-2"}
			}`)

			signal1, _ := adapter.Parse(ctx, eventPod1)
			signal2, _ := adapter.Parse(ctx, eventPod2)

			// BUSINESS OUTCOME: Different resources get separate remediation workflows
			Expect(signal1.Fingerprint).NotTo(Equal(signal2.Fingerprint),
				"Different fingerprints enable separate AI analysis")

			// Business capability verified:
			// Different resources → Different fingerprints → 2 CRDs → AI remediates both
		})
	})

	Context("BR-GATEWAY-027: Adapter Source Identification Methods", func() {
		It("GetSourceService() should return monitoring system name", func() {
			// BR-GATEWAY-027: Return monitoring system name for LLM tool selection
			// BUSINESS LOGIC: LLM uses signal_source to determine investigation tools
			// - "kubernetes-events" → LLM uses kubectl for investigation
			// - NOT "k8s-event-adapter" (internal implementation detail)

			adapter := adapters.NewKubernetesEventAdapter()

			sourceName := adapter.GetSourceService()

			Expect(sourceName).To(Equal("kubernetes-events"),
				"BR-GATEWAY-027: Must return monitoring system name, not adapter name")
			Expect(sourceName).NotTo(Equal("k8s-event-adapter"),
				"BR-GATEWAY-027: Adapter name is internal detail, not useful for LLM")
		})

		It("GetSourceType() should return signal type identifier", func() {
			// BUSINESS LOGIC: Signal type distinguishes alert sources for metrics/logging
			// Used for: metrics labels, logging, signal classification

			adapter := adapters.NewKubernetesEventAdapter()

			sourceType := adapter.GetSourceType()

			Expect(sourceType).To(Equal("alert"),
				"Must return signal type for classification")
		})

		It("Parse() should use GetSourceService() for signal.Source field", func() {
			// BR-GATEWAY-027: Ensure Parse() uses method instead of hardcoded value
			// BUSINESS LOGIC: Consistency between method and Parse() output

			adapter := adapters.NewKubernetesEventAdapter()
			k8sEvent := []byte(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "test",
					"name": "test-pod"
				}
			}`)

			signal, err := adapter.Parse(ctx, k8sEvent)
			Expect(err).NotTo(HaveOccurred())

			// Signal.Source must match GetSourceService()
			Expect(signal.Source).To(Equal(adapter.GetSourceService()),
				"BR-GATEWAY-027: Parse() must use GetSourceService() method")

			// Signal.SourceType must match GetSourceType()
			Expect(signal.SourceType).To(Equal(adapter.GetSourceType()),
				"Parse() must use GetSourceType() method")
		})
	})
})
