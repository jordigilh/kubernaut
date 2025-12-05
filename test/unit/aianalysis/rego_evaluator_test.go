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
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// BR-AI-011: Rego policy evaluation tests
var _ = Describe("RegoEvaluator", func() {
	var (
		evaluator *rego.Evaluator
		ctx       context.Context
	)

	// Helper to get testdata path relative to this test file
	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	// BR-AI-011: Policy evaluation
	Describe("Evaluate", func() {
		Context("with valid policy and input", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				})
			})

			It("should return approval decision", func() {
				input := &rego.PolicyInput{
					Environment:        "production",
					TargetInOwnerChain: true,
					Confidence:         0.85,
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
				Expect(result.ApprovalRequired).To(BeAssignableToTypeOf(true))
			})

		})

		// BR-AI-013: Approval scenarios using DescribeTable
		Context("determines approval requirement", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				})
			})

			DescribeTable("based on environment and data quality",
				func(env string, targetInChain bool, confidence float64, failedDetections []string, warnings []string, expectedApproval bool) {
					input := &rego.PolicyInput{
						Environment:        env,
						TargetInOwnerChain: targetInChain,
						Confidence:         confidence,
						FailedDetections:   failedDetections,
						Warnings:           warnings,
					}

					result, err := evaluator.Evaluate(ctx, input)

					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(Equal(expectedApproval))
				},
				// Production + data quality issues = approval required
				Entry("production + target not in chain",
					"production", false, 0.9, nil, nil, true),
				Entry("production + failed detections",
					"production", true, 0.9, []string{"gitOpsManaged"}, nil, true),
				Entry("production + warnings",
					"production", true, 0.9, nil, []string{"High memory pressure"}, true),
				Entry("production + low confidence",
					"production", true, 0.6, nil, nil, true),

				// Non-production = auto-approve (regardless of issues)
				Entry("development + any state",
					"development", false, 0.5, []string{"gitOpsManaged"}, nil, false),
				Entry("staging + any state",
					"staging", true, 0.8, nil, nil, false),

				// Production + clean state = auto-approve
				Entry("production + clean state + high confidence",
					"production", true, 0.9, nil, nil, false),
			)
		})

		// BR-AI-014: Graceful degradation - missing policy
		Context("when policy file is missing", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: "nonexistent/path/policy.rego",
				})
			})

			It("should default to manual approval (graceful degradation)", func() {
				result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
					Environment: "production",
				})

				// Should not error - graceful degradation
				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(BeTrue())
				Expect(result.Degraded).To(BeTrue())
			})
		})

		// BR-AI-014: Graceful degradation - syntax error
		Context("when policy has syntax error", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("invalid_policies/invalid.rego"),
				})
			})

			It("should default to manual approval (graceful degradation)", func() {
				result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
					Environment: "production",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(BeTrue())
				Expect(result.Degraded).To(BeTrue())
				Expect(result.Reason).To(ContainSubstring("Policy"))
			})
		})
	})
})

