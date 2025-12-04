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

package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("RegoEvaluator", func() {
	var (
		ctx       context.Context
		evaluator *rego.Evaluator
		logger    logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
	})

	// BR-AI-011: Policy evaluation
	Describe("Evaluate", func() {
		Context("with valid policy and input", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyDir: "testdata/policies",
				}, logger)
			})

			It("should return approval decision - BR-AI-011", func() {
				input := &rego.PolicyInput{
					Environment:        "development",
					TargetInOwnerChain: true,
					DetectedLabels: map[string]interface{}{
						"gitOpsManaged": true,
						"pdbProtected":  true,
					},
					FailedDetections: []string{},
					Warnings:         []string{},
				}

				result, err := evaluator.Evaluate(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				// Development environment should auto-approve
				Expect(result.ApprovalRequired).To(BeFalse())
				Expect(result.Degraded).To(BeFalse())
			})
		})

		// BR-AI-013: Approval scenarios
		DescribeTable("determines approval requirement",
			func(env string, targetInChain bool, failedDetections []string, warnings []string, expectedApproval bool) {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyDir: "testdata/policies",
				}, logger)

				input := &rego.PolicyInput{
					Environment:        env,
					TargetInOwnerChain: targetInChain,
					FailedDetections:   failedDetections,
					Warnings:           warnings,
				}

				result, err := evaluator.Evaluate(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(Equal(expectedApproval))
			},
			// Production + data quality issues = approval required
			Entry("production + target not in chain", "production", false, nil, nil, true),
			Entry("production + failed detections", "production", true, []string{"gitOpsManaged"}, nil, true),
			Entry("production + warnings", "production", true, nil, []string{"some warning"}, true),

			// Non-production = auto-approve
			Entry("development + any state", "development", false, []string{"gitOpsManaged"}, nil, false),
			Entry("staging + any state", "staging", true, nil, nil, false),

			// Production + good data = auto-approve
			Entry("production + clean state", "production", true, nil, nil, false),
		)

		// BR-AI-014: Graceful degradation
		Context("when policy directory does not exist", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyDir: "nonexistent/path",
				}, logger)
			})

			It("should default to manual approval - BR-AI-014", func() {
				result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{})

				// Should not error - graceful degradation
				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(BeTrue())
				Expect(result.Degraded).To(BeTrue())
			})
		})

		// BR-AI-014: Syntax error handling
		Context("when policy has syntax error", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyDir: "testdata/invalid_policies",
				}, logger)
			})

			It("should default to manual approval - BR-AI-014", func() {
				result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(BeTrue())
				Expect(result.Degraded).To(BeTrue())
				Expect(result.Reason).To(ContainSubstring("failed"))
			})
		})
	})
})

