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

package reconstruction_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
// Test Plan: docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// This test validates the mapper component that maps parsed audit data to RR CRD fields.
var _ = Describe("Audit Event Mapper", func() {

	Context("MAPPER-GW-01: Map gateway audit data to RR Spec (Gaps #1-3)", func() {
		It("should map Signal, SignalLabels, SignalAnnotations, SignalFingerprint to RR spec", func() {
			// TDD RED: Test mapping gateway audit data to RR spec fields
			// BR-AUDIT-005: signalFingerprint is required for deduplication identity
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighCPU",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				SignalLabels:      map[string]string{"alertname": "HighCPU", "severity": "critical"},
				SignalAnnotations: map[string]string{"summary": "CPU usage is high"},
				OriginalPayload:   `{"alert":"data"}`,
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Spec).ToNot(BeNil())

			// Validate Signal field mapping
			Expect(rrFields.Spec.SignalName).To(Equal("HighCPU"))
			Expect(rrFields.Spec.SignalType).To(Equal("alert"))

			// Validate SignalFingerprint mapping (BR-AUDIT-005)
			Expect(rrFields.Spec.SignalFingerprint).To(Equal("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"),
				"Mapper must set spec.signalFingerprint from parsed gateway data")

			// Validate SignalLabels mapping
			Expect(rrFields.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "HighCPU"))
			Expect(rrFields.Spec.SignalLabels).To(HaveKeyWithValue("severity", "critical"))

			// Validate SignalAnnotations mapping
			Expect(rrFields.Spec.SignalAnnotations).To(HaveKeyWithValue("summary", "CPU usage is high"))

			// Validate OriginalPayload mapping (string, issue #96)
			Expect(rrFields.Spec.OriginalPayload).To(Equal(`{"alert":"data"}`))
		})

		It("should return error for missing required alert name", func() {
			// Validates error handling when alert name is missing
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:  "gateway.signal.received",
				SignalType: "alert",
				// AlertName is missing - should error
			}

			_, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alert name is required"))
		})
	})

	Context("MAPPER-RO-01: Map orchestrator audit data to RR Status (Gap #8)", func() {
		It("should map TimeoutConfig to RR status", func() {
			// Validates TimeoutConfig mapping to status field
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType: "orchestrator.lifecycle.created",
				TimeoutConfig: &reconstructionpkg.TimeoutConfigData{
					Global:     "1h0m0s",
					Processing: "30m0s",
					Analyzing:  "15m0s",
					Executing:  "45m0s",
				},
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Status).ToNot(BeNil())

			// Validate TimeoutConfig in status (metav1.Duration pointers)
			Expect(rrFields.Status.TimeoutConfig).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Global).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Global.Duration.String()).To(Equal("1h0m0s"))
			Expect(rrFields.Status.TimeoutConfig.Processing).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Processing.Duration.String()).To(Equal("30m0s"))
			Expect(rrFields.Status.TimeoutConfig.Analyzing).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Analyzing.Duration.String()).To(Equal("15m0s"))
			Expect(rrFields.Status.TimeoutConfig.Executing).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Executing.Duration.String()).To(Equal("45m0s"))
		})

		It("should handle partial TimeoutConfig", func() {
			// Validates optional TimeoutConfig fields
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType: "orchestrator.lifecycle.created",
				TimeoutConfig: &reconstructionpkg.TimeoutConfigData{
					Global: "1h0m0s",
					// Other fields omitted - should be empty strings
				},
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields.Status.TimeoutConfig.Global).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Global.Duration.String()).To(Equal("1h0m0s"))
			Expect(rrFields.Status.TimeoutConfig.Processing).To(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Analyzing).To(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Executing).To(BeNil())
		})
	})

	Context("MAPPER-MERGE-01: Merge multiple audit events", func() {
		It("should merge gateway and orchestrator data into single RR", func() {
			// Validates merging multiple audit events into complete RR
			// BR-AUDIT-005: signalFingerprint must survive merge
			gatewayData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighMemory",
				SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
				SignalLabels:      map[string]string{"alertname": "HighMemory"},
				SignalAnnotations: map[string]string{"summary": "Memory usage is high"},
			}

			orchestratorData := &reconstructionpkg.ParsedAuditData{
				EventType: "orchestrator.lifecycle.created",
				TimeoutConfig: &reconstructionpkg.TimeoutConfigData{
					Global: "2h0m0s",
				},
			}

			// Merge both events
			rrFields, err := reconstructionpkg.MergeAuditData([]reconstructionpkg.ParsedAuditData{*gatewayData, *orchestratorData})

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())

			// Validate gateway data in spec
			Expect(rrFields.Spec.SignalName).To(Equal("HighMemory"))
			Expect(rrFields.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "HighMemory"))
			// BR-AUDIT-005: signalFingerprint must be preserved through merge
			Expect(rrFields.Spec.SignalFingerprint).To(Equal("b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3"),
				"SignalFingerprint must survive merge from gateway event")

			// Validate orchestrator data in status
			Expect(rrFields.Status.TimeoutConfig.Global).ToNot(BeNil())
			Expect(rrFields.Status.TimeoutConfig.Global.Duration.String()).To(Equal("2h0m0s"))
		})

		It("should return error when gateway event is missing", func() {
			// Validates that gateway event is mandatory for reconstruction
			orchestratorData := &reconstructionpkg.ParsedAuditData{
				EventType: "orchestrator.lifecycle.created",
				TimeoutConfig: &reconstructionpkg.TimeoutConfigData{
					Global: "1h0m0s",
				},
			}

			_, err := reconstructionpkg.MergeAuditData([]reconstructionpkg.ParsedAuditData{*orchestratorData})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gateway.signal.received event is required"))
		})
	})

	// ========================================
	// MAPPER-CLUSTER-01: ClusterID → Spec.ClusterID mapping [CC8.1]
	// BR-AUDIT-005 v2.0 / DD-AUDIT-003 v2.2: Fleet cluster-scoped audit
	// ========================================
	Context("MAPPER-CLUSTER-01: Map ClusterID to RR Spec.ClusterID (DD-AUDIT-003 v2.2)", func() {
		It("should map ClusterID from gateway event to Spec.ClusterID [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighCPU",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				SignalLabels:      map[string]string{"alertname": "HighCPU"},
				ClusterID:         "prod-east",
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields.Spec.ClusterID).To(Equal("prod-east"),
				"CC8.1: Mapper must set Spec.ClusterID from ClusterID for fleet reconstruction")
		})

		It("should leave Spec.ClusterID empty for single-cluster events (backward compat)", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighCPU",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				SignalLabels:      map[string]string{"alertname": "HighCPU"},
				// ClusterID intentionally empty (single-cluster)
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields.Spec.ClusterID).To(BeEmpty(),
				"Single-cluster events must not populate Spec.ClusterID")
		})
	})

	Context("MAPPER-MERGE-CLUSTER-01: ClusterID survives merge (DD-AUDIT-003 v2.2)", func() {
		It("should preserve ClusterID through merge of gateway + orchestrator events [CC8.1]", func() {
			gatewayData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighMemory",
				SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
				SignalLabels:      map[string]string{"alertname": "HighMemory"},
				ClusterID:         "prod-west",
			}

			orchestratorData := &reconstructionpkg.ParsedAuditData{
				EventType: "orchestrator.lifecycle.created",
				TimeoutConfig: &reconstructionpkg.TimeoutConfigData{
					Global: "2h0m0s",
				},
				ClusterID: "prod-west",
			}

			rrFields, err := reconstructionpkg.MergeAuditData(
				[]reconstructionpkg.ParsedAuditData{*gatewayData, *orchestratorData},
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields.Spec.ClusterID).To(Equal("prod-west"),
				"CC8.1: ClusterID must survive merge for fleet RR reconstruction")
		})
	})

	// ========================================
	// MAPPER-RO-COMPLETED: orchestrator.lifecycle.completed → Status mapping [CC8.1]
	// BR-AUDIT-005 v2.0: Outcome mapping for completed lifecycle events
	// ========================================
	Context("MAPPER-RO-COMPLETED: Map orchestrator.lifecycle.completed to RR Status [CC8.1]", func() {
		It("should map Outcome to Status.Outcome [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:  "orchestrator.lifecycle.completed",
				Outcome:    "success",
				DurationMs: 45000,
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Status).ToNot(BeNil())
			Expect(rrFields.Status.Outcome).To(Equal("success"),
				"CC8.1: Mapper must set Status.Outcome from completed event Outcome")
		})

		It("should not map DurationMs to any RR field (metadata only) [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:  "orchestrator.lifecycle.completed",
				Outcome:    "success",
				DurationMs: 120000,
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Status.Outcome).To(Equal("success"),
				"CC8.1: Outcome must still be mapped even when DurationMs is present")
			Expect(rrFields.Status.FailurePhase).To(BeNil(),
				"CC8.1: Completed event must not set FailurePhase")
			Expect(rrFields.Status.FailureReason).To(BeNil(),
				"CC8.1: Completed event must not set FailureReason")
		})
	})

	// ========================================
	// MAPPER-RO-FAILED: orchestrator.lifecycle.failed → Status mapping [CC8.1]
	// BR-AUDIT-005 v2.0: Failure fields mapping for failed lifecycle events
	// ========================================
	Context("MAPPER-RO-FAILED: Map orchestrator.lifecycle.failed to RR Status [CC8.1]", func() {
		It("should map FailurePhase to Status.FailurePhase pointer [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:    "orchestrator.lifecycle.failed",
				Outcome:      "failure",
				FailurePhase: "execution",
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Status).ToNot(BeNil())
			Expect(rrFields.Status.FailurePhase).ToNot(BeNil(),
				"CC8.1: Mapper must set Status.FailurePhase pointer from failed event")
			Expect(*rrFields.Status.FailurePhase).To(Equal(remediationv1.FailurePhase("execution")),
				"CC8.1: Status.FailurePhase must match parsed FailurePhase value")
		})

		It("should map ErrorDetails.Message to Status.FailureReason pointer [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:    "orchestrator.lifecycle.failed",
				Outcome:      "failure",
				FailurePhase: "analysis",
				ErrorDetails: &reconstructionpkg.ErrorDetailsData{
					Message:       "AI analysis timed out after 15m",
					Code:          "TIMEOUT",
					Component:     "ai-analysis",
					RetryPossible: true,
				},
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Status.FailureReason).ToNot(BeNil(),
				"CC8.1: Mapper must set Status.FailureReason pointer from ErrorDetails.Message")
			Expect(*rrFields.Status.FailureReason).To(Equal("AI analysis timed out after 15m"),
				"CC8.1: Status.FailureReason must match ErrorDetails.Message")
		})

		It("should map Outcome to Status.Outcome [CC8.1]", func() {
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:    "orchestrator.lifecycle.failed",
				Outcome:      "failure",
				FailurePhase: "execution",
				ErrorDetails: &reconstructionpkg.ErrorDetailsData{
					Message: "remediation script returned non-zero exit code",
				},
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields.Status.Outcome).To(Equal("failure"),
				"CC8.1: Mapper must set Status.Outcome from failed event Outcome")
		})
	})

	// ========================================
	// MAPPER-MERGE-FAILURE: FailurePhase/FailureReason survive merge [CC8.1]
	// BR-AUDIT-005 v2.0: Failure fields must be preserved through merge
	// ========================================
	Context("MAPPER-MERGE-FAILURE: FailurePhase/FailureReason survive merge [CC8.1]", func() {
		It("should preserve FailurePhase and FailureReason through merge of gateway + failed orchestrator events [CC8.1]", func() {
			gatewayData := &reconstructionpkg.ParsedAuditData{
				EventType:         "gateway.signal.received",
				SignalType:        "alert",
				SignalName:        "HighCPU",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				SignalLabels:      map[string]string{"alertname": "HighCPU"},
			}

			failedData := &reconstructionpkg.ParsedAuditData{
				EventType:    "orchestrator.lifecycle.failed",
				Outcome:      "failure",
				FailurePhase: "verification",
				ErrorDetails: &reconstructionpkg.ErrorDetailsData{
					Message: "post-remediation verification failed: service still unhealthy",
				},
			}

			rrFields, err := reconstructionpkg.MergeAuditData(
				[]reconstructionpkg.ParsedAuditData{*gatewayData, *failedData},
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())

			// Validate gateway spec data survived merge
			Expect(rrFields.Spec.SignalName).To(Equal("HighCPU"),
				"CC8.1: Gateway spec data must survive merge with failed event")

			// Validate failure fields survived merge
			Expect(rrFields.Status.Outcome).To(Equal("failure"),
				"CC8.1: Outcome must survive merge from failed orchestrator event")
			Expect(rrFields.Status.FailurePhase).ToNot(BeNil(),
				"CC8.1: FailurePhase must survive merge from failed orchestrator event")
			Expect(*rrFields.Status.FailurePhase).To(Equal(remediationv1.FailurePhase("verification")),
				"CC8.1: FailurePhase value must be preserved through merge")
			Expect(rrFields.Status.FailureReason).ToNot(BeNil(),
				"CC8.1: FailureReason must survive merge from failed orchestrator event")
			Expect(*rrFields.Status.FailureReason).To(Equal("post-remediation verification failed: service still unhealthy"),
				"CC8.1: FailureReason value must be preserved through merge")
		})
	})
})
