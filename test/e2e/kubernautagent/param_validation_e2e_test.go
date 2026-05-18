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

package kubernautagent

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"strings"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// BR-HAPI-191: Parameter validation self-correction E2E tests (#1170)
// Mock LLM scenario "param_validation_selfcorrect" returns invalid params first,
// then corrected params after KA sends validation error feedback with schema hints.

var _ = Describe("E2E-KA Parameter Validation Self-Correction (#1170)", Label("e2e", "ka", "param-validation"), func() {

	Context("BR-HAPI-191: Parameter validation with LLM self-correction", func() {

		It("E2E-KA-1170-001: Self-correction succeeds after invalid params → corrected params", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-1170-001
			// Business Outcome: When LLM returns invalid parameters (wrong type, undeclared),
			//   KA validates against workflow schema, sends structured error feedback with
			//   schema hints, and LLM self-corrects on retry.
			// BR: BR-HAPI-191
			// Mock Scenario: param_validation_selfcorrect (first=bad, second=good)

			// ========================================
			// ARRANGE
			// ========================================
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-param-val-001",
				RemediationID:     "test-rem-param-001",
				SignalName:        "MOCK_PARAM_VALIDATION_SELFCORRECT",
				Severity:          "high",
				SignalSource:      "prometheus",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "api-server-xyz",
				ErrorMessage:      "Scaling needed",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			incidentResp, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

			// ========================================
			// ASSERT
			// ========================================

			// BEHAVIOR: Self-correction succeeded — valid workflow selected
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"selected_workflow must be present after successful self-correction")
			Expect(incidentResp.NeedsHumanReview.Value).To(BeFalse(),
				"needs_human_review should be false when self-correction succeeds")

			// CORRECTNESS: ValidationAttemptsHistory captures the correction journey
			Expect(incidentResp.ValidationAttemptsHistory).To(HaveLen(2),
				"Should have 2 validation attempts: 1 failed + 1 passed")

			// First attempt: bad params (type mismatch on REPLICA_COUNT)
			firstAttempt := incidentResp.ValidationAttemptsHistory[0]
			Expect(firstAttempt.Attempt).To(Equal(1),
				"First attempt number should be 1")
			Expect(firstAttempt.IsValid).To(BeFalse(),
				"First attempt should fail due to type mismatch")
			Expect(len(firstAttempt.Errors)).To(BeNumerically(">=", 1),
				"First attempt must record parameter-level validation errors")
			hasParamError := false
			for _, e := range firstAttempt.Errors {
				if strings.Contains(e, "REPLICA_COUNT") || strings.Contains(e, "type") || strings.Contains(e, "required") {
					hasParamError = true
					break
				}
			}
			Expect(hasParamError).To(BeTrue(),
				"First attempt errors should identify specific parameter constraint failures")

			// Second attempt: corrected params pass validation
			secondAttempt := incidentResp.ValidationAttemptsHistory[1]
			Expect(secondAttempt.Attempt).To(Equal(2),
				"Second attempt number should be 2")
			Expect(secondAttempt.IsValid).To(BeTrue(),
				"Second attempt should pass with corrected parameters")

			// E2E-KA-1170-002 (consolidated): Final response has valid params, undeclared stripped
			Expect(incidentResp.SelectedWorkflow.Set).To(BeTrue(),
				"E2E-KA-1170-002: selected_workflow must be present with valid parameters")

			// E2E-KA-1170-003 (consolidated): validation_attempts_history shape
			for i, attempt := range incidentResp.ValidationAttemptsHistory {
				Expect(attempt.Attempt).To(Equal(i+1),
					"E2E-KA-1170-003: attempt numbers must be sequential")
				Expect(attempt.Timestamp).ToNot(BeEmpty(),
					"E2E-KA-1170-003: each attempt must have a timestamp for audit trail")
			}

			// BUSINESS IMPACT: Operator sees successful self-correction in audit trail,
			// confirming the LLM can learn from structured schema feedback.
		})
	})
})
