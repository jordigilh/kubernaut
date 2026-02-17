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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// BR-AI-011: Rego policy evaluation tests
var _ = Describe("RegoEvaluator", func() {
	var (
		evaluator *rego.Evaluator
		ctx       context.Context
		cancel    context.CancelFunc
	)

	// Helper to get testdata path relative to this test file
	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		// Cancel context to stop goroutines
		if cancel != nil {
			cancel()
		}
		// Note: Not calling evaluator.Stop() as context cancellation handles cleanup
	})

	// BR-AI-011: Policy evaluation
	Describe("Evaluate", func() {
		Context("with valid policy and input", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				}, logr.Discard())

				// ADR-050: Startup validation required
				err := evaluator.StartHotReload(ctx)
				Expect(err).NotTo(HaveOccurred(), "Policy should load successfully")
			})

			// BR-AI-013: Business outcome - production with clean state should auto-approve
			It("should auto-approve production environment with clean state and high confidence", func() {
				input := &rego.PolicyInput{
					Environment: "production",
					AffectedResource: &rego.AffectedResourceInput{
						Kind: "Deployment", Name: "api", Namespace: "production",
					},
					Confidence: 0.85,
					DetectedLabels: map[string]interface{}{
						"gitOpsManaged": true,
						"pdbProtected":  true,
					},
					FailedDetections: []string{},
					Warnings:         []string{},
				}

				result, err := evaluator.Evaluate(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Production ALWAYS requires approval per BR-AI-013
				Expect(result.ApprovalRequired).To(BeTrue(), "Production environment always requires approval per BR-AI-013")
				Expect(result.Reason).To(ContainSubstring("Production environment"), "Should provide production approval reason")
				Expect(result.Degraded).To(BeFalse(), "Should not be in degraded mode")
			})

		})

		// BR-AI-013: Approval scenarios using DescribeTable
		Context("determines approval requirement", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				}, logr.Discard())

				// ADR-050: Startup validation required
				err := evaluator.StartHotReload(ctx)
				Expect(err).NotTo(HaveOccurred(), "Policy should load successfully")
			})

			DescribeTable("based on environment and data quality",
				func(env string, affectedResource *rego.AffectedResourceInput, confidence float64, failedDetections []string, warnings []string, expectedApproval bool) {
					input := &rego.PolicyInput{
						Environment:      env,
						AffectedResource: affectedResource,
						Confidence:       confidence,
						FailedDetections: failedDetections,
						Warnings:         warnings,
					}

					result, err := evaluator.Evaluate(ctx, input)

					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(Equal(expectedApproval))
				},
				// BR-AI-085-005: Missing affected resource = approval required (default-deny)
				Entry("production + missing affected resource",
					"production", (*rego.AffectedResourceInput)(nil), 0.9, nil, nil, true),
				Entry("production + failed detections",
					"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, []string{"gitOpsManaged"}, nil, true),
				Entry("production + warnings",
					"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, nil, []string{"High memory pressure"}, true),
				Entry("production + low confidence",
					"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.6, nil, nil, true),

				// Non-production = auto-approve (regardless of issues)
				Entry("development + missing affected resource",
					"development", (*rego.AffectedResourceInput)(nil), 0.5, []string{"gitOpsManaged"}, nil, true),
				Entry("staging + any state",
					"staging", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "staging"}, 0.8, nil, nil, false),

				// Production ALWAYS requires approval per BR-AI-013
				Entry("production + clean state + high confidence",
					"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, nil, nil, true),
			)

			// BR-AI-013: Recovery scenario tests (per IMPLEMENTATION_PLAN_V1.0.md)
			DescribeTable("based on recovery context",
				func(isRecovery bool, recoveryAttemptNumber int, severity string, env string, expectedApproval bool) {
					input := &rego.PolicyInput{
						Environment: env,
						AffectedResource: &rego.AffectedResourceInput{
							Kind: "Deployment", Name: "api", Namespace: env,
						},
						Confidence:            0.9, // High confidence
						IsRecoveryAttempt:     isRecovery,
						RecoveryAttemptNumber: recoveryAttemptNumber,
						Severity:              severity,
					}

					result, err := evaluator.Evaluate(ctx, input)

					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(Equal(expectedApproval))
				},
				// Multiple recovery attempts (3+) = approval required (any environment)
				Entry("development + 3rd recovery = approval required",
					true, 3, "warning", "development", true),
				Entry("staging + 4th recovery = approval required",
					true, 4, "warning", "staging", true),

				// Production ALWAYS requires approval (BR-AI-013)
				Entry("production + 1st recovery = require approval",
					true, 1, "warning", "production", true),
				Entry("production + not recovery = require approval",
					false, 0, "warning", "production", true),

				// Non-production with <3 recovery attempts = auto-approve
				Entry("development + 1st recovery = auto-approve",
					true, 1, "warning", "development", false),
				Entry("staging + 2nd recovery = auto-approve",
					true, 2, "warning", "staging", false),
				Entry("development + not recovery = auto-approve",
					false, 0, "warning", "development", false),

				// High severity + recovery = approval required (any environment)
				Entry("development + high severity + recovery = approval",
					true, 1, "critical", "development", true),
				Entry("staging + P0 + recovery = approval",
					true, 1, "P0", "staging", true),
			)

			// BR-AI-013: Signal context in policy input
			Context("handles signal context fields", func() {
				It("should pass all signal context fields to policy", func() {
					input := &rego.PolicyInput{
						SignalType:       "OOMKilled",
						Severity:         "critical",
						Environment:      "development",
						BusinessPriority: "P0",
						TargetResource: rego.TargetResourceInput{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
						AffectedResource: &rego.AffectedResourceInput{
							Kind: "Deployment", Name: "api", Namespace: "default",
						},
						Confidence:            0.9,
						IsRecoveryAttempt:     true,
						RecoveryAttemptNumber: 1,
					}

					result, err := evaluator.Evaluate(ctx, input)

					// Critical severity + recovery = approval required
					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(BeTrue())
				})
			})
		})

		// BR-AI-014: Graceful degradation - missing policy
		Context("when policy file is missing", func() {
			BeforeEach(func() {
				evaluator = rego.NewEvaluator(rego.Config{
					PolicyPath: "nonexistent/path/policy.rego",
				}, logr.Discard())
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
				}, logr.Discard())
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
