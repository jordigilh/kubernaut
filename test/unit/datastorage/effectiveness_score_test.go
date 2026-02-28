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

package datastorage

import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// EFFECTIVENESS SCORE COMPUTATION TESTS
// Business Requirements: BR-EM-001 to BR-EM-004
// Architecture: DD-017 v2.1 Scoring Formula
// ========================================
var _ = Describe("Effectiveness Score Computation (DD-017 v2.1)", func() {

	// ========================================
	// ComputeWeightedScore - Pure Scoring Logic
	// ========================================
	Describe("ComputeWeightedScore", func() {

		// UT-DS-EFF-001: All three components assessed
		It("UT-DS-EFF-001: should compute correct weighted score with all three components", func() {
			healthScore := 1.0 // 100% health pass rate
			alertScore := 1.0  // all alerts resolved
			metricsScore := 0.8

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     &healthScore,
				AlertAssessed:   true,
				AlertScore:      &alertScore,
				MetricsAssessed: true,
				MetricsScore:    &metricsScore,
			}

			// DD-017: score = (1.0*0.40 + 1.0*0.35 + 0.8*0.25) / (0.40+0.35+0.25)
			// = (0.40 + 0.35 + 0.20) / 1.00 = 0.95
			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 0.95, 0.001)))
		})

		// UT-DS-EFF-002: Two components (health + alert), metrics missing
		It("UT-DS-EFF-002: should redistribute weights when metrics is missing", func() {
			healthScore := 1.0
			alertScore := 0.0

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     &healthScore,
				AlertAssessed:   true,
				AlertScore:      &alertScore,
				MetricsAssessed: false,
			}

			// DD-017 redistribution: health=0.40/(0.40+0.35)=0.5333, alert=0.35/(0.40+0.35)=0.4667
			// score = (1.0*0.5333 + 0.0*0.4667) = 0.5333
			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 0.5333, 0.001)))
		})

		// UT-DS-EFF-003: One component (health only)
		It("UT-DS-EFF-003: should normalize to 1.0 weight for single component", func() {
			healthScore := 0.75

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     &healthScore,
				AlertAssessed:   false,
				MetricsAssessed: false,
			}

			// With only health: score = 0.75 * (0.40/0.40) = 0.75
			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 0.75, 0.001)))
		})

		// UT-DS-EFF-004: No components assessed -> nil score
		It("UT-DS-EFF-004: should return nil score when no components assessed", func() {
			components := &server.EffectivenessComponents{
				HealthAssessed:  false,
				AlertAssessed:   false,
				MetricsAssessed: false,
			}

			score := server.ComputeWeightedScore(components)
			Expect(score).To(BeNil())
		})

		// UT-DS-EFF-005: Perfect score (all 1.0)
		It("UT-DS-EFF-005: should return 1.0 for perfect scores", func() {
			h, a, m := 1.0, 1.0, 1.0

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     &h,
				AlertAssessed:   true,
				AlertScore:      &a,
				MetricsAssessed: true,
				MetricsScore:    &m,
			}

			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 1.0, 0.001)))
		})

		// UT-DS-EFF-006: Zero score (all 0.0)
		It("UT-DS-EFF-006: should return 0.0 for zero scores", func() {
			h, a, m := 0.0, 0.0, 0.0

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     &h,
				AlertAssessed:   true,
				AlertScore:      &a,
				MetricsAssessed: true,
				MetricsScore:    &m,
			}

			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 0.0, 0.001)))
		})

		// UT-DS-EFF-007: Components assessed but score is nil (assessed=true, score=nil)
		It("UT-DS-EFF-007: should skip components with nil score even when assessed", func() {
			alertScore := 1.0

			components := &server.EffectivenessComponents{
				HealthAssessed:  true,
				HealthScore:     nil, // assessed but no score
				AlertAssessed:   true,
				AlertScore:      &alertScore,
				MetricsAssessed: false,
			}

			// Only alert has a score, health assessed but nil score -> not included in weighted calc
			// score = 1.0 * (0.35/0.35) = 1.0
			score := server.ComputeWeightedScore(components)
			Expect(score).To(HaveValue(BeNumerically("~", 1.0, 0.001)))
		})
	})

	// ========================================
	// BuildEffectivenessResponse - Event List to Response
	// ========================================
	Describe("BuildEffectivenessResponse", func() {

		// UT-DS-EFF-008: Full event set -> complete response
		It("UT-DS-EFF-008: should build complete response from full event set", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      1.0,
						"details":    "All pods healthy",
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.alert.assessed",
						"assessed":   true,
						"score":      1.0,
						"details":    "Alert resolved",
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.metrics.assessed",
						"assessed":   true,
						"score":      0.8,
						"details":    "Metrics improved",
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type":                  "effectiveness.hash.computed",
						"post_remediation_spec_hash":  "sha256:aaa",
						"pre_remediation_spec_hash":   "sha256:bbb",
						"hash_match":                  false,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "full",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-test-001", events)

			Expect(resp.CorrelationID).To(Equal("rr-test-001"))
			Expect(resp.Score).To(HaveValue(BeNumerically("~", 0.95, 0.001)))
			Expect(resp.AssessmentStatus).To(Equal("full"))
			Expect(resp.Components.HealthAssessed).To(BeTrue())
			Expect(resp.Components.AlertAssessed).To(BeTrue())
			Expect(resp.Components.MetricsAssessed).To(BeTrue())
			Expect(resp.HashComparison.PostHash).To(Equal("sha256:aaa"))
			Expect(resp.HashComparison.PreHash).To(Equal("sha256:bbb"))
			Expect(resp.HashComparison.Match).To(HaveValue(BeFalse()))
		})

		// UT-DS-EFF-009: Partial events -> in_progress status
		It("UT-DS-EFF-009: should produce in_progress status for partial events", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      1.0,
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-test-002", events)

			Expect(resp.AssessmentStatus).To(Equal("in_progress"))
			Expect(resp.Components.HealthAssessed).To(BeTrue())
			Expect(resp.Components.AlertAssessed).To(BeFalse())
		})

		// UT-DS-EFF-010: Empty events -> no_data status
		It("UT-DS-EFF-010: should produce no_data status for empty events", func() {
			events := []*server.EffectivenessEvent{}

			resp := server.BuildEffectivenessResponse("rr-test-003", events)

			Expect(resp.AssessmentStatus).To(Equal("no_data"))
			Expect(resp.Score).To(BeNil())
		})

		// UT-DS-EFF-011: Hash comparison data extracted correctly
		It("UT-DS-EFF-011: should extract hash comparison data from hash.computed event", func() {
			match := true
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type":                  "effectiveness.hash.computed",
						"post_remediation_spec_hash":  "sha256:same",
						"pre_remediation_spec_hash":   "sha256:same",
						"hash_match":                  match,
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-test-004", events)

			Expect(resp.HashComparison.PostHash).To(Equal("sha256:same"))
			Expect(resp.HashComparison.PreHash).To(Equal("sha256:same"))
			Expect(resp.HashComparison.Match).To(HaveValue(BeTrue()))
		})

		// UT-DS-EFF-012: Nil EventData skipped
		It("UT-DS-EFF-012: should skip events with nil EventData", func() {
			events := []*server.EffectivenessEvent{
				{EventData: nil},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      0.5,
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-test-005", events)

			Expect(resp.Components.HealthAssessed).To(BeTrue())
			Expect(*resp.Components.HealthScore).To(BeNumerically("~", 0.5, 0.001))
		})

		// UT-DS-EFF-013: Spec drift short-circuit to score 0.0 (DD-EM-002 v1.1)
		It("UT-DS-EFF-013: should short-circuit to score 0.0 when assessment reason is spec_drift", func() {
			// Even with a healthy health score, spec drift should override to 0.0
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      0.9,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "spec_drift",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-drift-001", events)

			Expect(resp.AssessmentStatus).To(Equal("spec_drift"))
			Expect(resp.Score).To(HaveValue(Equal(0.0)),
				"spec_drift should short-circuit to score 0.0 regardless of component scores")
		})

		// UT-DS-EFF-014: Spec drift overrides even all-component scores
		It("UT-DS-EFF-014: should return 0.0 even when all components have high scores", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      1.0,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.alert.assessed",
						"assessed":   true,
						"score":      1.0,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.metrics.assessed",
						"assessed":   true,
						"score":      0.9,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "spec_drift",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-drift-002", events)

			Expect(resp.AssessmentStatus).To(Equal("spec_drift"))
			Expect(resp.Score).To(HaveValue(Equal(0.0)),
				"spec_drift overrides all component scores to 0.0")
		})

		// UT-DS-211-004: spec_drift has priority regardless of event ordering (DD-EM-002 v1.1)
		// When two assessment.completed events exist (e.g., one "full" and one "spec_drift"),
		// spec_drift always takes precedence because it invalidates the assessment.
		It("UT-DS-211-004: spec_drift takes priority when full comes first", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      0.9,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "full",
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "spec_drift",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-order-001", events)

			Expect(resp.AssessmentStatus).To(Equal("spec_drift"),
				"DD-EM-002 v1.1: spec_drift must take priority over full")
			Expect(resp.Score).To(HaveValue(Equal(0.0)),
				"spec_drift short-circuits to score 0.0")
		})

		// UT-DS-211-004b: spec_drift has priority even when it arrives before full
		// Covers the edge case where event ordering puts spec_drift before full
		// (e.g., same timestamp with non-deterministic UUID ordering).
		It("UT-DS-211-004b: spec_drift takes priority even when full comes after", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "spec_drift",
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "full",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-order-002", events)

			Expect(resp.AssessmentStatus).To(Equal("spec_drift"),
				"DD-EM-002 v1.1: spec_drift must not be overwritten by later full event")
			Expect(resp.Score).To(HaveValue(Equal(0.0)),
				"spec_drift short-circuits to score 0.0")
		})

		// UT-DS-EFF-015: Non-drift assessment still computes normally
		It("UT-DS-EFF-015: should compute normal weighted score for non-drift reasons", func() {
			events := []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.health.assessed",
						"assessed":   true,
						"score":      1.0,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "full",
					},
				},
			}

			resp := server.BuildEffectivenessResponse("rr-normal-001", events)

			Expect(resp.AssessmentStatus).To(Equal("full"))
			Expect(resp.Score).To(HaveValue(BeNumerically(">", 0.0)),
				"non-drift assessment should compute a positive score from components")
		})
	})
})
