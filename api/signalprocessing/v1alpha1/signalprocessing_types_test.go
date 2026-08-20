/*
Copyright 2026 Jordi Gil.

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

package v1alpha1_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// Issue #2209: SignalProcessingStatus god-struct decomposition
// ========================================
// Authority: GO-ANTIPATTERN-AUDIT-2026-07-01 category 4b (behavioral god
// object). Mirrors the AIAnalysisStatus.RCAResult/Approval/Review/
// InvestigationMetadata pointer-bundle + Get*/Ensure* accessor pattern
// established by #2205 (api/aianalysis/v1alpha1/aianalysis_types.go).
//
// RED: SignalClassification/FailureInfo do not exist yet on
// SignalProcessingStatus -- this file must fail to compile until GREEN
// adds the sub-structs and accessors.
// ========================================
var _ = Describe("SignalProcessingStatus — Issue #2209 god-struct decomposition", func() {

	Describe("SignalClassification accessors", func() {
		It("UT-SP-2209-001: GetSignalClassification returns a zero-value struct when nil, without mutating Status", func() {
			status := v1alpha1.SignalProcessingStatus{}

			got := status.GetSignalClassification()

			Expect(got).ToNot(BeNil())
			Expect(*got).To(Equal(v1alpha1.SignalClassification{}))
			Expect(status.SignalClassification).To(BeNil(), "Get* must not initialize the field as a read-only accessor")
		})

		It("UT-SP-2209-002: EnsureSignalClassification initializes the field and returns the same pointer on repeated calls", func() {
			status := v1alpha1.SignalProcessingStatus{}

			first := status.EnsureSignalClassification()
			Expect(status.SignalClassification).ToNot(BeNil())

			first.Severity = "critical"
			second := status.EnsureSignalClassification()

			Expect(second).To(BeIdenticalTo(first), "Ensure* must return the existing pointer, not re-initialize it")
			Expect(second.Severity).To(Equal("critical"))
		})
	})

	Describe("FailureInfo accessors", func() {
		It("UT-SP-2209-003: GetFailureInfo returns a zero-value struct when nil, without mutating Status", func() {
			status := v1alpha1.SignalProcessingStatus{}

			got := status.GetFailureInfo()

			Expect(got).ToNot(BeNil())
			Expect(*got).To(Equal(v1alpha1.FailureInfo{}))
			Expect(status.FailureInfo).To(BeNil(), "Get* must not initialize the field as a read-only accessor")
		})

		It("UT-SP-2209-004: EnsureFailureInfo initializes the field and returns the same pointer on repeated calls", func() {
			status := v1alpha1.SignalProcessingStatus{}

			first := status.EnsureFailureInfo()
			Expect(status.FailureInfo).ToNot(BeNil())

			first.ConsecutiveFailures = 3
			second := status.EnsureFailureInfo()

			Expect(second).To(BeIdenticalTo(first), "Ensure* must return the existing pointer, not re-initialize it")
			Expect(second.ConsecutiveFailures).To(Equal(int32(3)))
		})
	})

	Describe("Backward-compatibility-relevant JSON shape (SOC2 CC7.4 / AU-3 audit-trail fields)", func() {
		It("UT-SP-2209-005: PolicyHash and SourceSignalName round-trip unchanged through the nested SignalClassification shape", func() {
			status := v1alpha1.SignalProcessingStatus{
				Phase: v1alpha1.PhaseCompleted,
				SignalClassification: &v1alpha1.SignalClassification{
					Severity:              v1alpha1.SeverityCritical,
					PolicyHash:            "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
					ClusterClassification: "production",
					SignalMode:            v1alpha1.SignalModeProactive,
					SignalName:            "OOMKilled",
					SourceSignalName:      "PredictedOOMKill",
				},
			}

			raw, err := json.Marshal(&status)
			Expect(err).ToNot(HaveOccurred())

			// Assert the wire shape is nested under "signalClassification", not
			// flat top-level fields — this is the shape change consumers must adapt to.
			var asMap map[string]interface{}
			Expect(json.Unmarshal(raw, &asMap)).To(Succeed())
			classification, ok := asMap["signalClassification"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "expected nested 'signalClassification' object in JSON output, got: %s", string(raw))
			Expect(classification["policyHash"]).To(Equal("a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9"))
			Expect(classification["sourceSignalName"]).To(Equal("PredictedOOMKill"))

			var roundTripped v1alpha1.SignalProcessingStatus
			Expect(json.Unmarshal(raw, &roundTripped)).To(Succeed())
			Expect(roundTripped.GetSignalClassification().PolicyHash).To(Equal(status.SignalClassification.PolicyHash))
			Expect(roundTripped.GetSignalClassification().SourceSignalName).To(Equal(status.SignalClassification.SourceSignalName))
		})
	})
})
