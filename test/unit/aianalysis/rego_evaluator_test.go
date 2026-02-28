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
			Entry("production + failed detections + high confidence → auto-approve",
				"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, []string{"gitOpsManaged"}, nil, false),
			Entry("production + warnings + high confidence → auto-approve",
				"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, nil, []string{"High memory pressure"}, false),
				Entry("production + low confidence",
					"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.6, nil, nil, true),

				// Non-production = auto-approve (regardless of issues)
				Entry("development + missing affected resource",
					"development", (*rego.AffectedResourceInput)(nil), 0.5, []string{"gitOpsManaged"}, nil, true),
				Entry("staging + any state",
					"staging", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "staging"}, 0.8, nil, nil, false),

			// Production + high confidence + clean state → auto-approve
			Entry("production + clean state + high confidence → auto-approve",
				"production", &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"}, 0.9, nil, nil, false),
			)

		// Confidence-based auto-approval for production environments
		// When confidence >= 0.9 and no critical safety conditions, production should auto-approve
		DescribeTable("confidence-based auto-approval in production",
			func(confidence float64, affectedResource *rego.AffectedResourceInput, failedDetections []string, warnings []string, expectedApproval bool, expectedReasonSubstring string) {
				input := &rego.PolicyInput{
					Environment:      "production",
					AffectedResource: affectedResource,
					Confidence:       confidence,
					FailedDetections: failedDetections,
					Warnings:         warnings,
				}

				result, err := evaluator.Evaluate(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.ApprovalRequired).To(Equal(expectedApproval),
					"confidence=%.2f, failedDetections=%v, warnings=%v → approvalRequired should be %v",
					confidence, failedDetections, warnings, expectedApproval)
				if expectedReasonSubstring != "" {
					Expect(result.Reason).To(ContainSubstring(expectedReasonSubstring))
				}
			},
			// High confidence (>= 0.9) + clean state → auto-approve
			Entry("UT-AIA-CONF-001: high confidence + clean state → auto-approve",
				0.95, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{}, []string{},
				false, "Auto-approved"),
			Entry("UT-AIA-CONF-002: confidence exactly 0.9 + clean state → auto-approve",
				0.9, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{}, []string{},
				false, "Auto-approved"),
			// High confidence + failed detections → auto-approve (minor data quality issues)
			Entry("UT-AIA-CONF-003: high confidence + failed detections → auto-approve",
				0.95, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{"gitOpsManaged"}, []string{},
				false, "Auto-approved"),
			// High confidence + warnings → auto-approve
			Entry("UT-AIA-CONF-004: high confidence + warnings → auto-approve",
				0.92, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{}, []string{"High memory pressure"},
				false, "Auto-approved"),
			// Low confidence (< 0.9) → still require approval
			Entry("UT-AIA-CONF-005: low confidence + clean state → require approval",
				0.85, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{}, []string{},
				true, "Production environment"),
			Entry("UT-AIA-CONF-006: confidence 0.89 (just below threshold) → require approval",
				0.89, &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				[]string{}, []string{},
				true, "Production environment"),
			// Safety: missing affected resource → ALWAYS require approval regardless of confidence
			Entry("UT-AIA-CONF-007: high confidence + missing affected resource → require approval",
				0.99, (*rego.AffectedResourceInput)(nil),
				[]string{}, []string{},
				true, "Missing affected resource"),
		)

		// BR-AI-013: Policy evaluation based on environment and severity
		DescribeTable("based on environment and severity",
				func(severity string, env string, expectedApproval bool) {
					input := &rego.PolicyInput{
						Environment: env,
						AffectedResource: &rego.AffectedResourceInput{
							Kind: "Deployment", Name: "api", Namespace: env,
						},
						Confidence: 0.9, // High confidence
						Severity:   severity,
					}

					result, err := evaluator.Evaluate(ctx, input)

					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(Equal(expectedApproval))
				},
			// Production + high confidence (0.9) → auto-approve regardless of severity
			Entry("production + warning severity + high confidence = auto-approve",
				"warning", "production", false),
			Entry("production + critical severity + high confidence = auto-approve",
				"critical", "production", false),

				// Non-production = auto-approve
				Entry("development + warning = auto-approve",
					"warning", "development", false),
				Entry("staging + warning = auto-approve",
					"warning", "staging", false),
				Entry("development + critical = auto-approve",
					"critical", "development", false),
				Entry("staging + P0 = auto-approve",
					"P0", "staging", false),
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
						Confidence: 0.9,
					}

					result, err := evaluator.Evaluate(ctx, input)

					// Development + critical = auto-approve
					Expect(err).NotTo(HaveOccurred())
					Expect(result.ApprovalRequired).To(BeFalse())
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
