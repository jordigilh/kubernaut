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

package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("BR-HAPI-191: WorkflowMeta.Parameters population (#1170)", func() {

	Context("UT-KA-1170-060: WorkflowMeta.Parameters populated from schema", func() {
		It("should store parameters in WorkflowMeta when set", func() {
			v := parser.NewValidator([]string{"test-workflow"})
			schema := []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Replicas"},
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Namespace"},
			}
			v.SetWorkflowMeta("test-workflow", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				Parameters:      schema,
			})

			meta, ok := v.GetWorkflowMeta("test-workflow")
			Expect(ok).To(BeTrue())
			Expect(meta.Parameters).To(HaveLen(2))
			Expect(meta.Parameters[0].Name).To(Equal("REPLICA_COUNT"))
			Expect(meta.Parameters[1].Name).To(Equal("NAMESPACE"))
		})
	})

	Context("UT-KA-1170-061: Missing Content leaves Parameters nil (fail-closed)", func() {
		It("should leave Parameters nil when no schema is set", func() {
			v := parser.NewValidator([]string{"test-workflow"})
			v.SetWorkflowMeta("test-workflow", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
			})

			meta, ok := v.GetWorkflowMeta("test-workflow")
			Expect(ok).To(BeTrue())
			Expect(meta.Parameters).To(BeNil())
		})
	})

	Context("UT-KA-1170-062: Empty schema leaves Parameters as empty slice", func() {
		It("should store empty Parameters when schema has no params", func() {
			v := parser.NewValidator([]string{"test-workflow"})
			v.SetWorkflowMeta("test-workflow", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				Parameters:      []models.WorkflowParameter{},
			})

			meta, ok := v.GetWorkflowMeta("test-workflow")
			Expect(ok).To(BeTrue())
			Expect(meta.Parameters).NotTo(BeNil())
			Expect(meta.Parameters).To(BeEmpty())
		})
	})
})

var _ = Describe("BR-HAPI-191: SelfCorrect with Parameter Validation (#1170)", func() {

	var (
		v *parser.Validator
	)

	BeforeEach(func() {
		v = parser.NewValidator([]string{"test-workflow"})
		min := float64(1)
		max := float64(10)
		v.SetWorkflowMeta("test-workflow", parser.WorkflowMeta{
			ExecutionEngine: "tekton",
			ExecutionBundle: "quay.io/test/bundle:v1",
			Parameters: []models.WorkflowParameter{
				{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min, Maximum: &max},
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "STRATEGY", Type: "string", Required: false, Description: "Strategy", Enum: []string{"rolling", "recreate"}},
			},
		})
	})

	Context("UT-KA-1170-053: SelfCorrect records multi-error in ValidationAttemptsHistory", func() {
		It("should record all parameter validation errors in history", func() {
			badResult := &katypes.InvestigationResult{
				WorkflowID: "test-workflow",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"REPLICA_COUNT": "not-a-number",
					"STRATEGY":      "canary",
				},
			}

			exhaustedResult, err := v.SelfCorrect(badResult, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					WorkflowID: "test-workflow",
					Confidence: 0.85,
					Parameters: map[string]interface{}{
						"REPLICA_COUNT": "still-not-a-number",
						"STRATEGY":      "still-invalid",
					},
				}, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(exhaustedResult.HumanReviewNeeded).To(BeTrue())
			Expect(exhaustedResult.ValidationAttemptsHistory).To(HaveLen(3))
			for _, attempt := range exhaustedResult.ValidationAttemptsHistory {
				Expect(attempt.IsValid).To(BeFalse())
				Expect(len(attempt.Errors)).To(BeNumerically(">=", 2),
					"Each attempt should record multiple parameter errors")
			}
		})
	})

	Context("UT-KA-1170-054: SelfCorrect passes ValidationResult to correctionFn", func() {
		It("should pass the ParameterValidationResult to correctionFn for template rendering", func() {
			badResult := &katypes.InvestigationResult{
				WorkflowID: "test-workflow",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"REPLICA_COUNT": "bad",
				},
			}

			var receivedErr error
			_, err := v.SelfCorrect(badResult, 2, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				receivedErr = validationErr
				return &katypes.InvestigationResult{
					WorkflowID: "test-workflow",
					Confidence: 0.85,
					Parameters: map[string]interface{}{
						"REPLICA_COUNT": float64(3),
						"NAMESPACE":     "default",
					},
				}, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(receivedErr).NotTo(BeNil())
			paramErr, ok := receivedErr.(*parser.ParameterValidationError)
			Expect(ok).To(BeTrue(), "correctionFn should receive *ParameterValidationError")
			Expect(paramErr.Result).NotTo(BeNil())
			Expect(paramErr.Result.SchemaHint).NotTo(BeEmpty())
		})
	})

	Context("UT-KA-1170-056: Existing allowlist/confidence checks still work", func() {
		It("should still reject invalid workflow_id before parameter validation", func() {
			badResult := &katypes.InvestigationResult{
				WorkflowID: "unknown-workflow",
				Confidence: 0.85,
				Parameters: map[string]interface{}{"REPLICA_COUNT": float64(3)},
			}

			exhaustedResult, err := v.SelfCorrect(badResult, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				return r, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(exhaustedResult.HumanReviewNeeded).To(BeTrue())
			Expect(exhaustedResult.ValidationAttemptsHistory[0].Errors[0]).To(ContainSubstring("workflow"))
		})

		It("should still reject confidence out of range", func() {
			badResult := &katypes.InvestigationResult{
				WorkflowID: "test-workflow",
				Confidence: 2.0,
				Parameters: map[string]interface{}{"REPLICA_COUNT": float64(3)},
			}

			exhaustedResult, err := v.SelfCorrect(badResult, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				return r, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(exhaustedResult.HumanReviewNeeded).To(BeTrue())
			Expect(exhaustedResult.ValidationAttemptsHistory[0].Errors[0]).To(ContainSubstring("confidence"))
		})
	})

	Context("UT-KA-1170-057: HumanReviewNeeded short-circuits validation", func() {
		It("should skip parameter validation when HumanReviewNeeded is true", func() {
			result := &katypes.InvestigationResult{
				WorkflowID:        "test-workflow",
				Confidence:        0.85,
				HumanReviewNeeded: true,
				Parameters:        map[string]interface{}{"REPLICA_COUNT": "bad"},
			}

			correctedResult, err := v.SelfCorrect(result, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				Fail("correctionFn should not be called when HumanReviewNeeded is true")
				return r, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(correctedResult.ValidationAttemptsHistory).To(HaveLen(1))
			Expect(correctedResult.ValidationAttemptsHistory[0].IsValid).To(BeTrue())
		})
	})
})
