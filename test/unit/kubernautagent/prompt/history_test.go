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

package prompt_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

func boolPtr(v bool) *bool       { return &v }
func floatPtr(v float64) *float64 { return &v }

var _ = Describe("Remediation History Prompt Builder — KA Parity (#433)", func() {

	Describe("UT-KA-433-HP-001: EffectivenessLevel classification", func() {
		It("should return 'good' for score >= 0.7", func() {
			Expect(prompt.EffectivenessLevel(floatPtr(0.85))).To(Equal("good"))
			Expect(prompt.EffectivenessLevel(floatPtr(0.7))).To(Equal("good"))
		})

		It("should return 'moderate' for score >= 0.4 and < 0.7", func() {
			Expect(prompt.EffectivenessLevel(floatPtr(0.5))).To(Equal("moderate"))
			Expect(prompt.EffectivenessLevel(floatPtr(0.4))).To(Equal("moderate"))
		})

		It("should return 'poor' for score < 0.4", func() {
			Expect(prompt.EffectivenessLevel(floatPtr(0.2))).To(Equal("poor"))
			Expect(prompt.EffectivenessLevel(floatPtr(0.0))).To(Equal("poor"))
		})

		It("should return 'unknown' for nil score", func() {
			Expect(prompt.EffectivenessLevel(nil)).To(Equal("unknown"))
		})
	})

	Describe("UT-KA-433-HP-002: FormatHealthChecks", func() {
		It("should format all health check fields", func() {
			hc := &enrichment.HealthChecks{
				PodRunning:    boolPtr(true),
				ReadinessPass: boolPtr(true),
				RestartDelta:  intPtr(0),
				CrashLoops:    boolPtr(false),
				OomKilled:     boolPtr(false),
			}
			result := prompt.FormatHealthChecks(hc)
			Expect(result).To(ContainSubstring("pod_running=yes"))
			Expect(result).To(ContainSubstring("readiness=pass"))
			Expect(result).To(ContainSubstring("restart_delta=0"))
			Expect(result).To(ContainSubstring("crash_loops=no"))
			Expect(result).To(ContainSubstring("oom_killed=no"))
		})

		It("should return N/A for nil health checks", func() {
			Expect(prompt.FormatHealthChecks(nil)).To(Equal("N/A"))
		})

		It("should include pending_pods when pendingCount > 0", func() {
			hc := &enrichment.HealthChecks{PendingCount: intPtr(3)}
			result := prompt.FormatHealthChecks(hc)
			Expect(result).To(ContainSubstring("pending_pods=3"))
		})
	})

	Describe("UT-KA-433-HP-003: FormatMetricDeltas", func() {
		It("should format before->after metric pairs", func() {
			md := &enrichment.MetricDeltas{
				CpuBefore:    floatPtr(0.85),
				CpuAfter:     floatPtr(0.45),
				MemoryBefore: floatPtr(0.9),
				MemoryAfter:  floatPtr(0.6),
			}
			result := prompt.FormatMetricDeltas(md)
			Expect(result).To(ContainSubstring("cpu: 0.85 -> 0.45"))
			Expect(result).To(ContainSubstring("memory: 0.9 -> 0.6"))
		})

		It("should return N/A for nil metric deltas", func() {
			Expect(prompt.FormatMetricDeltas(nil)).To(Equal("N/A"))
		})
	})

	Describe("UT-KA-433-HP-004: FormatTier1Entry — normal entry", func() {
		It("should format a normal Tier1 entry with all fields", func() {
			entry := enrichment.Tier1Entry{
				RemediationUID:     "wf-001",
				ActionType:         "increase_memory",
				Outcome:            "Success",
				SignalType:         "OOMKilled",
				EffectivenessScore: floatPtr(0.85),
				SignalResolved:     boolPtr(true),
				HashMatch:          "postRemediation",
				CompletedAt:        time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
				HealthChecks: &enrichment.HealthChecks{
					PodRunning: boolPtr(true),
				},
			}
			result := prompt.FormatTier1Entry(entry, nil)
			Expect(result).To(ContainSubstring("Remediation wf-001"))
			Expect(result).To(ContainSubstring("increase_memory"))
			Expect(result).To(ContainSubstring("Effectiveness: 0.85 (good)"))
			Expect(result).To(ContainSubstring("Signal resolved: YES"))
			Expect(result).To(ContainSubstring("Hash match: postRemediation"))
			Expect(result).To(ContainSubstring("Health:"))
		})
	})

	Describe("UT-KA-433-HP-005: FormatTier1Entry — spec_drift entry", func() {
		It("should render INCONCLUSIVE for spec_drift entries", func() {
			entry := enrichment.Tier1Entry{
				RemediationUID:   "wf-drift-001",
				ActionType:       "restart_pod",
				Outcome:          "Success",
				AssessmentReason: "spec_drift",
				CompletedAt:      time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			}
			result := prompt.FormatTier1Entry(entry, nil)
			Expect(result).To(ContainSubstring("INCONCLUSIVE (spec drift)"))
			Expect(result).NotTo(ContainSubstring("Effectiveness:"))
			Expect(result).NotTo(ContainSubstring("Health:"))
		})

		It("should use causal chain variant when follow-up UID is found", func() {
			entry := enrichment.Tier1Entry{
				RemediationUID:          "wf-drift-001",
				ActionType:              "restart_pod",
				Outcome:                 "Success",
				AssessmentReason:        "spec_drift",
				PostRemediationSpecHash: "sha256:aaa",
				CompletedAt:             time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			}
			causalChains := map[string]string{"wf-drift-001": "wf-followup-002"}
			result := prompt.FormatTier1Entry(entry, causalChains)
			Expect(result).To(ContainSubstring("INCONCLUSIVE (spec drift -- led to follow-up remediation)"))
			Expect(result).To(ContainSubstring("wf-followup-002"))
		})
	})

	Describe("UT-KA-433-HP-006: FormatTier2Summary", func() {
		It("should format a compact Tier2 summary", func() {
			s := enrichment.Tier2Summary{
				RemediationUID:     "wf-old-001",
				ActionType:         "restart_pod",
				Outcome:            "Failed",
				EffectivenessScore: floatPtr(0.2),
				HashMatch:          "none",
				CompletedAt:        time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC),
			}
			result := prompt.FormatTier2Summary(s)
			Expect(result).To(ContainSubstring("wf-old-001"))
			Expect(result).To(ContainSubstring("restart_pod"))
			Expect(result).To(ContainSubstring("Failed"))
			Expect(result).To(ContainSubstring("0.20 (poor)"))
		})

		It("should render INCONCLUSIVE for spec_drift tier2 entries", func() {
			s := enrichment.Tier2Summary{
				RemediationUID:   "wf-drift-old",
				ActionType:       "scale_up",
				Outcome:          "Success",
				AssessmentReason: "spec_drift",
				CompletedAt:      time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC),
			}
			result := prompt.FormatTier2Summary(s)
			Expect(result).To(ContainSubstring("INCONCLUSIVE (spec drift)"))
		})
	})

	Describe("UT-KA-433-HP-007: DetectDecliningEffectiveness", func() {
		It("should detect strictly declining scores for an action type with >= 3 entries", func() {
			chain := []enrichment.Tier1Entry{
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.8)},
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.5)},
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.2)},
			}
			declining := prompt.DetectDecliningEffectiveness(chain)
			Expect(declining).To(ConsistOf("restart_pod"))
		})

		It("should not detect when fewer than 3 entries", func() {
			chain := []enrichment.Tier1Entry{
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.8)},
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.5)},
			}
			Expect(prompt.DetectDecliningEffectiveness(chain)).To(BeEmpty())
		})

		It("should exclude spec_drift entries from declining detection", func() {
			chain := []enrichment.Tier1Entry{
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.8)},
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.0), AssessmentReason: "spec_drift"},
				{ActionType: "restart_pod", EffectivenessScore: floatPtr(0.9)},
			}
			Expect(prompt.DetectDecliningEffectiveness(chain)).To(BeEmpty())
		})
	})

	Describe("UT-KA-433-HP-008: DetectCompletedButRecurring", func() {
		It("should detect recurring completed remediations for same action+signal", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success"},
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success"},
			}
			recurring := prompt.DetectCompletedButRecurring(entries, 2)
			Expect(recurring).To(HaveLen(1))
			Expect(recurring[0].ActionType).To(Equal("restart_pod"))
			Expect(recurring[0].Count).To(Equal(2))
			Expect(recurring[0].SignalType).To(Equal("OOMKilled"))
		})

		It("should not count spec_drift entries", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success"},
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", AssessmentReason: "spec_drift"},
			}
			recurring := prompt.DetectCompletedButRecurring(entries, 2)
			Expect(recurring).To(BeEmpty())
		})
	})

	Describe("UT-KA-433-HP-009: AllZeroEffectiveness", func() {
		It("should return true when all completed entries have zero effectiveness", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0)},
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Completed", EffectivenessScore: floatPtr(0.0)},
			}
			Expect(prompt.AllZeroEffectiveness(entries, "restart_pod", "OOMKilled")).To(BeTrue())
		})

		It("should return false when any entry has non-zero effectiveness", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0)},
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.3)},
			}
			Expect(prompt.AllZeroEffectiveness(entries, "restart_pod", "OOMKilled")).To(BeFalse())
		})

		It("should return false when signalResolved is true", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0), SignalResolved: boolPtr(true)},
			}
			Expect(prompt.AllZeroEffectiveness(entries, "restart_pod", "OOMKilled")).To(BeFalse())
		})
	})

	Describe("UT-KA-433-HP-010: DetectSpecDriftCausalChains", func() {
		It("should map spec_drift UID to follow-up UID when postHash matches preHash", func() {
			chain := []enrichment.Tier1Entry{
				{
					RemediationUID:          "wf-drift-001",
					AssessmentReason:        "spec_drift",
					PostRemediationSpecHash: "sha256:aaa",
				},
				{
					RemediationUID:         "wf-followup-002",
					PreRemediationSpecHash: "sha256:aaa",
				},
			}
			causal := prompt.DetectSpecDriftCausalChains(chain)
			Expect(causal).To(HaveKeyWithValue("wf-drift-001", "wf-followup-002"))
		})

		It("should return empty map when no causal chains exist", func() {
			chain := []enrichment.Tier1Entry{
				{RemediationUID: "wf-001", PostRemediationSpecHash: "sha256:aaa"},
				{RemediationUID: "wf-002", PreRemediationSpecHash: "sha256:bbb"},
			}
			causal := prompt.DetectSpecDriftCausalChains(chain)
			Expect(causal).To(BeEmpty())
		})
	})

	Describe("UT-KA-725-001: All-zero effectiveness warning references investigation_outcome", func() {
		It("should contain 'investigation_outcome' to 'inconclusive' and not 'needs_human_review'", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource: "default/Pod/web",
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "wf-a", ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0), CompletedAt: time.Now()},
					{RemediationUID: "wf-b", ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0), CompletedAt: time.Now()},
				},
				Tier1Window: "24h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("MANDATORY: You MUST NOT re-select"),
				"all-zero escalation warning must still be present")
			Expect(output).To(ContainSubstring("`investigation_outcome` to `inconclusive`"),
				"all-zero warning must reference investigation_outcome to inconclusive")
			Expect(output).NotTo(ContainSubstring("`needs_human_review`"),
				"all-zero warning must NOT reference non-existent needs_human_review schema field")
		})
	})

	Describe("UT-KA-725-002: Repeated ineffective warning references investigation_outcome", func() {
		It("should contain 'investigation_outcome' to 'inconclusive' and not 'needs_human_review'", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource: "default/Pod/web",
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "wf-a", ActionType: "scale_up", SignalType: "HighLatency", Outcome: "Success", EffectivenessScore: floatPtr(0.3), CompletedAt: time.Now()},
					{RemediationUID: "wf-b", ActionType: "scale_up", SignalType: "HighLatency", Outcome: "Success", EffectivenessScore: floatPtr(0.2), CompletedAt: time.Now()},
				},
				Tier1Window: "24h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("WARNING: REPEATED INEFFECTIVE REMEDIATION"),
				"repeated ineffective warning must be present")
			Expect(output).To(ContainSubstring("`investigation_outcome` to `inconclusive`"),
				"repeated ineffective warning must reference investigation_outcome to inconclusive")
			Expect(output).NotTo(ContainSubstring("`needs_human_review`"),
				"repeated ineffective warning must NOT reference non-existent needs_human_review schema field")
		})
	})

	Describe("UT-KA-433-HP-011: BuildRemediationHistorySection full integration", func() {
		It("should return empty string for nil result", func() {
			Expect(prompt.BuildRemediationHistorySection(nil, 2)).To(BeEmpty())
		})

		It("should return empty string when both tiers are empty", func() {
			result := &enrichment.RemediationHistoryResult{}
			Expect(prompt.BuildRemediationHistorySection(result, 2)).To(BeEmpty())
		})

		It("should render regression warning when regressionDetected is true", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource:     "default/Deployment/api-server",
				RegressionDetected: true,
				Tier1: []enrichment.Tier1Entry{
					{
						RemediationUID: "wf-001",
						ActionType:     "increase_memory",
						Outcome:        "Success",
						HashMatch:      "preRemediation",
						CompletedAt:    time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
					},
				},
				Tier1Window: "24h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("REMEDIATION HISTORY for default/Deployment/api-server"))
			Expect(output).To(ContainSubstring("WARNING: CONFIGURATION REGRESSION DETECTED"))
			Expect(output).To(ContainSubstring("Recent Remediations (last 24h)"))
			Expect(output).To(ContainSubstring("Reasoning Guidance"))
		})

		It("should render both Tier1 and Tier2 sections", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource: "prod/Pod/web-1",
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "wf-t1", ActionType: "restart", Outcome: "Success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
				},
				Tier1Window: "24h",
				Tier2: []enrichment.Tier2Summary{
					{RemediationUID: "wf-t2", ActionType: "scale_up", Outcome: "Failed", CompletedAt: time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)},
				},
				Tier2Window: "2160h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("Recent Remediations (last 24h)"))
			Expect(output).To(ContainSubstring("Historical Remediations (last 2160h)"))
		})

		It("should include spec drift note when spec_drift entries exist", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource: "default/Pod/web",
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "wf-drift", AssessmentReason: "spec_drift", ActionType: "restart", Outcome: "Success", CompletedAt: time.Now()},
				},
				Tier1Window: "24h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("Note on spec drift entries"))
		})

		It("should include MANDATORY escalation warning for all-zero recurring entries", func() {
			result := &enrichment.RemediationHistoryResult{
				TargetResource: "default/Pod/web",
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "wf-a", ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0), CompletedAt: time.Now()},
					{RemediationUID: "wf-b", ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Success", EffectivenessScore: floatPtr(0.0), CompletedAt: time.Now()},
				},
				Tier1Window: "24h",
			}
			output := prompt.BuildRemediationHistorySection(result, 2)
			Expect(output).To(ContainSubstring("MANDATORY: You MUST NOT re-select"))
			Expect(output).To(ContainSubstring("restart_pod"))
		})
	})

	// ========================================
	// Issue #722: Remediated and Inconclusive outcome detection
	// BR-AI-056: KA must detect recurring patterns with new outcome values
	// ========================================
	Describe("UT-KA-722-001: DetectCompletedButRecurring with Remediated outcome", func() {
		It("should detect entries with outcome=Remediated as completed", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "increase_memory", SignalType: "OOMKilled", Outcome: "Remediated"},
				{ActionType: "increase_memory", SignalType: "OOMKilled", Outcome: "Remediated"},
				{ActionType: "increase_memory", SignalType: "OOMKilled", Outcome: "Remediated"},
			}
			recurring := prompt.DetectCompletedButRecurring(entries, 2)
			Expect(recurring).To(HaveLen(1))
			Expect(recurring[0].ActionType).To(Equal("increase_memory"))
			Expect(recurring[0].Count).To(Equal(3))
			Expect(recurring[0].SignalType).To(Equal("OOMKilled"))
		})
	})

	Describe("UT-KA-722-002: DetectCompletedButRecurring with Inconclusive outcome", func() {
		It("should detect entries with outcome=Inconclusive as completed", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "HighCPU", Outcome: "Inconclusive"},
				{ActionType: "restart_pod", SignalType: "HighCPU", Outcome: "Inconclusive"},
			}
			recurring := prompt.DetectCompletedButRecurring(entries, 2)
			Expect(recurring).To(HaveLen(1))
			Expect(recurring[0].ActionType).To(Equal("restart_pod"))
			Expect(recurring[0].Count).To(Equal(2))
			Expect(recurring[0].SignalType).To(Equal("HighCPU"))
		})
	})

	Describe("UT-KA-722-003: AllZeroEffectiveness with Remediated outcome", func() {
		It("should include Remediated entries in zero-effectiveness check", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Remediated", EffectivenessScore: floatPtr(0.0), SignalResolved: boolPtr(false)},
				{ActionType: "restart_pod", SignalType: "OOMKilled", Outcome: "Remediated", EffectivenessScore: floatPtr(0.0), SignalResolved: boolPtr(false)},
			}
			Expect(prompt.AllZeroEffectiveness(entries, "restart_pod", "OOMKilled")).To(BeTrue())
		})
	})

	Describe("UT-KA-722-004: AllZeroEffectiveness with Inconclusive outcome", func() {
		It("should include Inconclusive entries in zero-effectiveness check", func() {
			entries := []prompt.HistoryEntry{
				{ActionType: "scale_up", SignalType: "HighLatency", Outcome: "Inconclusive", EffectivenessScore: nil, SignalResolved: boolPtr(false)},
				{ActionType: "scale_up", SignalType: "HighLatency", Outcome: "Inconclusive", EffectivenessScore: floatPtr(0.0), SignalResolved: boolPtr(false)},
			}
			Expect(prompt.AllZeroEffectiveness(entries, "scale_up", "HighLatency")).To(BeTrue())
		})
	})
})

func intPtr(v int) *int { return &v }
