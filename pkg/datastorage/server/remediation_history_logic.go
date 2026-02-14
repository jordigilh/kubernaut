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

// Pure correlation functions for remediation history context.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.1: Two-step query pattern with EM scoring infrastructure.
//
// These functions operate on data structures only (no I/O). They live in
// package server to access EM's exported types (EffectivenessComponents,
// ComputeWeightedScore, BuildEffectivenessResponse).
//
// Architecture:
//   - ComputeHashMatch: three-way hash comparison (DD-HAPI-016 v1.1)
//   - CorrelateTier1Chain: detailed entries joining RO + EM events (Tier 1)
//   - BuildTier2Summaries: summary entries for wider time window (Tier 2)
//   - DetectRegression: checks for preRemediation hash match (regression signal)
//   - mapHealthChecks/mapMetricDeltas/mapAlertResolution: typed sub-object mappers
package server

import (
	"sort"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ============================================================================
// HASH COMPARISON
// ============================================================================

// ComputeHashMatch performs the three-way hash comparison defined in DD-HAPI-016 v1.1.
// Compares the current resource spec hash against both pre and post remediation hashes.
//
// Priority: preRemediation (regression) > postRemediation > none.
// If current matches preHash, it indicates the resource has reverted to its
// pre-remediation state (regression). This takes priority even if postHash also matches.
func ComputeHashMatch(currentSpecHash, preHash, postHash string) api.RemediationHistoryEntryHashMatch {
	if currentSpecHash == preHash {
		return api.RemediationHistoryEntryHashMatchPreRemediation
	}
	if currentSpecHash == postHash {
		return api.RemediationHistoryEntryHashMatchPostRemediation
	}
	return api.RemediationHistoryEntryHashMatchNone
}

// toSummaryHashMatch converts an Entry-level hash match to the Summary-level equivalent.
// Both types have identical values but are distinct Go types in the ogen-generated code.
func toSummaryHashMatch(hm api.RemediationHistoryEntryHashMatch) api.OptRemediationHistorySummaryHashMatch {
	switch hm {
	case api.RemediationHistoryEntryHashMatchPreRemediation:
		return api.OptRemediationHistorySummaryHashMatch{
			Value: api.RemediationHistorySummaryHashMatchPreRemediation, Set: true,
		}
	case api.RemediationHistoryEntryHashMatchPostRemediation:
		return api.OptRemediationHistorySummaryHashMatch{
			Value: api.RemediationHistorySummaryHashMatchPostRemediation, Set: true,
		}
	default:
		return api.OptRemediationHistorySummaryHashMatch{
			Value: api.RemediationHistorySummaryHashMatchNone, Set: true,
		}
	}
}

// ============================================================================
// TYPED SUB-OBJECT MAPPERS
// Per ADR-EM-001 v1.3: EM emits component-level audit events with typed
// sub-objects (health_checks, metric_deltas, alert_resolution). These mappers
// convert from untyped map[string]interface{} (DB JSONB) to ogen types.
// ============================================================================

// mapHealthChecks extracts the health_checks typed sub-object from an EM
// effectiveness.health.assessed event's data.
//
// DD-HAPI-016 v1.1: health_checks sub-object provides pod_running, readiness_pass,
// restart_delta, crash_loops, oom_killed, pending_count. Fields that are absent
// in the event data remain unset (Opt types with Set=false), supporting
// partially populated assessments as the EM team enhances their assessors.
func mapHealthChecks(eventData map[string]interface{}) api.OptRemediationHealthChecks {
	hcMap, ok := eventData["health_checks"].(map[string]interface{})
	if !ok {
		return api.OptRemediationHealthChecks{}
	}

	hc := api.RemediationHealthChecks{}
	if v, ok := hcMap["pod_running"].(bool); ok {
		hc.PodRunning = api.OptBool{Value: v, Set: true}
	}
	if v, ok := hcMap["readiness_pass"].(bool); ok {
		hc.ReadinessPass = api.OptBool{Value: v, Set: true}
	}
	if v, ok := hcMap["restart_delta"].(float64); ok {
		hc.RestartDelta = api.OptInt{Value: int(v), Set: true}
	}
	if v, ok := hcMap["crash_loops"].(bool); ok {
		hc.CrashLoops = api.OptBool{Value: v, Set: true}
	}
	if v, ok := hcMap["oom_killed"].(bool); ok {
		hc.OomKilled = api.OptBool{Value: v, Set: true}
	}
	if v, ok := hcMap["pending_count"].(float64); ok {
		hc.PendingCount = api.OptInt{Value: int(v), Set: true}
	}
	return api.OptRemediationHealthChecks{Value: hc, Set: true}
}

// mapMetricDeltas extracts the metric_deltas typed sub-object from an EM
// effectiveness.metrics.assessed event's data.
//
// DD-HAPI-016 v1.1: metric_deltas provides before/after pairs for CPU, memory,
// latency p95, and error rate. Currently only CPU is populated by the EM team;
// remaining metrics fields are optional and absent until additional PromQL
// queries are implemented.
func mapMetricDeltas(eventData map[string]interface{}) api.OptRemediationMetricDeltas {
	mdMap, ok := eventData["metric_deltas"].(map[string]interface{})
	if !ok {
		return api.OptRemediationMetricDeltas{}
	}

	md := api.RemediationMetricDeltas{}
	if v, ok := mdMap["cpu_before"].(float64); ok {
		md.CpuBefore = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["cpu_after"].(float64); ok {
		md.CpuAfter = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["memory_before"].(float64); ok {
		md.MemoryBefore = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["memory_after"].(float64); ok {
		md.MemoryAfter = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["latency_p95_before_ms"].(float64); ok {
		md.LatencyP95BeforeMs = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["latency_p95_after_ms"].(float64); ok {
		md.LatencyP95AfterMs = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["error_rate_before"].(float64); ok {
		md.ErrorRateBefore = api.OptFloat64{Value: v, Set: true}
	}
	if v, ok := mdMap["error_rate_after"].(float64); ok {
		md.ErrorRateAfter = api.OptFloat64{Value: v, Set: true}
	}
	return api.OptRemediationMetricDeltas{Value: md, Set: true}
}

// mapAlertResolution extracts signalResolved from the alert_resolution typed
// sub-object on an EM effectiveness.alert.assessed event.
//
// DD-HAPI-016 v1.1: signalResolved is read directly from
// alert_resolution.alert_resolved (typed boolean). Returns unset if the
// alert was not assessed or alert_resolution is absent.
func mapAlertResolution(eventData map[string]interface{}) api.OptNilBool {
	arMap, ok := eventData["alert_resolution"].(map[string]interface{})
	if !ok {
		return api.OptNilBool{}
	}
	if resolved, ok := arMap["alert_resolved"].(bool); ok {
		return api.OptNilBool{Value: resolved, Set: true}
	}
	return api.OptNilBool{}
}

// ============================================================================
// SHARED EM CORRELATION HELPERS
// ============================================================================

// emCorrelationResult holds the shared data extracted from EM events for a
// single correlation_id. Used by both CorrelateTier1Chain and BuildTier2Summaries.
type emCorrelationResult struct {
	score          api.OptNilFloat64
	postHash       string
	signalResolved api.OptNilBool
}

// correlateEMEvents processes EM events for a given correlation_id, extracting
// the effectiveness score, post-remediation hash, and signal resolution status.
// These fields are common to both Tier 1 (detailed) and Tier 2 (summary) entries.
func correlateEMEvents(correlationID string, emEvts []*EffectivenessEvent) emCorrelationResult {
	var result emCorrelationResult

	// Use BuildEffectivenessResponse (DD-017 v2.1) for score + hash comparison
	resp := BuildEffectivenessResponse(correlationID, emEvts)

	if resp.Score != nil {
		result.score = api.OptNilFloat64{Value: *resp.Score, Set: true}
	}

	if resp.HashComparison.PostHash != "" {
		result.postHash = resp.HashComparison.PostHash
	}

	// Extract signalResolved from alert_resolution typed sub-object
	for _, evt := range emEvts {
		if evt.EventData == nil {
			continue
		}
		eventType, _ := evt.EventData["event_type"].(string)
		if eventType == "effectiveness.alert.assessed" {
			result.signalResolved = mapAlertResolution(evt.EventData)
			break
		}
	}

	return result
}

// ============================================================================
// TIER 1: DETAILED CORRELATION
// ============================================================================

// CorrelateTier1Chain joins RO events with EM component events to produce
// detailed RemediationHistoryEntry records for Tier 1.
//
// DD-HAPI-016 v1.1 Steps 1-3:
//  1. RO events provide the remediation chain skeleton (correlation_id, preHash, outcome, etc.)
//  2. EM events provide effectiveness scoring and typed sub-objects (health_checks, metric_deltas, alert_resolution)
//  3. Correlation by correlation_id, score computed via ComputeWeightedScore (DD-017 v2.1)
//
// Returns entries sorted by completedAt descending (most recent first).
func CorrelateTier1Chain(
	roEvents []repository.RawAuditRow,
	emEvents map[string][]*EffectivenessEvent,
	currentSpecHash string,
) []api.RemediationHistoryEntry {
	if len(roEvents) == 0 {
		return nil
	}

	entries := make([]api.RemediationHistoryEntry, 0, len(roEvents))

	for _, ro := range roEvents {
		entry := api.RemediationHistoryEntry{
			RemediationUID: ro.CorrelationID,
			CompletedAt:    ro.EventTimestamp,
		}

		// Extract RO event_data fields
		if v, ok := ro.EventData["signal_type"].(string); ok {
			entry.SignalType = api.OptString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["signal_fingerprint"].(string); ok {
			entry.SignalFingerprint = api.OptString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["workflow_type"].(string); ok {
			entry.WorkflowType = api.OptNilString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["outcome"].(string); ok {
			entry.Outcome = api.OptString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["pre_remediation_spec_hash"].(string); ok {
			entry.PreRemediationSpecHash = api.OptString{Value: v, Set: true}
		}

		// Look up EM events for this correlation_id
		preHash, _ := ro.EventData["pre_remediation_spec_hash"].(string)
		postHash := ""

		if emEvts, found := emEvents[ro.CorrelationID]; found && len(emEvts) > 0 {
			// Shared EM correlation (score, postHash, signalResolved)
			emResult := correlateEMEvents(ro.CorrelationID, emEvts)
			entry.EffectivenessScore = emResult.score
			entry.SignalResolved = emResult.signalResolved
			postHash = emResult.postHash

			if postHash != "" {
				entry.PostRemediationSpecHash = api.OptString{Value: postHash, Set: true}
			}

			// Tier 1-specific: extract typed sub-objects from EM event data
			for _, evt := range emEvts {
				if evt.EventData == nil {
					continue
				}
				eventType, _ := evt.EventData["event_type"].(string)

				switch eventType {
				case "effectiveness.health.assessed":
					entry.HealthChecks = mapHealthChecks(evt.EventData)
				case "effectiveness.metrics.assessed":
					entry.MetricDeltas = mapMetricDeltas(evt.EventData)
				}
			}
		}

		// Compute hash match (three-way comparison)
		hashMatch := ComputeHashMatch(currentSpecHash, preHash, postHash)
		entry.HashMatch = api.OptRemediationHistoryEntryHashMatch{Value: hashMatch, Set: true}

		entries = append(entries, entry)
	}

	// Sort by completedAt descending (most recent first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CompletedAt.After(entries[j].CompletedAt)
	})

	return entries
}

// ============================================================================
// TIER 2: SUMMARY CORRELATION
// ============================================================================

// BuildTier2Summaries creates summary-level entries for Tier 2 (wider time window).
// Summaries have fewer fields than Tier 1: no healthChecks, metricDeltas, or sideEffects.
//
// DD-HAPI-016 v1.1 Step 4: historical hash lookup for regression detection.
//
// Returns summaries sorted by completedAt descending (most recent first).
func BuildTier2Summaries(
	roEvents []repository.RawAuditRow,
	emEvents map[string][]*EffectivenessEvent,
	currentSpecHash string,
) []api.RemediationHistorySummary {
	if len(roEvents) == 0 {
		return nil
	}

	summaries := make([]api.RemediationHistorySummary, 0, len(roEvents))

	for _, ro := range roEvents {
		summary := api.RemediationHistorySummary{
			RemediationUID: ro.CorrelationID,
			CompletedAt:    ro.EventTimestamp,
		}

		// Extract RO event_data fields
		if v, ok := ro.EventData["signal_type"].(string); ok {
			summary.SignalType = api.OptString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["workflow_type"].(string); ok {
			summary.WorkflowType = api.OptNilString{Value: v, Set: true}
		}
		if v, ok := ro.EventData["outcome"].(string); ok {
			summary.Outcome = api.OptString{Value: v, Set: true}
		}

		// Look up EM events for score and hash comparison
		preHash, _ := ro.EventData["pre_remediation_spec_hash"].(string)
		postHash := ""

		if emEvts, found := emEvents[ro.CorrelationID]; found && len(emEvts) > 0 {
			// Shared EM correlation (score, postHash, signalResolved)
			emResult := correlateEMEvents(ro.CorrelationID, emEvts)
			summary.EffectivenessScore = emResult.score
			summary.SignalResolved = emResult.signalResolved
			postHash = emResult.postHash
		}

		// Compute hash match (using summary-specific type)
		hashMatch := ComputeHashMatch(currentSpecHash, preHash, postHash)
		summary.HashMatch = toSummaryHashMatch(hashMatch)

		summaries = append(summaries, summary)
	}

	// Sort by completedAt descending (most recent first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CompletedAt.After(summaries[j].CompletedAt)
	})

	return summaries
}

// ============================================================================
// REGRESSION DETECTION
// ============================================================================

// DetectRegression checks whether any Tier 1 entry has a hashMatch of preRemediation,
// indicating the current resource spec has reverted to a pre-remediation state.
//
// DD-HAPI-016 v1.1: regression = currentSpecHash matches a previous preRemediationSpecHash.
func DetectRegression(entries []api.RemediationHistoryEntry) bool {
	for _, entry := range entries {
		if entry.HashMatch.Set && entry.HashMatch.Value == api.RemediationHistoryEntryHashMatchPreRemediation {
			return true
		}
	}
	return false
}
