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
	"slices"
	"strings"
	"text/template"
	"time"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// notAvailable is the placeholder rendered in prompt text when a value is
// absent, inconclusive, or not populated for the given history entry.
const notAvailable = "N/A"

// completedOutcomes defines which RR outcome values indicate a remediation that
// "completed" from the orchestrator's perspective. Used only by qualitative
// recurring detection; assessment validity does not derive from these outcomes.
// Issue #722: Added "Remediated" and "Inconclusive" (CRD-level outcomes).
var completedOutcomes = map[string]bool{
	"completed": true, "success": true, "Completed": true, "Success": true,
	"Remediated": true, "Inconclusive": true,
}

// HistoryEntry is a unified interface for detection algorithms that operate on
// both Tier1 and Tier2 entries (recurring detection, all-zero-effectiveness).
type HistoryEntry struct {
	WorkflowID         string
	WorkflowName       string
	ActionType         string
	SignalType         string
	Outcome            string
	EffectivenessScore *float64
	SignalResolved     *bool
	HashMatch          string
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
	TargetResource                 string
	RegressionDetected             bool
	ChronologicalEntries           []string
	Tier1Count                     int
	Tier1Window                    string
	Tier2Count                     int
	Tier2Window                    string
	DecliningEffectivenessWarnings []string
	RecurringRemediationWarnings   []string
	KnownIneffectiveWarnings       []string
	RecurrenceDurabilityGuidance   []string
	HasSpecDrift                   bool
}

// BuildRemediationHistorySection renders the full remediation history prompt section.
// Recurrence is inferred from the linked history itself; callers must not
// provide a retry-count threshold that changes the prompt's interpretation.
func BuildRemediationHistorySection(result *enrichment.RemediationHistoryResult) string {
	if result == nil {
		return ""
	}
	if len(result.Tier1) == 0 && len(result.Tier2) == 0 {
		return ""
	}

	causalChains := DetectSpecDriftCausalChains(result.Tier1)
	allEntries := toHistoryEntries(result.Tier1, result.Tier2)

	data := remediationHistoryTemplateData{
		TargetResource:                 sanitizeField(result.TargetResource),
		RegressionDetected:             result.RegressionDetected,
		ChronologicalEntries:           renderChronologicalHistory(result, causalChains),
		Tier1Count:                     len(result.Tier1),
		Tier1Window:                    result.Tier1Window,
		Tier2Count:                     len(result.Tier2),
		Tier2Window:                    result.Tier2Window,
		DecliningEffectivenessWarnings: buildDecliningEffectivenessWarnings(result.Tier1),
		RecurringRemediationWarnings:   buildRecurringRemediationWarnings(allEntries),
		KnownIneffectiveWarnings:       buildKnownIneffectiveWarnings(allEntries),
		RecurrenceDurabilityGuidance:   buildRecurrenceDurabilityGuidance(allEntries),
		HasSpecDrift:                   anyAssessmentReasonSpecDrift(allEntries),
	}

	return renderRemediationHistoryTemplate(data)
}

// BuildRCAHistoryReviewSection renders target-scoped history for the
// server-assisted RCA continuation. It intentionally excludes workflow
// selection instructions because the RCA loop has not reached discovery yet.
// BR-KA-016 / DD-KA-2511: prior remediation history is evidence, not background.
// BuildRCAProvisionalReviewSection renders the accepted provisional RCA as
// server-owned context for a history-aware continuation. Free-form model output
// is sanitized before it is placed in the next user message (DD-KA-2511,
// FedRAMP SI-10, OWASP ASVS V2.2.x).
//
//nolint:gocyclo // each optional RCA field has an independent sanitized rendering contract
func BuildRCAProvisionalReviewSection(result *katypes.InvestigationResult) string {
	if result == nil {
		return ""
	}

	lines := []string{
		"## PROVISIONAL RCA",
		"This is the server-preserved provisional RCA from the preceding submit_result. Re-evaluate it using the current target-scoped evidence; do not treat it as final authority.",
	}
	if result.RCASummary != "" {
		lines = append(lines, "Summary: "+sanitizeField(result.RCASummary))
	}
	if result.SignalName != "" {
		lines = append(lines, "Signal: "+sanitizeField(result.SignalName))
	}
	if result.Severity != "" {
		lines = append(lines, "Severity: "+sanitizeField(result.Severity))
	}
	if result.RemediationTarget.Kind != "" || result.RemediationTarget.Name != "" {
		target := sanitizeField(result.RemediationTarget.Kind) + "/" + sanitizeField(result.RemediationTarget.Name)
		if result.RemediationTarget.Namespace != "" {
			target += " (namespace: " + sanitizeField(result.RemediationTarget.Namespace) + ")"
		}
		if result.RemediationTarget.APIVersion != "" {
			target += " (api_version: " + sanitizeField(result.RemediationTarget.APIVersion) + ")"
		}
		lines = append(lines, "Target: "+target)
	}
	if len(result.ContributingFactors) > 0 {
		factors := make([]string, len(result.ContributingFactors))
		for i, factor := range result.ContributingFactors {
			factors[i] = sanitizeField(factor)
		}
		lines = append(lines, "Contributing factors: "+strings.Join(factors, "; "))
	}
	if len(result.CausalChain) > 0 {
		chain := make([]string, len(result.CausalChain))
		for i, step := range result.CausalChain {
			chain[i] = sanitizeField(step)
		}
		lines = append(lines, "Causal chain: "+strings.Join(chain, " -> "))
	}
	if result.CausalMechanismStatus != "" {
		lines = append(lines, "Causal mechanism status: "+sanitizeField(result.CausalMechanismStatus))
	}
	if result.ConfidenceSet {
		lines = append(lines, fmt.Sprintf("Confidence: %.4f", result.Confidence))
	} else {
		lines = append(lines, "Confidence: unknown")
	}
	if result.InvestigationOutcome != "" {
		lines = append(lines, "Investigation outcome: "+sanitizeField(result.InvestigationOutcome))
	}
	return strings.Join(lines, "\n")
}

func BuildRCAHistoryReviewSection(result *enrichment.RemediationHistoryResult) string {
	if result == nil || len(result.Tier1) == 0 && len(result.Tier2) == 0 {
		return ""
	}

	lines := []string{
		"## PRIOR REMEDIATION HISTORY REVIEW",
		fmt.Sprintf("Target: %s", withDefault(sanitizeField(result.TargetResource), unknownHistoryValue)),
		"Prior remediation history is evidence, not background. Do not restate the prior RCA solely because the same symptom remains.",
		"Re-evaluate the causal mechanism using current observations, prior application/effectiveness evidence, contradictory evidence, and the current target state.",
		"Explicitly classify the deeper causal mechanism as verified, probable, or unknown. Do not infer a mechanism from a symptom or an effectiveness score alone.",
		"If the RCA target changes, continue investigating the newly identified target; the server will provide target-scoped history for it.",
		"Recorded prior attempts:",
	}
	if result.CurrentSpecHash != "" {
		lines = append(lines, "Current target spec hash: "+sanitizeField(result.CurrentSpecHash))
	}
	lines = append(lines, renderChronologicalHistory(result, DetectSpecDriftCausalChains(result.Tier1))...)
	return strings.Join(lines, "\n")
}

type chronologicalHistoryEntry struct {
	recordedAt time.Time
	uid        string
	rendered   string
}

// DD-KA-2496: numbering is global across tiers; source slices are never sorted
// in-place. UID and rendered detail provide a deterministic tie-break.
func renderChronologicalHistory(result *enrichment.RemediationHistoryResult, causalChains map[string]string) []string {
	entries := make([]chronologicalHistoryEntry, 0, len(result.Tier1))
	for _, e := range result.Tier1 {
		entries = append(entries, chronologicalHistoryEntry{e.CompletedAt, e.RemediationUID, FormatTier1Entry(e, causalChains)})
	}
	entries = slices.Grow(entries, len(result.Tier2))
	for _, e := range result.Tier2 {
		entries = append(entries, chronologicalHistoryEntry{e.CompletedAt, e.RemediationUID, FormatTier2Summary(e)})
	}
	slices.SortStableFunc(entries, func(a, b chronologicalHistoryEntry) int {
		if cmp := a.recordedAt.Compare(b.recordedAt); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.uid, b.uid); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.rendered, b.rendered)
	})
	rendered := make([]string, 0, len(entries))
	for i, e := range entries {
		rendered = append(rendered, fmt.Sprintf("- **Attempt %d** — %s", i+1, strings.TrimPrefix(e.rendered, "- ")))
	}
	return rendered
}

// buildDecliningEffectivenessWarnings builds one warning string per
// workflow exhibiting monotonically declining effectiveness (KA
// _detect_declining_effectiveness()).
func buildDecliningEffectivenessWarnings(tier1 []enrichment.Tier1Entry) []string {
	declining := DetectDecliningEffectiveness(tier1)
	warnings := make([]string, 0, len(declining))
	for _, actionType := range declining {
		warnings = append(warnings, fmt.Sprintf(
			"**WARNING: DECLINING EFFECTIVENESS for '%s' workflow** -- "+
				"Each successive application is less effective, suggesting the workflow "+
				"treats the symptom rather than the root cause. Consider a different approach.",
			sanitizeField(actionType),
		))
	}
	return warnings
}

// Recurrence is qualitative evidence, never an action-family ban (DD-KA-2496).
func buildRecurringRemediationWarnings(allEntries []HistoryEntry) []string {
	patterns := DetectCompletedButRecurring(allEntries)
	warnings := make([]string, 0, len(patterns))
	for _, r := range patterns {
		warnings = append(warnings, fmt.Sprintf(
			"**WARNING: RECURRING REMEDIATION for action '%s'** -- "+
				"Completed %d times for signal '%s' but the issue recurred. "+
				"Compare individual workflow IDs and current evidence; shared action/name is not workflow identity. "+
				"If no catalog workflow is suitable, set `investigation_outcome` to `inconclusive` and omit `selected_workflow`.",
			sanitizeField(r.ActionType), r.Count, sanitizeField(r.SignalType),
		))
	}
	return warnings
}

// A single assessed failure is relevant. Zero alone does not explain its cause.
func buildKnownIneffectiveWarnings(entries []HistoryEntry) []string {
	warnings := make([]string, 0, len(entries))
	for _, e := range entries {
		if !AllZeroEffectiveness([]HistoryEntry{e}, e.ActionType, e.SignalType) {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"**KNOWN INEFFECTIVE ATTEMPT:** Workflow ID: %s | Workflow name: %s had known zero effectiveness and an explicitly unresolved signal in a completed valid assessment. "+
				"The failure cause is unknown from that score alone. Weigh current evidence and explain the prior outcome's relevance before repeating the same workflow ID; all candidates remain available.",
			withDefault(sanitizeField(e.WorkflowID), unknownHistoryValue), withDefault(sanitizeField(e.WorkflowName), unknownHistoryValue)))
	}
	return warnings
}

// buildRecurrenceDurabilityGuidance renders qualitative evidence for the LLM.
// A successful/Remediated outcome and a resolved signal describe what happened
// during the effectiveness-assessment window; neither proves that the root
// cause was durably removed. Repeated linked entries are evidence to weigh with
// the RCA and spec-hash chain, not a numeric escalation rule.
func buildRecurrenceDurabilityGuidance(entries []HistoryEntry) []string {
	patterns := DetectCompletedButRecurring(entries)
	guidance := make([]string, 0, len(patterns)+1)

	for _, entry := range entries {
		if isRemediatedOutcome(entry.Outcome) || (entry.SignalResolved != nil && *entry.SignalResolved) {
			guidance = append(guidance,
				"**DURABILITY CONTEXT:** A `Remediated` outcome or a resolved signal records symptom resolution during the effectiveness-assessment window; it does not by itself prove durable root-cause resolution. Use the RCA, recurrence evidence, and linked pre/post spec hashes to assess durability.",
			)
			break
		}
	}

	for _, pattern := range patterns {
		guidance = append(guidance, fmt.Sprintf(
			"**RECURRENCE CONTEXT:** Linked history shows completed `%s` remediations for signal `%s` on multiple observations (%d recorded entries). Treat this as qualitative recurrence evidence; do not apply an arbitrary retry-count threshold or infer durability from a single successful or `Remediated` assessment.",
			sanitizeField(pattern.ActionType), sanitizeField(pattern.SignalType), pattern.Count,
		))
	}

	return guidance
}

func isRemediatedOutcome(outcome string) bool {
	return strings.EqualFold(strings.TrimSpace(outcome), "remediated")
}

// anyAssessmentReasonSpecDrift reports whether any entry was assessed as
// spec-drift-inconclusive.
func anyAssessmentReasonSpecDrift(entries []HistoryEntry) bool {
	for _, e := range entries {
		if e.AssessmentReason == eav1.AssessmentReasonSpecDrift {
			return true
		}
	}
	return false
}

// renderRemediationHistoryTemplate parses and executes the
// remediation_history.tmpl template, returning an inline error marker string
// (rather than an error) on failure so prompt rendering never aborts.
func renderRemediationHistoryTemplate(data remediationHistoryTemplateData) string {
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
func appendSpecDriftAssessment(lines []string, entry enrichment.Tier1Entry, causalChains map[string]string) []string {
	if entry.AssessmentReason != eav1.AssessmentReasonSpecDrift {
		return lines
	}
	if causalChains != nil {
		if followupUID, ok := causalChains[entry.RemediationUID]; ok {
			return append(lines, fmt.Sprintf(
				"  **Assessment: INCONCLUSIVE (spec drift -- led to follow-up remediation)** -- "+
					"The target resource spec changed after this remediation, and a subsequent "+
					"remediation (%s) was triggered from the resulting state. This suggests "+
					"the outcome was unstable, but the workflow may still work under different "+
					"conditions. Use with caution.",
				sanitizeField(followupUID),
			))
		}
	}
	return append(lines,
		"  **Assessment: INCONCLUSIVE (spec drift)** -- The target resource spec was "+
			"modified by an external actor during the assessment window, invalidating "+
			"effectiveness data. This workflow may still be viable under different "+
			"conditions. The spec change could indicate a competing controller, GitOps "+
			"sync, or manual edit that is itself relevant to the root cause.",
	)
}

func FormatTier1Entry(entry enrichment.Tier1Entry, causalChains map[string]string) string {
	recorded := formatHistoryRecordedTime(entry.CompletedAt)
	action := withDefault(sanitizeField(entry.ActionType), unknownHistoryValue)
	outcome := withDefault(sanitizeField(entry.Outcome), unknownHistoryValue)
	signal := sanitizeField(entry.SignalType)
	remediationUID := sanitizeField(entry.RemediationUID)

	lines := []string{
		fmt.Sprintf("- **Remediation %s** (Recorded at: %s)", remediationUID, recorded),
		"  " + formatHistorySelection(entry.WorkflowID, entry.WorkflowName, entry.WorkflowSelectionConfidence),
		fmt.Sprintf("  Action type: %s | Outcome: %s | Signal: %s", action, outcome, signal),
		fmt.Sprintf("  Assessment: %s", withDefault(sanitizeField(entry.AssessmentReason), "unknown/pending")),
	}

	lines = appendSpecDriftAssessment(lines, entry, causalChains)

	if entry.EffectivenessScore != nil {
		level := EffectivenessLevel(entry.EffectivenessScore)
		lines = append(lines, fmt.Sprintf("  Effectiveness: %.2f (%s)", *entry.EffectivenessScore, level))
	}

	if entry.HashMatch != "" && entry.HashMatch != "none" {
		lines = append(lines, fmt.Sprintf("  Hash match: %s", sanitizeField(entry.HashMatch)))
	}
	if entry.AppliedState != "" {
		lines = append(lines, fmt.Sprintf("  Applied state: %s", sanitizeField(entry.AppliedState)))
	}
	if entry.SpecChangeStatus != "" {
		lines = append(lines, fmt.Sprintf("  Spec change status: %s", sanitizeField(entry.SpecChangeStatus)))
	}
	if entry.PreRemediationSpecHash != "" || entry.PostRemediationSpecHash != "" {
		lines = append(lines, fmt.Sprintf("  Spec hashes: pre=%s | post=%s",
			withDefault(sanitizeField(entry.PreRemediationSpecHash), unknownHistoryValue),
			withDefault(sanitizeField(entry.PostRemediationSpecHash), unknownHistoryValue)))
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
	recorded := formatHistoryRecordedTime(entry.CompletedAt)
	action := withDefault(sanitizeField(entry.ActionType), unknownHistoryValue)
	outcome := withDefault(sanitizeField(entry.Outcome), unknownHistoryValue)

	var scoreText string
	switch {
	case entry.AssessmentReason == eav1.AssessmentReasonSpecDrift:
		scoreText = "INCONCLUSIVE (spec drift)"
	case entry.EffectivenessScore != nil:
		level := EffectivenessLevel(entry.EffectivenessScore)
		scoreText = fmt.Sprintf("%.2f (%s)", *entry.EffectivenessScore, level)
	default:
		scoreText = notAvailable
	}

	hashMatch := withDefault(sanitizeField(entry.HashMatch), "none")

	resolved := unknownHistoryValue
	if entry.SignalResolved != nil {
		resolved = boolYesNo(*entry.SignalResolved)
	}
	return fmt.Sprintf("- %s (Recorded at: %s): %s | Action type: %s -> %s, effectiveness=%s, signal_resolved=%s, assessment=%s, hashMatch=%s, applied_state=%s, spec_change_status=%s, pre_hash=%s, post_hash=%s",
		sanitizeField(entry.RemediationUID), recorded, formatHistorySelection(entry.WorkflowID, entry.WorkflowName, entry.WorkflowSelectionConfidence),
		action, outcome, scoreText, resolved, withDefault(sanitizeField(entry.AssessmentReason), "unknown/pending"), hashMatch,
		withDefault(sanitizeField(entry.AppliedState), unknownHistoryValue),
		withDefault(sanitizeField(entry.SpecChangeStatus), unknownHistoryValue),
		withDefault(sanitizeField(entry.PreRemediationSpecHash), unknownHistoryValue),
		withDefault(sanitizeField(entry.PostRemediationSpecHash), unknownHistoryValue))
}

const unknownHistoryValue = "unknown"

func formatHistoryRecordedTime(recorded time.Time) string {
	if recorded.IsZero() {
		return unknownHistoryValue
	}
	return recorded.UTC().Format(time.RFC3339)
}

func formatHistorySelection(id, name string, confidence *float64) string {
	score := unknownHistoryValue
	if confidence != nil {
		score = fmt.Sprintf("%.2f", *confidence)
	}
	return fmt.Sprintf("Workflow ID: %s | Workflow name: %s | workflow_selection_confidence: %s",
		withDefault(sanitizeField(id), unknownHistoryValue), withDefault(sanitizeField(name), unknownHistoryValue), score)
}

// FormatHealthChecks formats health check results into readable text.
// 1:1 port of KA _format_health_checks().
func FormatHealthChecks(hc *enrichment.HealthChecks) string {
	if hc == nil {
		return notAvailable
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
		return notAvailable
	}
	return strings.Join(parts, ", ")
}

// FormatMetricDeltas formats metric deltas with before->after notation.
// 1:1 port of KA _format_metric_deltas().
func FormatMetricDeltas(md *enrichment.MetricDeltas) string {
	if md == nil {
		return notAvailable
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
	parts = append(parts, formatClusterScopedAndThroughputDeltas(md)...)
	if len(parts) == 0 {
		return notAvailable
	}
	return strings.Join(parts, ", ")
}

// formatClusterScopedAndThroughputDeltas renders the throughput backfill and
// the 6 cluster-scoped Node/PersistentVolume metric_deltas fields (Issue
// #193, DD-EM-005 v1.1) as human-readable before->after pairs for the LLM
// prompt. Extracted from FormatMetricDeltas to keep that function within the
// project's line-length convention -- pure code motion alongside the new
// field mappings, no behavior change to the existing fields formatted above.
func formatClusterScopedAndThroughputDeltas(md *enrichment.MetricDeltas) []string {
	var parts []string
	if md.ThroughputBeforeRps != nil && md.ThroughputAfterRps != nil {
		parts = append(parts, fmt.Sprintf("throughput: %.1f -> %.1f rps", *md.ThroughputBeforeRps, *md.ThroughputAfterRps))
	}
	if md.NodeNotReadyBefore != nil && md.NodeNotReadyAfter != nil {
		parts = append(parts, fmt.Sprintf("node_not_ready: %.0f -> %.0f", *md.NodeNotReadyBefore, *md.NodeNotReadyAfter))
	}
	if md.NodeMemoryPressureBefore != nil && md.NodeMemoryPressureAfter != nil {
		parts = append(parts, fmt.Sprintf("node_memory_pressure: %.0f -> %.0f", *md.NodeMemoryPressureBefore, *md.NodeMemoryPressureAfter))
	}
	if md.NodeDiskPressureBefore != nil && md.NodeDiskPressureAfter != nil {
		parts = append(parts, fmt.Sprintf("node_disk_pressure: %.0f -> %.0f", *md.NodeDiskPressureBefore, *md.NodeDiskPressureAfter))
	}
	if md.PvPhaseFailedBefore != nil && md.PvPhaseFailedAfter != nil {
		parts = append(parts, fmt.Sprintf("pv_phase_failed: %.0f -> %.0f", *md.PvPhaseFailedBefore, *md.PvPhaseFailedAfter))
	}
	if md.PvPhasePendingBefore != nil && md.PvPhasePendingAfter != nil {
		parts = append(parts, fmt.Sprintf("pv_phase_pending: %.0f -> %.0f", *md.PvPhasePendingBefore, *md.PvPhasePendingAfter))
	}
	if md.PvUsageRatioBefore != nil && md.PvUsageRatioAfter != nil {
		parts = append(parts, fmt.Sprintf("pv_usage_ratio: %.2f -> %.2f", *md.PvUsageRatioBefore, *md.PvUsageRatioAfter))
	}
	return parts
}

// DetectDecliningEffectiveness groups Tier1 entries by actionType and checks
// for monotonically decreasing scores across >= 3 entries.
// 1:1 port of KA _detect_declining_effectiveness().
func DetectDecliningEffectiveness(chain []enrichment.Tier1Entry) []string {
	chronological := slices.Clone(chain)
	slices.SortStableFunc(chronological, func(a, b enrichment.Tier1Entry) int {
		return a.CompletedAt.Compare(b.CompletedAt)
	})
	actionScores := make(map[string][]float64, len(chain))
	for _, e := range chronological {
		if e.AssessmentReason == eav1.AssessmentReasonSpecDrift {
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
	slices.Sort(declining)
	return declining
}

// DetectCompletedButRecurring finds workflows that completed successfully
// multiple times for the same signal type across all tiers. Repetition is a
// property of the linked history; it is intentionally not controlled by a
// prompt retry threshold.
func DetectCompletedButRecurring(entries []HistoryEntry) []RecurringPattern {

	type key struct{ actionType, signalType string }
	counts := make(map[key]int)

	for _, e := range entries {
		if e.AssessmentReason == eav1.AssessmentReasonSpecDrift {
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
		if count > 1 {
			result = append(result, RecurringPattern{
				ActionType: k.actionType,
				Count:      count,
				SignalType: k.signalType,
			})
		}
	}
	slices.SortFunc(result, func(a, b RecurringPattern) int {
		if a.ActionType != b.ActionType {
			return strings.Compare(a.ActionType, b.ActionType)
		}
		return strings.Compare(a.SignalType, b.SignalType)
	})
	return result
}

// AllZeroEffectiveness never treats absent/pending assessment data as zero.
// No production selection prohibition is derived from this predicate.
func AllZeroEffectiveness(entries []HistoryEntry, actionType, signalType string) bool {

	matched := false
	for _, e := range entries {
		if e.AssessmentReason == eav1.AssessmentReasonSpecDrift {
			continue
		}
		if e.ActionType != actionType || e.SignalType != signalType {
			continue
		}
		matched = true
		if !enrichment.IsCompletedValidAssessment(e.AssessmentReason, e.EffectivenessScore) ||
			*e.EffectivenessScore != 0 || e.SignalResolved == nil || *e.SignalResolved {
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
		if e.AssessmentReason != eav1.AssessmentReasonSpecDrift {
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
	entries := make([]HistoryEntry, 0, len(tier1))
	for _, e := range tier1 {
		entries = append(entries, HistoryEntry{
			WorkflowID:         e.WorkflowID,
			WorkflowName:       e.WorkflowName,
			ActionType:         e.ActionType,
			SignalType:         e.SignalType,
			Outcome:            e.Outcome,
			EffectivenessScore: e.EffectivenessScore,
			SignalResolved:     e.SignalResolved,
			HashMatch:          e.HashMatch,
			AssessmentReason:   e.AssessmentReason,
		})
	}
	entries = slices.Grow(entries, len(tier2))
	for _, e := range tier2 {
		entries = append(entries, HistoryEntry{
			WorkflowID:         e.WorkflowID,
			WorkflowName:       e.WorkflowName,
			ActionType:         e.ActionType,
			SignalType:         e.SignalType,
			Outcome:            e.Outcome,
			EffectivenessScore: e.EffectivenessScore,
			SignalResolved:     e.SignalResolved,
			HashMatch:          e.HashMatch,
			AssessmentReason:   e.AssessmentReason,
		})
	}
	return entries
}
