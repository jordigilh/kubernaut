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

package reconstruction

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
// Test Plan: docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// This test validates the mapper component that maps parsed audit data to RR CRD fields.
var _ = Describe("Audit Event Mapper", func() {

	Context("MAPPER-GW-01: Map gateway audit data to RR Spec (Gaps #1-3)", func() {
		It("should map Signal, SignalLabels, SignalAnnotations to RR spec", func() {
			// TDD RED: Test mapping gateway audit data to RR spec fields
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:     "gateway.signal.received",
				SignalType:    "prometheus-alert",
				AlertName:     "HighCPU",
				SignalLabels:  map[string]string{"alertname": "HighCPU", "severity": "critical"},
				SignalAnnotations: map[string]string{"summary": "CPU usage is high"},
				OriginalPayload: `{"alert":"data"}`,
			}

			rrFields, err := reconstructionpkg.MapToRRFields(parsedData)

			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())
			Expect(rrFields.Spec).ToNot(BeNil())

			// Validate Signal field mapping
			Expect(rrFields.Spec.SignalName).To(Equal("HighCPU"))
			Expect(rrFields.Spec.SignalType).To(Equal("prometheus-alert"))

			// Validate SignalLabels mapping
			Expect(rrFields.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "HighCPU"))
			Expect(rrFields.Spec.SignalLabels).To(HaveKeyWithValue("severity", "critical"))

			// Validate SignalAnnotations mapping
			Expect(rrFields.Spec.SignalAnnotations).To(HaveKeyWithValue("summary", "CPU usage is high"))

			// Validate OriginalPayload mapping ([]byte)
			Expect(rrFields.Spec.OriginalPayload).To(Equal([]byte(`{"alert":"data"}`)))
		})

		It("should return error for missing required alert name", func() {
			// Validates error handling when alert name is missing
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:  "gateway.signal.received",
				SignalType: "prometheus-alert",
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
			gatewayData := &reconstructionpkg.ParsedAuditData{
				EventType:     "gateway.signal.received",
				SignalType:    "prometheus-alert",
				AlertName:     "HighMemory",
				SignalLabels:  map[string]string{"alertname": "HighMemory"},
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

	// NOTE: Additional mapper tests for Gaps #4-7 will be added during GREEN phase
	// when we implement workflow, AI provider, and error data mapping
})
