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

// Package datastorage contains unit tests for the DataStorage service.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.1: Pure correlation logic tests -- ComputeHashMatch,
// CorrelateTier1Chain, BuildTier2Summaries, DetectRegression.
package datastorage

import (
	"time"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Remediation History Correlation Logic (DD-HAPI-016 v1.1)", func() {

	// Fixed timestamps for deterministic testing.
	var (
		fixedTime = time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
		laterTime = time.Date(2026, 2, 12, 11, 0, 0, 0, time.UTC)
	)

	// makeROEvent creates a RawAuditRow simulating a remediation.workflow_created event.
	makeROEvent := func(correlationID, preHash, outcome, signalType, signalFingerprint, workflowType string, ts time.Time) repository.RawAuditRow {
		return repository.RawAuditRow{
			EventType:      "remediation.workflow_created",
			CorrelationID:  correlationID,
			EventTimestamp: ts,
			EventData: map[string]interface{}{
				"pre_remediation_spec_hash": preHash,
				"outcome":                  outcome,
				"signal_type":              signalType,
				"signal_fingerprint":       signalFingerprint,
				"workflow_type":            workflowType,
				"target_resource":          "default/Deployment/nginx",
			},
		}
	}

	// makeFullEMEvents creates a complete set of EM component events with typed sub-objects.
	// Per ADR-EM-001 v1.3: health, alert, metrics, hash, completed.
	makeFullEMEvents := func() []*server.EffectivenessEvent {
		return []*server.EffectivenessEvent{
			{
				EventData: map[string]interface{}{
					"event_type": "effectiveness.health.assessed",
					"assessed":   true,
					"score":      1.0,
					"details":    "All pods healthy",
					"health_checks": map[string]interface{}{
						"pod_running":    true,
						"readiness_pass": true,
						"restart_delta":  float64(0),
						"pending_count":  float64(0),
					},
				},
			},
			{
				EventData: map[string]interface{}{
					"event_type": "effectiveness.alert.assessed",
					"assessed":   true,
					"score":      1.0,
					"details":    "Alert resolved",
					"alert_resolution": map[string]interface{}{
						"alert_resolved":          true,
						"active_count":            float64(0),
						"resolution_time_seconds": 120.5,
					},
				},
			},
			{
				EventData: map[string]interface{}{
					"event_type": "effectiveness.metrics.assessed",
					"assessed":   true,
					"score":      0.8,
					"details":    "Metrics improved",
					"metric_deltas": map[string]interface{}{
						"cpu_before": 0.85,
						"cpu_after":  0.45,
					},
				},
			},
			{
				EventData: map[string]interface{}{
					"event_type":                 "effectiveness.hash.computed",
					"pre_remediation_spec_hash":  "sha256:pre123",
					"post_remediation_spec_hash": "sha256:post456",
					"hash_match":                 false,
				},
			},
			{
				EventData: map[string]interface{}{
					"event_type": "effectiveness.assessment.completed",
					"reason":     "full",
				},
			},
		}
	}

	// ========================================
	// ComputeHashMatch -- three-way hash comparison
	// DD-HAPI-016 v1.1: currentSpecHash vs preHash vs postHash
	// ========================================
	Describe("ComputeHashMatch", func() {

		It("UT-RH-LOGIC-001: should return postRemediation when current matches postHash", func() {
			result := server.ComputeHashMatch("sha256:abc", "sha256:def", "sha256:abc")
			Expect(result).To(Equal(api.RemediationHistoryEntryHashMatchPostRemediation))
		})

		It("UT-RH-LOGIC-002: should return preRemediation when current matches preHash (regression)", func() {
			result := server.ComputeHashMatch("sha256:abc", "sha256:abc", "sha256:def")
			Expect(result).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation))
		})

		It("UT-RH-LOGIC-003: should return none when current matches neither hash", func() {
			result := server.ComputeHashMatch("sha256:abc", "sha256:def", "sha256:ghi")
			Expect(result).To(Equal(api.RemediationHistoryEntryHashMatchNone))
		})

		It("UT-RH-LOGIC-004: should return preRemediation when current matches both (regression priority)", func() {
			// When current matches both preHash and postHash (i.e., pre==post==current),
			// regression detection takes priority per DD-HAPI-016 v1.1.
			result := server.ComputeHashMatch("sha256:abc", "sha256:abc", "sha256:abc")
			Expect(result).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation))
		})
	})

	// ========================================
	// CorrelateTier1Chain -- joins RO + EM events into detailed entries
	// DD-HAPI-016 v1.1 Steps 1-3: two-step query + correlation
	// ========================================
	Describe("CorrelateTier1Chain", func() {

		It("UT-RH-LOGIC-005: should build full entry with EM data, score, healthChecks, metricDeltas, signalResolved", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-001", "sha256:pre123", "success", "alert", "fp-alert-001", "restart", fixedTime),
			}
			emEvents := map[string][]*server.EffectivenessEvent{
				"rr-001": makeFullEMEvents(),
			}

			entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

			Expect(entries).To(HaveLen(1))
			entry := entries[0]

			// Basic RO fields
			Expect(entry.RemediationUID).To(Equal("rr-001"))
			Expect(entry.SignalType.Value).To(Equal("alert"))
			Expect(entry.SignalFingerprint.Value).To(Equal("fp-alert-001"))
			Expect(entry.WorkflowType.Value).To(Equal("restart"))
			Expect(entry.Outcome.Value).To(Equal("success"))
			Expect(entry.PreRemediationSpecHash.Value).To(Equal("sha256:pre123"))
			Expect(entry.CompletedAt).To(Equal(fixedTime))

			// EM-derived: effectiveness score (DD-017 v2.1 weighted)
			// health=1.0*0.40, alert=1.0*0.35, metrics=0.8*0.25 -> (0.40+0.35+0.20)/1.0=0.95
			Expect(entry.EffectivenessScore.Set).To(BeTrue())
			Expect(entry.EffectivenessScore.Value).To(BeNumerically("~", 0.95, 0.001))

			// signalResolved from alert_resolution.alert_resolved
			Expect(entry.SignalResolved.Set).To(BeTrue())
			Expect(entry.SignalResolved.Value).To(BeTrue())

			// postRemediationSpecHash from hash.computed event
			Expect(entry.PostRemediationSpecHash.Set).To(BeTrue())
			Expect(entry.PostRemediationSpecHash.Value).To(Equal("sha256:post456"))

			// hashMatch: current=sha256:current, pre=sha256:pre123, post=sha256:post456 -> none
			Expect(entry.HashMatch.Set).To(BeTrue())
			Expect(entry.HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchNone))

			// healthChecks from typed sub-object
			Expect(entry.HealthChecks.Set).To(BeTrue())
			Expect(entry.HealthChecks.Value.PodRunning.Value).To(BeTrue())
			Expect(entry.HealthChecks.Value.ReadinessPass.Value).To(BeTrue())
			Expect(entry.HealthChecks.Value.RestartDelta.Value).To(Equal(0))
			Expect(entry.HealthChecks.Value.PendingCount.Value).To(Equal(0))

			// metricDeltas from typed sub-object
			Expect(entry.MetricDeltas.Set).To(BeTrue())
			Expect(entry.MetricDeltas.Value.CpuBefore.Value).To(BeNumerically("~", 0.85, 0.001))
			Expect(entry.MetricDeltas.Value.CpuAfter.Value).To(BeNumerically("~", 0.45, 0.001))
		})

		It("UT-RH-LOGIC-006: should build entry with nil effectiveness when no EM data", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-002", "sha256:pre789", "success", "alert", "fp-002", "restart", fixedTime),
			}
			emEvents := map[string][]*server.EffectivenessEvent{} // no EM data

			entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

			Expect(entries).To(HaveLen(1))
			entry := entries[0]

			// RO fields still populated
			Expect(entry.RemediationUID).To(Equal("rr-002"))
			Expect(entry.Outcome.Value).To(Equal("success"))

			// EM fields are nil/unset
			Expect(entry.EffectivenessScore.Set).To(BeFalse())
			Expect(entry.SignalResolved.Set).To(BeFalse())
			Expect(entry.HealthChecks.Set).To(BeFalse())
			Expect(entry.MetricDeltas.Set).To(BeFalse())
			Expect(entry.PostRemediationSpecHash.Set).To(BeFalse())

			// hashMatch: no postHash available -> compare with preHash only
			// current=sha256:current, pre=sha256:pre789 -> none
			Expect(entry.HashMatch.Set).To(BeTrue())
			Expect(entry.HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchNone))
		})

		It("UT-RH-LOGIC-007: should sort multiple entries by completedAt descending", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-older", "sha256:pre1", "success", "alert", "fp-1", "restart", fixedTime),
				makeROEvent("rr-newer", "sha256:pre2", "failed", "alert", "fp-2", "scale", laterTime),
			}
			emEvents := map[string][]*server.EffectivenessEvent{}

			entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

			Expect(entries).To(HaveLen(2))
			// Descending order: newer first
			Expect(entries[0].RemediationUID).To(Equal("rr-newer"))
			Expect(entries[1].RemediationUID).To(Equal("rr-older"))
		})

		It("UT-RH-LOGIC-008: should handle partially populated EM data (nil sub-fields)", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-partial", "sha256:pre", "success", "alert", "fp-3", "restart", fixedTime),
			}
			// Health event with only pod_running/readiness_pass -- no crash_loops, oom_killed, pending_count
			emEvents := map[string][]*server.EffectivenessEvent{
				"rr-partial": {
					{
						EventData: map[string]interface{}{
							"event_type": "effectiveness.health.assessed",
							"assessed":   true,
							"score":      0.75,
							"health_checks": map[string]interface{}{
								"pod_running":    true,
								"readiness_pass": true,
								// crash_loops, oom_killed, pending_count intentionally absent
							},
						},
					},
				},
			}

			entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

			Expect(entries).To(HaveLen(1))
			entry := entries[0]
			Expect(entry.HealthChecks.Set).To(BeTrue())
			Expect(entry.HealthChecks.Value.PodRunning.Set).To(BeTrue())
			Expect(entry.HealthChecks.Value.PodRunning.Value).To(BeTrue())
			// Missing fields should be unset (Opt types default to Set=false)
			Expect(entry.HealthChecks.Value.CrashLoops.Set).To(BeFalse())
			Expect(entry.HealthChecks.Value.OomKilled.Set).To(BeFalse())
			Expect(entry.HealthChecks.Value.PendingCount.Set).To(BeFalse())
		})

		It("UT-RH-LOGIC-009: should detect regression when current matches preHash", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-regress", "sha256:current", "success", "alert", "fp-4", "restart", fixedTime),
			}
			// EM events with postHash different from current
			emEvents := map[string][]*server.EffectivenessEvent{
				"rr-regress": {
					{
						EventData: map[string]interface{}{
							"event_type":                 "effectiveness.hash.computed",
							"pre_remediation_spec_hash":  "sha256:current",
							"post_remediation_spec_hash": "sha256:fixed",
							"hash_match":                 false,
						},
					},
				},
			}

			entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")
			Expect(entries).To(HaveLen(1))
			// currentSpecHash matches preHash -> preRemediation (regression)
			Expect(entries[0].HashMatch.Set).To(BeTrue())
			Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation))
		})

		It("UT-RH-LOGIC-010: should return empty slice for no RO events", func() {
			entries := server.CorrelateTier1Chain(nil, nil, "sha256:current")
			Expect(entries).To(BeEmpty())
		})
	})

	// ========================================
	// BuildTier2Summaries -- summary entries for Tier 2 (wider time window)
	// DD-HAPI-016 v1.1 Step 4: historical hash lookup
	// ========================================
	Describe("BuildTier2Summaries", func() {

		It("UT-RH-LOGIC-011: should build summary entries with score and hashMatch", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-t2-001", "sha256:pre", "success", "alert", "fp-t2", "restart", fixedTime),
			}
			emEvents := map[string][]*server.EffectivenessEvent{
				"rr-t2-001": {
					{
						EventData: map[string]interface{}{
							"event_type": "effectiveness.health.assessed",
							"assessed":   true,
							"score":      0.9,
						},
					},
					{
						EventData: map[string]interface{}{
							"event_type":                 "effectiveness.hash.computed",
							"pre_remediation_spec_hash":  "sha256:pre",
							"post_remediation_spec_hash": "sha256:post",
							"hash_match":                 false,
						},
					},
					{
						EventData: map[string]interface{}{
							"event_type":    "effectiveness.alert.assessed",
							"assessed":      true,
							"score":         1.0,
							"alert_resolution": map[string]interface{}{
								"alert_resolved": true,
							},
						},
					},
				},
			}

			summaries := server.BuildTier2Summaries(roEvents, emEvents, "sha256:current")

			Expect(summaries).To(HaveLen(1))
			s := summaries[0]
			Expect(s.RemediationUID).To(Equal("rr-t2-001"))
			Expect(s.SignalType.Value).To(Equal("alert"))
			Expect(s.WorkflowType.Value).To(Equal("restart"))
			Expect(s.Outcome.Value).To(Equal("success"))
			Expect(s.CompletedAt).To(Equal(fixedTime))

			// Score populated from EM (health-only -> 0.9)
			Expect(s.EffectivenessScore.Set).To(BeTrue())

			// signalResolved from alert_resolution
			Expect(s.SignalResolved.Set).To(BeTrue())
			Expect(s.SignalResolved.Value).To(BeTrue())

			// hashMatch: current=sha256:current, pre=sha256:pre, post=sha256:post -> none
			Expect(s.HashMatch.Set).To(BeTrue())
			Expect(s.HashMatch.Value).To(Equal(api.RemediationHistorySummaryHashMatchNone))
		})

		It("UT-RH-LOGIC-012: should return empty slice for no events", func() {
			summaries := server.BuildTier2Summaries(nil, nil, "sha256:current")
			Expect(summaries).To(BeEmpty())
		})

		It("UT-RH-LOGIC-013: should sort summaries by completedAt descending", func() {
			roEvents := []repository.RawAuditRow{
				makeROEvent("rr-t2-old", "sha256:pre1", "success", "alert", "fp-1", "restart", fixedTime),
				makeROEvent("rr-t2-new", "sha256:pre2", "failed", "alert", "fp-2", "scale", laterTime),
			}
			emEvents := map[string][]*server.EffectivenessEvent{}

			summaries := server.BuildTier2Summaries(roEvents, emEvents, "sha256:current")

			Expect(summaries).To(HaveLen(2))
			Expect(summaries[0].RemediationUID).To(Equal("rr-t2-new"))
			Expect(summaries[1].RemediationUID).To(Equal("rr-t2-old"))
		})
	})

	// ========================================
	// AssessmentReason propagation -- spec_drift / full / partial
	// DD-EM-002 v1.1: assessmentReason from BuildEffectivenessResponse
	// ========================================
	Describe("AssessmentReason propagation", func() {

		// makeSpecDriftEMEvents creates EM events where assessment.completed reason = "spec_drift".
		// BuildEffectivenessResponse short-circuits score to 0.0 for spec_drift.
		makeSpecDriftEMEvents := func() []*server.EffectivenessEvent {
			return []*server.EffectivenessEvent{
				{
					EventData: map[string]interface{}{
						"event_type":                 "effectiveness.hash.computed",
						"pre_remediation_spec_hash":  "sha256:pre-sd",
						"post_remediation_spec_hash": "sha256:post-sd",
						"hash_match":                 false,
					},
				},
				{
					EventData: map[string]interface{}{
						"event_type": "effectiveness.assessment.completed",
						"reason":     "spec_drift",
					},
				},
			}
		}

		Context("Tier 1 (CorrelateTier1Chain)", func() {

			It("UT-RH-LOGIC-017: should propagate assessmentReason='spec_drift' with score 0.0", func() {
				roEvents := []repository.RawAuditRow{
					makeROEvent("rr-sd-001", "sha256:pre-sd", "success", "alert", "fp-sd-1", "restart", fixedTime),
				}
				emEvents := map[string][]*server.EffectivenessEvent{
					"rr-sd-001": makeSpecDriftEMEvents(),
				}

				entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

				Expect(entries).To(HaveLen(1))
				entry := entries[0]

				// assessmentReason must be propagated
				Expect(entry.AssessmentReason.Set).To(BeTrue())
				Expect(string(entry.AssessmentReason.Value)).To(Equal("spec_drift"))

				// score is 0.0 (spec_drift short-circuit in BuildEffectivenessResponse)
				Expect(entry.EffectivenessScore.Set).To(BeTrue())
				Expect(entry.EffectivenessScore.Value).To(BeNumerically("==", 0.0))
			})

			It("UT-RH-LOGIC-018: should propagate assessmentReason='full' with normal score", func() {
				roEvents := []repository.RawAuditRow{
					makeROEvent("rr-full-001", "sha256:pre123", "success", "alert", "fp-full-1", "restart", fixedTime),
				}
				emEvents := map[string][]*server.EffectivenessEvent{
					"rr-full-001": makeFullEMEvents(),
				}

				entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

				Expect(entries).To(HaveLen(1))
				entry := entries[0]

				// assessmentReason must be "full"
				Expect(entry.AssessmentReason.Set).To(BeTrue())
				Expect(string(entry.AssessmentReason.Value)).To(Equal("full"))

				// score is normal (not 0.0)
				Expect(entry.EffectivenessScore.Set).To(BeTrue())
				Expect(entry.EffectivenessScore.Value).To(BeNumerically(">", 0.0))
			})

			It("UT-RH-LOGIC-019: should leave assessmentReason unset when no EM data", func() {
				roEvents := []repository.RawAuditRow{
					makeROEvent("rr-no-em", "sha256:pre", "success", "alert", "fp-1", "restart", fixedTime),
				}
				emEvents := map[string][]*server.EffectivenessEvent{}

				entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:current")

				Expect(entries).To(HaveLen(1))
				// No EM events -> assessmentReason should be unset
				Expect(entries[0].AssessmentReason.Set).To(BeFalse())
			})
		})

		Context("Tier 2 (BuildTier2Summaries)", func() {

			It("UT-RH-LOGIC-020: should propagate assessmentReason='spec_drift' on summary", func() {
				roEvents := []repository.RawAuditRow{
					makeROEvent("rr-t2-sd", "sha256:pre-sd", "success", "alert", "fp-sd", "restart", fixedTime),
				}
				emEvents := map[string][]*server.EffectivenessEvent{
					"rr-t2-sd": makeSpecDriftEMEvents(),
				}

				summaries := server.BuildTier2Summaries(roEvents, emEvents, "sha256:current")

				Expect(summaries).To(HaveLen(1))
				s := summaries[0]

				Expect(s.AssessmentReason.Set).To(BeTrue())
				Expect(string(s.AssessmentReason.Value)).To(Equal("spec_drift"))

				// score is 0.0
				Expect(s.EffectivenessScore.Set).To(BeTrue())
				Expect(s.EffectivenessScore.Value).To(BeNumerically("==", 0.0))
			})

			It("UT-RH-LOGIC-021: should propagate assessmentReason='full' on summary", func() {
				roEvents := []repository.RawAuditRow{
					makeROEvent("rr-t2-full", "sha256:pre123", "success", "alert", "fp-full", "restart", fixedTime),
				}
				emEvents := map[string][]*server.EffectivenessEvent{
					"rr-t2-full": makeFullEMEvents(),
				}

				summaries := server.BuildTier2Summaries(roEvents, emEvents, "sha256:current")

				Expect(summaries).To(HaveLen(1))
				s := summaries[0]

				Expect(s.AssessmentReason.Set).To(BeTrue())
				Expect(string(s.AssessmentReason.Value)).To(Equal("full"))
			})
		})
	})

	// ========================================
	// DetectRegression -- checks if any entry has preRemediation hashMatch
	// DD-HAPI-016 v1.1: regression = current spec matches a previous preHash
	// ========================================
	Describe("DetectRegression", func() {

		It("UT-RH-LOGIC-014: should return true when any entry has preRemediation hashMatch", func() {
			entries := []api.RemediationHistoryEntry{
				{
					RemediationUID: "rr-1",
					HashMatch: api.OptRemediationHistoryEntryHashMatch{
						Value: api.RemediationHistoryEntryHashMatchNone,
						Set:   true,
					},
				},
				{
					RemediationUID: "rr-2",
					HashMatch: api.OptRemediationHistoryEntryHashMatch{
						Value: api.RemediationHistoryEntryHashMatchPreRemediation,
						Set:   true,
					},
				},
			}

			Expect(server.DetectRegression(entries)).To(BeTrue())
		})

		It("UT-RH-LOGIC-015: should return false when no entry has preRemediation hashMatch", func() {
			entries := []api.RemediationHistoryEntry{
				{
					RemediationUID: "rr-1",
					HashMatch: api.OptRemediationHistoryEntryHashMatch{
						Value: api.RemediationHistoryEntryHashMatchPostRemediation,
						Set:   true,
					},
				},
				{
					RemediationUID: "rr-2",
					HashMatch: api.OptRemediationHistoryEntryHashMatch{
						Value: api.RemediationHistoryEntryHashMatchNone,
						Set:   true,
					},
				},
			}

			Expect(server.DetectRegression(entries)).To(BeFalse())
		})

		It("UT-RH-LOGIC-016: should return false for empty entries", func() {
			Expect(server.DetectRegression(nil)).To(BeFalse())
			Expect(server.DetectRegression([]api.RemediationHistoryEntry{})).To(BeFalse())
		})
	})
})
