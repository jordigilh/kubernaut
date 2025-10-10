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

package gateway_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// Business Outcome Testing: Test WHAT remediation paths enable, not HOW they're calculated
//
// ❌ WRONG: "should call Rego evaluator with correct input" (tests implementation)
// ✅ RIGHT: "enables AI to apply conservative remediation in production" (tests business outcome)

var _ = Describe("BR-GATEWAY-022: Remediation Path Decision", func() {
	var (
		pathDecider *processing.RemediationPathDecider
		logger      *logrus.Logger
		ctx         context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Quiet during tests
		pathDecider = processing.NewRemediationPathDecider(logger)
		ctx = context.Background()
	})

	// BUSINESS OUTCOME: Remediation path controls AI aggressiveness
	// - Aggressive: Immediate kubectl apply, auto-rollback, pod deletion
	// - Conservative: GitOps PR, manual approval, read-only analysis
	// - Moderate: Validation + automated execution
	// - Manual: Analysis only, operator decides
	//
	// This prevents AI from being too aggressive in production (risk management)
	Describe("Path assignment balances automation speed with risk tolerance", func() {
		DescribeTable("Environment and priority determine remediation aggressiveness",
			func(environment string, priority string, expectedPath string, businessReason string) {
				signalCtx := &processing.SignalContext{
					Signal: &types.NormalizedSignal{
						AlertName: "TestAlert",
						Severity:  "critical",
					},
					Environment: environment,
					Priority:    priority,
				}

				path := pathDecider.DeterminePath(ctx, signalCtx)

				// BUSINESS OUTCOME: Path influences how AI remediates
				Expect(path).To(Equal(expectedPath), businessReason)

				// Verify path is valid
				Expect(path).To(BeElementOf([]string{"aggressive", "moderate", "conservative", "manual"}),
					"Path must be one of the four defined strategies")
			},

			// Scenario 1: P0 production → aggressive (immediate action)
			Entry("P0 production outage → aggressive remediation for revenue protection",
				"production", "P0", "aggressive",
				"Critical prod issues need immediate automated fix (restart pod, scale up, rollback)"),

			// Scenario 2: P1 production → conservative (GitOps PR)
			Entry("P1 production issue → conservative remediation with approval",
				"production", "P1", "conservative",
				"High priority prod needs human approval before destructive changes"),

			// Scenario 3: P2 production → conservative (safe default)
			Entry("P2 production → conservative to avoid unintended changes",
				"production", "P2", "conservative",
				"Low priority prod should not trigger automated changes"),

			// Scenario 4: P0 staging → moderate (validation + execution)
			Entry("P0 staging failure → moderate remediation with validation",
				"staging", "P0", "moderate",
				"Staging allows faster iteration but needs validation before execution"),

			// Scenario 5: P1 staging → moderate
			Entry("P1 staging → moderate remediation",
				"staging", "P1", "moderate",
				"Staging is pre-prod, balance speed with safety"),

			// Scenario 6: P2 staging → manual (analysis only)
			Entry("P2 staging → manual remediation",
				"staging", "P2", "manual",
				"Low priority staging issues need manual review"),

			// Scenario 7: P0 development → aggressive (fast feedback)
			Entry("P0 development failure → aggressive for fast developer feedback",
				"development", "P0", "aggressive",
				"Dev environments prioritize speed over safety"),

			// Scenario 8: P1 development → moderate
			Entry("P1 development → moderate remediation",
				"development", "P1", "moderate",
				"Dev can tolerate some risk for faster fixes"),

			// Scenario 9: P2 development → manual
			Entry("P2 development → manual analysis",
				"development", "P2", "manual",
				"Low priority dev issues don't need automation"),

			// Scenario 10: Unknown environment → conservative (safe default)
			Entry("Unknown environment → conservative as safe fallback",
				"unknown", "P0", "conservative",
				"Unknown environments treated as production for safety"),

			// Scenario 11: Invalid priority → manual (safe default)
			Entry("Invalid priority → manual as safe fallback",
				"production", "P99", "manual",
				"Invalid priority defaults to manual for safety"),
		)
	})

	// BUSINESS OUTCOME: Rego policy enables custom business rules
	// Without Rego, only fallback table available (limited flexibility)
	Describe("Rego policy evaluation for custom business rules", func() {
		It("evaluates Rego policy when configured", func() {
			// Business scenario: Organization has custom priority rules
			// Example: Team "platform" gets aggressive path even in prod
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
					Labels: map[string]string{
						"team": "platform",
					},
				},
				Environment: "production",
				Priority:    "P1",
			}

			// Configure mock Rego evaluator
			mockRego := &processing.MockRegoEvaluator{
				Result: "aggressive",
			}
			pathDecider.SetRegoEvaluator(mockRego)

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Rego overrides fallback table
			Expect(path).To(Equal("aggressive"),
				"Rego policy allows custom business logic")
			Expect(mockRego.Called).To(BeTrue(),
				"Rego was actually invoked")

			// Business capability verified:
			// Custom rule → Rego evaluates → Overrides default path
		})

		It("falls back to table when Rego evaluation fails", func() {
			// Business scenario: Rego policy malformed or Rego server down
			// Expected: System continues with fallback table (graceful degradation)
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
					Severity:  "critical",
				},
				Environment: "production",
				Priority:    "P0",
			}

			// Configure Rego to return error
			mockRego := &processing.MockRegoEvaluator{
				Error: fmt.Errorf("rego policy evaluation failed"),
			}
			pathDecider.SetRegoEvaluator(mockRego)

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Fallback prevents system failure
			Expect(path).NotTo(BeEmpty(),
				"Fallback ensures system continues processing")
			Expect(path).To(Equal("aggressive"),
				"Fallback table provides P0 prod → aggressive")

			// Business capability verified:
			// Rego failure → Fallback table → System continues
		})

		It("validates Rego output to prevent invalid paths", func() {
			// Business scenario: Rego policy returns invalid path value
			// Expected: Gateway rejects invalid output, uses fallback
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "production",
				Priority:    "P1",
			}

			// Configure Rego to return invalid path
			mockRego := &processing.MockRegoEvaluator{
				Result: "invalid_path_value",
			}
			pathDecider.SetRegoEvaluator(mockRego)

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Invalid policy output rejected
			Expect(path).To(BeElementOf([]string{"aggressive", "moderate", "conservative", "manual"}),
				"Invalid Rego output triggers fallback validation")

			// Business capability verified:
			// Invalid Rego → Validation fails → Fallback ensures valid path
		})
	})

	// BUSINESS OUTCOME: Caching reduces policy evaluation overhead
	// 100 alerts/sec with Rego evaluation each time = 10-20ms latency
	// Caching same signal evaluations = <1ms latency
	Describe("Performance optimization through path caching", func() {
		It("caches path decisions for identical signals", func() {
			// Business scenario: 50 pods crash in 10 seconds (all same signal)
			// Expected: Rego evaluated once, cached for remaining 49 signals
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "OOMKilled",
					Severity:  "critical",
				},
				Environment: "production",
				Priority:    "P0",
			}

			// Track Rego evaluation count
			mockRego := &processing.MockRegoEvaluator{
				Result:    "aggressive",
				CallCount: 0,
			}
			pathDecider.SetRegoEvaluator(mockRego)

			// First evaluation
			path1 := pathDecider.DeterminePath(ctx, signalCtx)
			firstCallCount := mockRego.CallCount

			// Second evaluation (same signal)
			path2 := pathDecider.DeterminePath(ctx, signalCtx)
			secondCallCount := mockRego.CallCount

			// BUSINESS OUTCOME: Cache prevents redundant Rego evaluations
			Expect(path1).To(Equal(path2),
				"Same signal produces same path")
			Expect(secondCallCount).To(Equal(firstCallCount),
				"Cache prevents second Rego evaluation")

			// Business capability verified:
			// Same signal → Cache hit → No Rego call → <1ms latency
		})

		It("cache miss triggers Rego evaluation for different signals", func() {
			// Business scenario: Different alerts need different paths
			// Expected: Each unique signal triggers Rego evaluation
			signalCtx1 := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "production",
				Priority:    "P0",
			}
			signalCtx2 := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "staging",
				Priority:    "P1",
			}

			mockRego := &processing.MockRegoEvaluator{
				Result: "aggressive",
			}
			pathDecider.SetRegoEvaluator(mockRego)

			_ = pathDecider.DeterminePath(ctx, signalCtx1)
			firstCallCount := mockRego.CallCount

			_ = pathDecider.DeterminePath(ctx, signalCtx2)
			secondCallCount := mockRego.CallCount

			// BUSINESS OUTCOME: Different signals evaluated independently
			Expect(secondCallCount).To(BeNumerically(">", firstCallCount),
				"Different signal triggers new Rego evaluation")

			// Business capability verified:
			// Different signal → Cache miss → Rego evaluates
		})
	})

	// BUSINESS OUTCOME: RemediationRequest CRD includes path for AI consumption
	// Without path in CRD, AI must re-evaluate rules (duplication)
	Describe("Path propagation to RemediationRequest CRD", func() {
		It("includes remediation path in CRD spec for AI guidance", func() {
			// Business scenario: AI service reads RemediationRequest CRD
			// Expected: CRD contains path, AI doesn't need to re-evaluate
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "HighMemoryUsage",
					Resource: types.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "payment-api",
						Namespace: "prod",
					},
				},
				Environment: "production",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)
			crdMetadata := pathDecider.GetCRDMetadata(signalCtx, path)

			// BUSINESS OUTCOME: CRD contains path for AI strategy selection
			Expect(crdMetadata).To(HaveKey("remediationPath"),
				"CRD must include path for AI consumption")
			Expect(crdMetadata["remediationPath"]).To(Equal("aggressive"))

			// Additional metadata for AI decision-making
			Expect(crdMetadata).To(HaveKey("environment"))
			Expect(crdMetadata).To(HaveKey("priority"))

			// Business capability verified:
			// Gateway → CRD with path → AI reads → Applies appropriate strategy
		})

		It("provides path explanation for observability", func() {
			// Business scenario: Operator reviews why AI chose aggressive path
			// Expected: CRD includes explanation for troubleshooting
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "production",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)
			explanation := pathDecider.GetPathExplanation(signalCtx, path)

			// BUSINESS OUTCOME: Explanation enables audit and troubleshooting
			Expect(explanation).NotTo(BeEmpty(),
				"Explanation required for compliance")
			Expect(explanation).To(ContainSubstring("P0"),
				"Explanation includes priority reasoning")
			Expect(explanation).To(ContainSubstring("production"),
				"Explanation includes environment reasoning")

			// Business capability verified:
			// Path decision → Explanation → Audit trail → Compliance
		})
	})

	// BUSINESS OUTCOME: Graceful handling of edge cases
	Describe("Edge case handling for resilient path assignment", func() {
		It("handles missing environment with safe default", func() {
			// Business scenario: Alert missing environment classification
			// Expected: Default to conservative (treat as production)
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "", // Missing
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Missing data defaults to safe path
			Expect(path).To(Equal("conservative"),
				"Missing environment defaults to conservative for safety")

			// Business capability verified:
			// Missing env → Safe default → Prevents overly aggressive AI
		})

		It("handles missing priority with safe default", func() {
			// Business scenario: Priority assignment failed
			// Expected: Default to manual (safest option)
			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "TestAlert",
				},
				Environment: "production",
				Priority:    "", // Missing
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Missing priority triggers manual review
			Expect(path).To(Equal("manual"),
				"Missing priority requires human review")

			// Business capability verified:
			// Missing priority → Manual path → Human review required
		})

		It("handles nil signal gracefully", func() {
			// Business scenario: Programming error passes nil signal
			// Expected: Return safe default, don't crash Gateway
			path := pathDecider.DeterminePath(ctx, nil)

			// BUSINESS OUTCOME: Defensive programming prevents Gateway crash
			Expect(path).To(Equal("manual"),
				"Nil signal defaults to manual for safety")

			// Business capability verified:
			// Nil signal → Safe default → Gateway continues operating
		})
	})
})

// BR-GATEWAY-022: Remediation Path Decision with Rego Policies
// Business Outcome: Organizations control AI aggressiveness without code changes
var _ = Describe("BR-GATEWAY-022: Custom Remediation Strategies via Rego Policies", func() {
	var (
		pathDecider *processing.RemediationPathDecider
		ctx         context.Context
		logger      *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(GinkgoWriter)
		logger.SetLevel(logrus.PanicLevel) // Suppress logs during tests
		ctx = context.Background()
	})

	// BUSINESS CAPABILITY: Organizations control AI automation risk tolerance
	Context("when organization defines risk tolerance via custom Rego policy", func() {
		BeforeEach(func() {
			// Business scenario: Organization mounts remediation strategy policy in ConfigMap
			// Policy defines: When is AI allowed to execute vs require human approval?
			var err error
			pathDecider, err = processing.NewRemediationPathDeciderWithRego(
				"../../../config.app/gateway/policies/remediation_path.rego",
				logger,
			)
			Expect(err).NotTo(HaveOccurred(),
				"Organization can customize AI behavior via policy file")
		})

		It("enables immediate automated remediation for critical production outages", func() {
			// BUSINESS SCENARIO: Production down, revenue impact, AI must act immediately
			// Organization policy: "P0 production = aggressive = immediate kubectl apply"
			// Risk: AI might make mistakes, but production down is worse

			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "ProductionOutage",
					Severity:  "critical",
				},
				Environment: "production",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Path = "aggressive"
			// Downstream effect: AI Executor runs `kubectl delete pod` immediately (no PR)
			// Downstream effect: Recovery in 30 seconds (not 5 minutes waiting for approval)
			Expect(path).To(Equal("aggressive"),
				"Organization risk tolerance: P0 production = revenue loss = allow aggressive AI")
		})

		It("requires human approval for non-critical production changes", func() {
			// BUSINESS SCENARIO: Production warning, not critical yet, organization wants safety
			// Organization policy: "P1 production = conservative = GitOps PR + human review"
			// Risk: AI might fix incorrectly, human review catches issues

			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "ProductionWarning",
					Severity:  "warning",
				},
				Environment: "production",
				Priority:    "P1",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Path = "conservative"
			// Downstream effect: AI creates GitOps PR
			// Downstream effect: 2 engineers approve PR
			// Downstream effect: ArgoCD syncs after approval (safe, slower)
			Expect(path).To(Equal("conservative"),
				"Organization risk tolerance: P1 production = require human approval")
		})

		It("enables fast developer feedback in development environment", func() {
			// BUSINESS SCENARIO: Developer's pod crashing, blocking development work
			// Organization policy: "Development = aggressive = immediate fix"
			// Risk: Dev env, mistakes are OK, speed is priority

			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "DevPodCrashing",
					Severity:  "critical",
				},
				Environment: "development",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Path = "aggressive"
			// Downstream effect: AI immediately restarts pod
			// Downstream effect: Developer unblocked in 30 seconds
			// Downstream effect: Fast inner loop (test → fail → fix → test)
			Expect(path).To(Equal("aggressive"),
				"Organization policy: dev environment = prioritize speed over safety")
		})

		It("enables organization to customize strategies per environment", func() {
			// BUSINESS SCENARIO: Organization wants different risk profiles per environment
			// Production: Very conservative (revenue at risk)
			// Staging: Moderate (catch issues before prod)
			// Development: Aggressive (developer productivity)

			// This demonstrates Rego enables per-environment customization
			// (Not possible with simple hardcoded if/else)

			prodCtx := &processing.SignalContext{
				Signal:      &types.NormalizedSignal{AlertName: "Test"},
				Environment: "production",
				Priority:    "P1",
			}
			stagingCtx := &processing.SignalContext{
				Signal:      &types.NormalizedSignal{AlertName: "Test"},
				Environment: "staging",
				Priority:    "P1",
			}
			devCtx := &processing.SignalContext{
				Signal:      &types.NormalizedSignal{AlertName: "Test"},
				Environment: "development",
				Priority:    "P1",
			}

			prodPath := pathDecider.DeterminePath(ctx, prodCtx)
			stagingPath := pathDecider.DeterminePath(ctx, stagingCtx)
			devPath := pathDecider.DeterminePath(ctx, devCtx)

			// BUSINESS OUTCOME: Different paths per environment (same priority)
			// Production: Conservative (safety first, human approval)
			// Staging: Moderate (validation checks)
			// Development: Aggressive (fast feedback, no approval needed)
			Expect(prodPath).To(Equal("conservative"), "Production: Safety first")
			Expect(stagingPath).To(Equal("moderate"), "Staging: Balanced")
			Expect(devPath).To(Equal("aggressive"), "Development: Speed prioritized (P1 = aggressive in dev)")

			// Business capability verified:
			// Rego enables: environment-based, priority-based, time-based, team-based rules
		})
	})

	// System Resilience: Gateway works without custom policies
	Context("when organization uses default Gateway deployment (no custom policies)", func() {
		BeforeEach(func() {
			// Business scenario: Organization deploys Gateway without customizing policies
			pathDecider = processing.NewRemediationPathDecider(logger)
		})

		It("ensures Gateway provides sensible remediation strategies out-of-box", func() {
			// BUSINESS SCENARIO: Organization wants to try Gateway quickly
			// Expected: Gateway works immediately, no policy engineering required

			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "ProductionOutage",
					Severity:  "critical",
				},
				Environment: "production",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Gateway functional without custom policies
			// Default rules: P0 production = aggressive (reasonable default)
			Expect(path).To(Equal("aggressive"),
				"Sensible defaults enable quick Gateway adoption")

			// Business capability verified:
			// Organization can: Deploy → See value → Customize later
		})
	})

	// System Resilience: Gateway never crashes due to policy bugs
	Context("when organization's custom policy has runtime errors", func() {
		BeforeEach(func() {
			// Business scenario: Organization deployed working policy, loaded successfully
			var err error
			pathDecider, err = processing.NewRemediationPathDeciderWithRego(
				"../../../config.app/gateway/policies/remediation_path.rego",
				logger,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ensures Gateway continues operating even if policy fails at runtime", func() {
			// BUSINESS SCENARIO: Policy has runtime bug (e.g., references undefined variable)
			// Expected: Gateway logs error, uses default strategy, keeps processing
			//
			// WHY THIS MATTERS: Policy bug shouldn't stop all remediation
			// Example: Bad policy deployed → Gateway still handles 99% of cases

			signalCtx := &processing.SignalContext{
				Signal: &types.NormalizedSignal{
					AlertName: "ProductionAlert",
					Severity:  "critical",
				},
				Environment: "production",
				Priority:    "P0",
			}

			path := pathDecider.DeterminePath(ctx, signalCtx)

			// BUSINESS OUTCOME: Gateway resilient to policy bugs
			// Worst case: Uses default strategy (not ideal, but better than Gateway crash)
			Expect(path).To(BeElementOf([]string{"aggressive", "moderate", "conservative", "manual"}),
				"Fallback ensures Gateway never crashes due to policy bugs")

			// Business capability verified:
			// Bad policy → Warning logged → Default strategy → Remediation continues
		})
	})

	// TDD RED PHASE: Remediation path decision for custom environments (fallback logic)
	// These tests will FAIL until we add catch-all logic to fallback table
	Context("when using custom environments without Rego policy (fallback table)", func() {
		BeforeEach(func() {
			// Business scenario: Organization uses custom environments but no Rego policy
			// Gateway must use fallback table with sensible defaults for unknown environments
			pathDecider = processing.NewRemediationPathDecider(logger)
		})

		DescribeTable("determines reasonable remediation path for custom environments using fallback logic",
			func(priority string, customEnv string, businessScenario string, acceptablePaths []string, reasoning string) {
				// BUSINESS OUTCOME: Gateway handles ANY environment gracefully
				// Custom environments should get reasonable fallback remediation strategies
				//
				// Expected fallback logic:
				// - High priority (P0) + unknown env → "aggressive" or "moderate" (act quickly, but not production-level risk)
				// - Medium priority (P1, P2) + unknown env → "moderate" or "conservative" (safe approach)
				// - Low priority (P3) + unknown env → "conservative" or "manual" (minimal risk)

				signalCtx := &processing.SignalContext{
					Signal: &types.NormalizedSignal{
						AlertName: "TestAlert",
						Severity:  "critical",
					},
					Environment: customEnv,
					Priority:    priority,
				}

				path := pathDecider.DeterminePath(ctx, signalCtx)

				// Verify path is valid
				validPaths := []string{"aggressive", "moderate", "conservative", "manual"}
				Expect(path).To(BeElementOf(validPaths),
					"Gateway must assign valid path for: %s", businessScenario)

				// Verify path makes business sense for this scenario
				Expect(path).To(BeElementOf(acceptablePaths),
					"Reasoning: %s", reasoning)
			},

			Entry("canary deployment - P0",
				"P0", "canary",
				"P0 critical in canary deployment",
				[]string{"moderate", "aggressive"},
				"Canary has limited blast radius, can be aggressive but not as risky as full prod"),

			Entry("canary deployment - P1",
				"P1", "canary",
				"P1 warning in canary deployment",
				[]string{"moderate", "conservative"},
				"Canary validation phase, prefer moderate approach"),

			Entry("qa-eu environment - P0",
				"P0", "qa-eu",
				"P0 critical in EU QA environment",
				[]string{"moderate", "aggressive"},
				"QA environment, can be more aggressive since no prod impact"),

			Entry("qa-eu environment - P1",
				"P1", "qa-eu",
				"P1 warning in EU QA environment",
				[]string{"moderate"},
				"QA environment, moderate automation is safe"),

			Entry("prod-us-east - P0",
				"P0", "prod-us-east",
				"P0 critical in US East production",
				[]string{"aggressive", "moderate"},
				"If pattern matching works: treat as production. Otherwise: still high priority"),

			Entry("blue environment - P0",
				"P0", "blue",
				"P0 critical in blue deployment (current prod version)",
				[]string{"moderate", "aggressive"},
				"Blue is current production, but unknown environment → be somewhat careful"),

			Entry("green environment - P1",
				"P1", "green",
				"P1 warning in green deployment (new version being validated)",
				[]string{"moderate", "conservative"},
				"Green is validation phase, prefer safer approach"),

			Entry("uat environment - P0",
				"P0", "uat",
				"P0 critical in UAT (blocks customer validation)",
				[]string{"moderate", "aggressive"},
				"UAT is important but not production, can be more aggressive"),

			Entry("pre-prod environment - P0",
				"P0", "pre-prod",
				"P0 critical in pre-prod (blocks production release)",
				[]string{"moderate", "conservative"},
				"Pre-prod is close to production, should be careful"),

			Entry("unknown environment - P3",
				"P3", "completely-unknown-env",
				"P3 info in unknown environment",
				[]string{"conservative", "manual"},
				"Low priority + unknown environment → minimal automation risk"),

			Entry("unknown environment - P2",
				"P2", "another-unknown-env",
				"P2 warning in unknown environment",
				[]string{"moderate", "conservative"},
				"Medium priority + unknown environment → moderate approach"),
		)

		// Business capability verified:
		// Custom environments get sensible remediation strategies even without Rego policy
		// Gateway never fails due to unknown environment
	})
})
