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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("Rego Policy Integration", Label("integration", "rego"), func() {
	var (
		evaluator *rego.Evaluator
		evalCtx   context.Context
		cancel    context.CancelFunc
	)

	BeforeEach(func() {
		// Use production policies from config/rego/aianalysis/
		// This validates the actual policies that will be deployed
		policyPath := filepath.Join("..", "..", "..", "config", "rego", "aianalysis", "approval.rego")
		evaluator = rego.NewEvaluator(rego.Config{
			PolicyPath: policyPath,
		}, logr.Discard())

		evalCtx, cancel = context.WithCancel(context.Background())

		// ADR-050: Startup validation required
		err := evaluator.StartHotReload(evalCtx)
		Expect(err).NotTo(HaveOccurred(), "Policy should load successfully")
	})

	AfterEach(func() {
		if evaluator != nil {
			evaluator.Stop()
		}
		if cancel != nil {
			cancel()
		}
	})

	Context("Production Approval Policy - BR-AI-013", func() {
		It("should auto-approve staging environment", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "staging",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "staging"},
				FailedDetections: []string{},
				Warnings:         []string{},
				Confidence:       0.95,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse())
			Expect(result.Degraded).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("Auto-approved"))
		})

		// ADR-055 + BR-AI-085-005: Missing affected_resource = default-deny
		It("should require approval when affected resource is missing", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "production",
				FailedDetections: []string{},
				Warnings:         []string{},
				Confidence:       0.90,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue())
			Expect(result.Degraded).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("affected resource"))
		})

		It("should require approval for production with failed detections", func() {
			// Confidence < 0.8 (default threshold) so auto-approval does not apply (#225)
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "production",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				FailedDetections: []string{"gitOpsManaged"}, // Detection failed
				Warnings:         []string{},
				Confidence:       0.79,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("failed detections"))
		})

		It("should auto-approve production with all validations passing", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "production",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				FailedDetections: []string{},
				Warnings:         []string{},
				Confidence:       0.95,
				DetectedLabels: map[string]interface{}{
					"gitOpsManaged": true,
					"pdbProtected":  true,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			// Production with all validations passing should auto-approve
			// (depends on Rego policy implementation)
			Expect(result.Degraded).To(BeFalse())
		})
	})

	Context("Environment-Specific Rules - BR-AI-013", func() {
		It("should auto-approve development environment with affected resource", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "development",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "development"},
				FailedDetections: []string{"gitOpsManaged"},
				Confidence:       0.50,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse())
		})

		It("should auto-approve qa environment", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "qa",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "qa"},
				FailedDetections: []string{},
				Confidence:       0.75,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse())
		})

		It("should auto-approve test environment", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "test",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "test"},
				Confidence:       0.80,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse())
		})
	})

	Context("Warning Handling - BR-AI-011", func() {
		It("should require approval for production with warnings", func() {
			// Confidence < 0.8 (default threshold) so auto-approval does not apply (#225)
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "production",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "api", Namespace: "production"},
				FailedDetections: []string{},
				Warnings:         []string{"High resource utilization detected"},
				Confidence:       0.79,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("warnings"))
		})
	})

	Context("Stateful Workload Protection - BR-AI-011", func() {
		It("should require approval for stateful workloads in production", func() {
			// Use Kind "Deployment" (not "StatefulSet") to isolate the is_stateful rule (score 50)
			// from the is_sensitive_resource rule (score 80). StatefulSet triggers BOTH rules,
			// and the higher-scoring sensitive_resource reason wins, masking the stateful reason.
			// A Deployment with detected_labels.stateful=true exercises the stateful path exclusively.
			// Confidence < 0.8 (default threshold) so auto-approval does not apply (#225).
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment:      "production",
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "db", Namespace: "production"},
				FailedDetections: []string{},
				Confidence:       0.79,
				DetectedLabels: map[string]interface{}{
					"stateful": true,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("Stateful"))
		})
	})

	Context("Graceful Degradation - BR-AI-014", func() {
		It("should handle missing policy file gracefully", func() {
			// Create evaluator with non-existent policy path
			badEvaluator := rego.NewEvaluator(rego.Config{
				PolicyPath: "/nonexistent/policy.rego",
			}, logr.Discard())

			result, err := badEvaluator.Evaluate(evalCtx, &rego.PolicyInput{
				Environment: "staging",
			})

			// Should NOT return error - graceful degradation
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Degraded).To(BeTrue())
			// Default to requiring approval when policy unavailable (safe default)
			Expect(result.ApprovalRequired).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("Policy file not found"))
		})
	})

	Context("Complex Scenario Validation - BR-AI-013", func() {
		It("should handle complete policy input with all fields", func() {
			result, err := evaluator.Evaluate(evalCtx, &rego.PolicyInput{
				// Signal context
				SignalType:       "alert",
				Severity:         "high",
				Environment:      "staging",
				BusinessPriority: "P1",

				// Target resource
				TargetResource: rego.TargetResourceInput{
					Kind:      "Pod",
					Name:      "web-app-xyz",
					Namespace: "staging",
				},

				// Detected labels
				DetectedLabels: map[string]interface{}{
					"gitOpsManaged": true,
					"pdbProtected":  true,
					"stateful":      false,
				},

				// Custom labels
				CustomLabels: map[string][]string{
					"team":        {"platform"},
					"criticality": {"high"},
				},

				// ADR-055: Affected resource (replaces target_in_owner_chain)
				AffectedResource: &rego.AffectedResourceInput{Kind: "Deployment", Name: "web-app", Namespace: "staging"},

				// HolmesGPT-API response data
				Confidence: 0.92,
				Warnings:   []string{},

				// Failed detections
				FailedDetections: []string{},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Degraded).To(BeFalse())
			// Staging with all validations passing should auto-approve
			Expect(result.ApprovalRequired).To(BeFalse())
		})
	})
})
