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

package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

// RepeatedRemediationEscalationThreshold is the minimum count of completed-but-recurring
// remediations before the LLM is warned to escalate (KA constants.py:92).
const RepeatedRemediationEscalationThreshold = 2

// HistoryEntry is a unified interface for detection algorithms that operate on
// both Tier1 and Tier2 entries (recurring detection, all-zero-effectiveness).
type HistoryEntry struct {
	ActionType         string
	SignalType         string
	Outcome            string
	EffectivenessScore *float64
	SignalResolved     *bool
	AssessmentReason   string
}

// RecurringPattern holds a detected completed-but-recurring remediation pattern.
type RecurringPattern struct {
	ActionType string
	Count      int
	SignalType string
}

// remediationHistoryTemplateData matches the remediation_history.tmpl contract.
type remediationHistoryTemplateData struct {
	TargetResource                string
	RegressionDetected            bool
	Tier1Entries                  []string
	Tier1Window                   string
	Tier2Entries                  []string
	Tier2Window                   string
	DecliningEffectivenessWarnings []string
	RecurringRemediationWarnings   []string
	HasSpecDrift                  bool
}

// BuildRemediationHistorySection renders the full remediation history prompt section.
// 1:1 port of KA build_remediation_history_section().
func BuildRemediationHistorySection(result *enrichment.RemediationHistoryResult, escalationThreshold int) string {
	if result == nil {
		return ""
	}
	if len(result.Tier1) == 0 && len(result.Tier2) == 0 {
		return ""
	}

	causalChains := DetectSpecDriftCausalChains(result.Tier1)

	var tier1Rendered []string
	for _, e := range result.Tier1 {
		tier1Rendered = append(tier1Rendered, FormatTier1Entry(e, causalChains))
	}

	var tier2Rendered []string
	for _, e := range result.Tier2 {
		tier2Rendered = append(tier2Rendered, FormatTier2Summary(e))
	}

	declining := DetectDecliningEffectiveness(result.Tier1)
	var decliningWarnings []string
	for _, actionType := range declining {
		decliningWarnings = append(decliningWarnings, fmt.Sprintf(
			"**WARNING: DECLINING EFFECTIVENESS for '%s' workflow** -- "+
				"Each successive application is less effective, suggesting the workflow "+
				"treats the symptom rather than the root cause. Consider a different approach.",
			actionType,
		))
	}

	allEntries := toHistoryEntries(result.Tier1, result.Tier2)
	recurring := DetectCompletedButRecurring(allEntries, escalationThreshold)
	var recurringWarnings []string
	for _, r := range recurring {
		if AllZeroEffectiveness(allEntries, r.ActionType, r.SignalType) {
			recurringWarnings = append(recurringWarnings, fmt.Sprintf(
				"**MANDATORY: You MUST NOT re-select '%s' for signal "+
					"'%s'.** This workflow has been applied %d times with "+
					"zero effectiveness -- the signal continues to recur. Set "+
					"`investigation_outcome` to `inconclusive` and omit `selected_workflow`, "+
					"or select a fundamentally different remediation approach.",
				r.ActionType, r.SignalType, r.Count,
			))
		} else {
			recurringWarnings = append(recurringWarnings, fmt.Sprintf(
				"**WARNING: REPEATED INEFFECTIVE REMEDIATION for '%s'** -- "+
					"Completed %d times for signal '%s' but the issue continues "+
					"to recur. Set `investigation_outcome` to `inconclusive` and omit "+
					"`selected_workflow`, or select an alternative approach.",
				r.ActionType, r.Count, r.SignalType,
			))
		}
	}

	hasSpecDrift := false
	for _, e := range allEntries {
		if e.AssessmentReason == "spec_drift" {
			hasSpecDrift = true
			break
		}
	}

	data := remediationHistoryTemplateData{
		TargetResource:                 result.TargetResource,
		RegressionDetected:             result.RegressionDetected,
		Tier1Entries:                   tier1Rendered,
		Tier1Window:                    result.Tier1Window,
		Tier2Entries:                   tier2Rendered,
		Tier2Window:                    result.Tier2Window,
		DecliningEffectivenessWarnings: decliningWarnings,
		RecurringRemediationWarnings:   recurringWarnings,
		HasSpecDrift:                   hasSpecDrift,
	}

	tmpl, err := template.ParseFS(templateFS, "templates/remediation_history.tmpl")
	if err != nil {
		return fmt.Sprintf("[history template error: %v]", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("[history render error: %v]", err)
	}
	return buf.String()
}

// FormatTier1Entry formats a single Tier1 detailed entry for the prompt.
// 1:1 port of KA _format_tier1_entry().
func FormatTier1Entry(entry enrichment.Tier1Entry, causalChains map[string]string) string {
	completed := entry.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	workflow := withDefault(entry.ActionType, "unknown")
	outcome := withDefault(entry.Outcome, "unknown")
	signal := entry.SignalType

	lines := []string{
		fmt.Sprintf("- **Remediation %s** (%s)", entry.RemediationUID, completed),
		fmt.Sprintf("  Workflow: %s | Outcome: %s | Signal: %s", workflow, outcome, signal),
	}

	if entry.AssessmentReason == "spec_drift" {
		if causalChains != nil {
			if followupUID, ok := causalChains[entry.RemediationUID]; ok {
				lines = append(lines, fmt.Sprintf(
					"  **Assessment: INCONCLUSIVE (spec drift -- led to follow-up remediation)** -- "+
						"The target resource spec changed after this remediation, and a subsequent "+
						"remediation (%s) was triggered from the resulting state. This suggests "+
						"the outcome was unstable, but the workflow may still work under different "+
						"conditions. Use with caution.",
					followupUID,
				))
				return strings.Join(lines, "\n")
			}
		}
		lines = append(lines,
			"  **Assessment: INCONCLUSIVE (spec drift)** -- The target resource spec was "+
				"modified by an external actor during the assessment window, invalidating "+
				"effectiveness data. This workflow may still be viable under different "+
				"conditions. The spec change could indicate a competing controller, GitOps "+
				"sync, or manual edit that is itself relevant to the root cause.",
		)
		return strings.Join(lines, "\n")
	}

	if entry.EffectivenessScore != nil {
		level := EffectivenessLevel(entry.EffectivenessScore)
		lines = append(lines, fmt.Sprintf("  Effectiveness: %.2f (%s)", *entry.EffectivenessScore, level))
	}

	if entry.HashMatch != "" && entry.HashMatch != "none" {
		lines = append(lines, fmt.Sprintf("  Hash match: %s", entry.HashMatch))
	}

	if entry.SignalResolved != nil {
		resolved := "NO"
		if *entry.SignalResolved {
			resolved = "YES"
		}
		lines = append(lines, fmt.Sprintf("  Signal resolved: %s", resolved))
	}

	if entry.HealthChecks != nil {
		lines = append(lines, fmt.Sprintf("  Health: %s", FormatHealthChecks(entry.HealthChecks)))
	}

	if entry.MetricDeltas != nil {
		lines = append(lines, fmt.Sprintf("  Metrics: %s", FormatMetricDeltas(entry.MetricDeltas)))
	}

	return strings.Join(lines, "\n")
}

// FormatTier2Summary formats a single Tier2 compact summary entry for the prompt.
// 1:1 port of KA _format_tier2_entry().
func FormatTier2Summary(entry enrichment.Tier2Summary) string {
	completed := entry.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	workflow := withDefault(entry.ActionType, "unknown")
	outcome := withDefault(entry.Outcome, "unknown")

	var scoreText string
	if entry.AssessmentReason == "spec_drift" {
		scoreText = "INCONCLUSIVE (spec drift)"
	} else if entry.EffectivenessScore != nil {
		level := EffectivenessLevel(entry.EffectivenessScore)
		scoreText = fmt.Sprintf("%.2f (%s)", *entry.EffectivenessScore, level)
	} else {
		scoreText = "N/A"
	}

	hashMatch := withDefault(entry.HashMatch, "none")

	return fmt.Sprintf("- %s (%s): %s -> %s, effectiveness=%s, hashMatch=%s",
		entry.RemediationUID, completed, workflow, outcome, scoreText, hashMatch)
}

// FormatHealthChecks formats health check results into readable text.
// 1:1 port of KA _format_health_checks().
func FormatHealthChecks(hc *enrichment.HealthChecks) string {
	if hc == nil {
		return "N/A"
	}
	var parts []string
	if hc.PodRunning != nil {
		parts = append(parts, fmt.Sprintf("pod_running=%s", boolYesNo(*hc.PodRunning)))
	}
	if hc.ReadinessPass != nil {
		v := "fail"
		if *hc.ReadinessPass {
			v = "pass"
		}
		parts = append(parts, fmt.Sprintf("readiness=%s", v))
	}
	if hc.RestartDelta != nil {
		parts = append(parts, fmt.Sprintf("restart_delta=%d", *hc.RestartDelta))
	}
	if hc.CrashLoops != nil {
		parts = append(parts, fmt.Sprintf("crash_loops=%s", boolYesNo(*hc.CrashLoops)))
	}
	if hc.OomKilled != nil {
		parts = append(parts, fmt.Sprintf("oom_killed=%s", boolYesNo(*hc.OomKilled)))
	}
	if hc.PendingCount != nil && *hc.PendingCount > 0 {
		parts = append(parts, fmt.Sprintf("pending_pods=%d (scheduling/resource issue)", *hc.PendingCount))
	}
	if len(parts) == 0 {
		return "N/A"
	}
	return strings.Join(parts, ", ")
}

// FormatMetricDeltas formats metric deltas with before->after notation.
// 1:1 port of KA _format_metric_deltas().
func FormatMetricDeltas(md *enrichment.MetricDeltas) string {
	if md == nil {
		return "N/A"
	}
	var parts []string
	if md.CpuBefore != nil && md.CpuAfter != nil {
		parts = append(parts, fmt.Sprintf("cpu: %.2f -> %.2f", *md.CpuBefore, *md.CpuAfter))
	}
	if md.MemoryBefore != nil && md.MemoryAfter != nil {
		parts = append(parts, fmt.Sprintf("memory: %.1f -> %.1f", *md.MemoryBefore, *md.MemoryAfter))
	}
	if md.LatencyP95BeforeMs != nil && md.LatencyP95AfterMs != nil {
		parts = append(parts, fmt.Sprintf("latency_p95: %.1fms -> %.1fms", *md.LatencyP95BeforeMs, *md.LatencyP95AfterMs))
	}
	if md.ErrorRateBefore != nil && md.ErrorRateAfter != nil {
		parts = append(parts, fmt.Sprintf("error_rate: %.4f -> %.4f", *md.ErrorRateBefore, *md.ErrorRateAfter))
	}
	if len(parts) == 0 {
		return "N/A"
	}
	return strings.Join(parts, ", ")
}

// DetectDecliningEffectiveness groups Tier1 entries by actionType and checks
// for monotonically decreasing scores across >= 3 entries.
// 1:1 port of KA _detect_declining_effectiveness().
func DetectDecliningEffectiveness(chain []enrichment.Tier1Entry) []string {
	actionScores := make(map[string][]float64)
	for _, e := range chain {
		if e.AssessmentReason == "spec_drift" {
			continue
		}
		if e.ActionType != "" && e.EffectivenessScore != nil {
			actionScores[e.ActionType] = append(actionScores[e.ActionType], *e.EffectivenessScore)
		}
	}

	var declining []string
	for actionType, scores := range actionScores {
		if len(scores) < 3 {
			continue
		}
		isDeclining := true
		for i := 0; i < len(scores)-1; i++ {
			if scores[i] <= scores[i+1] {
				isDeclining = false
				break
			}
		}
		if isDeclining {
			declining = append(declining, actionType)
		}
	}
	return declining
}

// DetectCompletedButRecurring finds workflows that completed successfully
// multiple times for the same signal type across all tiers.
// 1:1 port of KA _detect_completed_but_recurring().
func DetectCompletedButRecurring(entries []HistoryEntry, threshold int) []RecurringPattern {
	completedOutcomes := map[string]bool{
		"completed": true, "success": true, "Completed": true, "Success": true,
	}

	type key struct{ actionType, signalType string }
	counts := make(map[key]int)

	for _, e := range entries {
		if e.AssessmentReason == "spec_drift" {
			continue
		}
		if !completedOutcomes[e.Outcome] {
			continue
		}
		if e.ActionType != "" && e.SignalType != "" {
			counts[key{e.ActionType, e.SignalType}]++
		}
	}

	var result []RecurringPattern
	for k, count := range counts {
		if count >= threshold {
			result = append(result, RecurringPattern{
				ActionType: k.actionType,
				Count:      count,
				SignalType: k.signalType,
			})
		}
	}
	return result
}

// AllZeroEffectiveness checks if all completed recurring entries for an
// action+signal combination have zero (or nil) effectiveness and unresolved signal.
// 1:1 port of KA _all_zero_effectiveness().
func AllZeroEffectiveness(entries []HistoryEntry, actionType, signalType string) bool {
	completedOutcomes := map[string]bool{
		"completed": true, "success": true, "Completed": true, "Success": true,
	}

	matched := false
	for _, e := range entries {
		if e.AssessmentReason == "spec_drift" {
			continue
		}
		if !completedOutcomes[e.Outcome] {
			continue
		}
		if e.ActionType != actionType || e.SignalType != signalType {
			continue
		}
		matched = true
		if e.EffectivenessScore != nil && *e.EffectivenessScore > 0 {
			return false
		}
		if e.SignalResolved != nil && *e.SignalResolved {
			return false
		}
	}
	return matched
}

// DetectSpecDriftCausalChains detects when a spec_drift entry's postRemediationSpecHash
// matches a subsequent entry's preRemediationSpecHash.
// 1:1 port of KA _detect_spec_drift_causal_chains().
func DetectSpecDriftCausalChains(chain []enrichment.Tier1Entry) map[string]string {
	preHashIndex := make(map[string]string)
	for _, e := range chain {
		if e.PreRemediationSpecHash != "" && e.RemediationUID != "" {
			preHashIndex[e.PreRemediationSpecHash] = e.RemediationUID
		}
	}

	causalMap := make(map[string]string)
	for _, e := range chain {
		if e.AssessmentReason != "spec_drift" {
			continue
		}
		if e.RemediationUID == "" || e.PostRemediationSpecHash == "" {
			continue
		}
		followupUID, ok := preHashIndex[e.PostRemediationSpecHash]
		if ok && followupUID != e.RemediationUID {
			causalMap[e.RemediationUID] = followupUID
		}
	}
	return causalMap
}

// EffectivenessLevel classifies a score into human-readable level.
// 1:1 port of KA effectiveness_level().
func EffectivenessLevel(score *float64) string {
	if score == nil {
		return "unknown"
	}
	if *score >= 0.7 {
		return "good"
	}
	if *score >= 0.4 {
		return "moderate"
	}
	return "poor"
}

func boolYesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// toHistoryEntries converts Tier1 and Tier2 entries into unified HistoryEntry slices
// for detection algorithms that operate across both tiers.
func toHistoryEntries(tier1 []enrichment.Tier1Entry, tier2 []enrichment.Tier2Summary) []HistoryEntry {
	entries := make([]HistoryEntry, 0, len(tier1)+len(tier2))
	for _, e := range tier1 {
		entries = append(entries, HistoryEntry{
			ActionType:         e.ActionType,
			SignalType:         e.SignalType,
			Outcome:            e.Outcome,
			EffectivenessScore: e.EffectivenessScore,
			SignalResolved:     e.SignalResolved,
			AssessmentReason:   e.AssessmentReason,
		})
	}
	for _, e := range tier2 {
		entries = append(entries, HistoryEntry{
			ActionType:         e.ActionType,
			SignalType:         e.SignalType,
			Outcome:            e.Outcome,
			EffectivenessScore: e.EffectivenessScore,
			SignalResolved:     e.SignalResolved,
			AssessmentReason:   e.AssessmentReason,
		})
	}
	return entries
}
