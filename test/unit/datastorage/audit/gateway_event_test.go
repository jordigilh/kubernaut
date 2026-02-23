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

package audit

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// ========================================
// TDD RED PHASE: Gateway Event Builder Tests
// BR-STORAGE-033: Event Data Helpers
// ========================================
//
// These tests define the contract for the Gateway event builder.
// Gateway Service uses this builder to create audit events for:
// - Signal ingestion (Prometheus AlertManager, K8s Events)
// - Deduplication decisions
// - Storm detection
// - Environment classification
// - Priority assignment
//
// Business Requirements:
// - BR-STORAGE-033-004: Gateway-specific event data structure
// - BR-STORAGE-033-005: Support for Prometheus and K8s Event signals
// - BR-STORAGE-033-006: Deduplication and storm metadata tracking
//
// Testing Principle: Behavior + Correctness
// ========================================

var _ = Describe("GatewayEventBuilder", func() {
	Context("BR-STORAGE-033-004: Gateway-specific event data structure", func() {
		// BEHAVIOR: Builder creates gateway event with correct service identifier
		// CORRECTNESS: Service is "gateway", version is "1.0"
		It("should create gateway event with base structure", func() {
			// TDD RED: This test will FAIL until we implement GatewayEventBuilder
			builder := audit.NewGatewayEvent("signal.received")
			Expect(builder).To(BeAssignableToTypeOf(&audit.GatewayEventBuilder{}))

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("version"))
			Expect(eventData["version"]).To(Equal("1.0"))
			Expect(eventData).To(HaveKey("service"))
			Expect(eventData["service"]).To(Equal("gateway"))
			Expect(eventData).To(HaveKey("event_type"))
			Expect(eventData["event_type"]).To(Equal("signal.received"))
		})

		It("should support fluent API for all Gateway fields", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("HighMemoryUsage").
				WithFingerprint("sha256:abc123").
				WithNamespace("production").
				WithResource("pod", "api-server-123").
				WithSeverity("critical").
				WithPriority("P0").
				WithEnvironment("production").
				WithDeduplicationStatus("new")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))
		})

		It("should include gateway-specific data in nested structure", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("PodOOMKilled")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("gateway"))

			gatewayData, ok := data["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(gatewayData).To(HaveKeyWithValue("signal_type", "alert"))
			Expect(gatewayData).To(HaveKeyWithValue("alert_name", "PodOOMKilled"))
		})
	})

	Context("BR-STORAGE-033-005: Support for Prometheus and K8s Event signals", func() {
		It("should build Prometheus signal event", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("HighMemoryUsage").
				WithFingerprint("sha256:prometheus-123").
				WithNamespace("production").
				WithResource("pod", "api-server-xyz").
				WithSeverity("warning").
				WithLabels(map[string]string{
					"app":  "api-server",
					"tier": "backend",
				})

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("signal_type", "alert"))
			Expect(gatewayData).To(HaveKeyWithValue("alert_name", "HighMemoryUsage"))
			Expect(gatewayData).To(HaveKeyWithValue("fingerprint", "sha256:prometheus-123"))
			Expect(gatewayData).To(HaveKeyWithValue("namespace", "production"))
			Expect(gatewayData).To(HaveKeyWithValue("resource_type", "pod"))
			Expect(gatewayData).To(HaveKeyWithValue("resource_name", "api-server-xyz"))
			Expect(gatewayData).To(HaveKeyWithValue("severity", "warning"))
			Expect(gatewayData).To(HaveKey("labels"))
		})

		It("should build Kubernetes Event signal", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("OOMKilled").
				WithFingerprint("sha256:k8s-event-456").
				WithNamespace("production").
				WithResource("pod", "database-pod-123").
				WithSeverity("critical")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("signal_type", "alert"))
			Expect(gatewayData).To(HaveKeyWithValue("event_reason", "OOMKilled"))
			Expect(gatewayData).To(HaveKeyWithValue("fingerprint", "sha256:k8s-event-456"))
		})

		It("should support source payload for debugging", func() {
			originalPayload := "base64encodedpayload=="

			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithSourcePayload(originalPayload)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("source_payload", originalPayload))
		})
	})

	Context("BR-STORAGE-033-006: Deduplication and storm metadata tracking", func() {
		It("should track deduplication status", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithDeduplicationStatus("duplicate")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("deduplication_status", "duplicate"))
		})

		It("should track storm detection", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("PodCrashLoop").
				WithStorm("storm-2025-11-18-001")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("storm_detected", true))
			Expect(gatewayData).To(HaveKeyWithValue("storm_id", "storm-2025-11-18-001"))
		})

		It("should not include storm_id if no storm detected", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			// storm_detected should default to false
			stormDetected, ok := gatewayData["storm_detected"]
			if ok {
				Expect(stormDetected).To(BeFalse())
			}

			// storm_id should not be present or be empty
			stormID, ok := gatewayData["storm_id"]
			if ok {
				Expect(stormID).To(BeEmpty())
			}
		})
	})

	Context("Gateway-specific field validation", func() {
		It("should track environment classification", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithEnvironment("staging")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("environment", "staging"))
		})

		It("should track priority assignment", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithPriority("P1")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("priority", "P1"))
		})

		It("should support resource type and name", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("FailedScheduling").
				WithResource("node", "worker-node-01")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			Expect(gatewayData).To(HaveKeyWithValue("resource_type", "node"))
			Expect(gatewayData).To(HaveKeyWithValue("resource_name", "worker-node-01"))
		})

		It("should support additional labels", func() {
			labels := map[string]string{
				"cluster":     "prod-us-west-2",
				"environment": "production",
				"team":        "platform",
			}

			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithLabels(labels)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			labelsData, ok := gatewayData["labels"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(labelsData).To(HaveKeyWithValue("cluster", "prod-us-west-2"))
			Expect(labelsData).To(HaveKeyWithValue("environment", "production"))
			Expect(labelsData).To(HaveKeyWithValue("team", "platform"))
		})
	})

	Context("Edge Cases", func() {
		It("should handle minimal Gateway event (only signal type)", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("prometheus")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))
		})

		It("should handle empty alert name", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			// Empty signal_name should not be present or be empty string
			alertName, ok := gatewayData["signal_name"]
			if ok {
				Expect(alertName).To(BeEmpty())
			}
		})

		It("should handle nil labels", func() {
			builder := audit.NewGatewayEvent("signal.received").
				WithSignalType("alert").
				WithSignalName("TestAlert").
				WithLabels(nil)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			gatewayData, _ := data["gateway"].(map[string]interface{})

			// Labels should not be present or be nil
			labels, ok := gatewayData["labels"]
			if ok {
				Expect(labels).To(BeNil())
			}
		})
	})
})
