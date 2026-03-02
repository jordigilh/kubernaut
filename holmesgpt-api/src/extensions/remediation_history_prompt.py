# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Remediation history prompt section builder.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: Two-step query pattern with EM scoring infrastructure.
ADR-EM-001 v1.3: Component-level audit events with typed sub-objects.

This module formats the remediation history context returned by the DataStorage
endpoint into a human-readable prompt section for the LLM. The formatted text
provides the LLM with actionable context about past remediations to inform
workflow selection and avoid repeating failed approaches.
"""

from typing import Any, Dict, List, Optional


def build_remediation_history_section(
    context: Optional[Dict[str, Any]],
    escalation_threshold: Optional[int] = None,
) -> str:
    """Build the remediation history section for LLM prompt enrichment.

    Args:
        context: The remediation history context from DS endpoint, or None.
                 Expected structure matches the JSON response from
                 GET /api/v1/remediation-history/context.
        escalation_threshold: Number of completed-but-recurring remediations
                 before warning the LLM to escalate. Defaults to
                 REPEATED_REMEDIATION_ESCALATION_THRESHOLD from constants.

    Returns:
        Formatted prompt section string, or empty string if no history.
    """
    if context is None:
        return ""

    tier1_chain = context.get("tier1", {}).get("chain", [])
    tier2_chain = context.get("tier2", {}).get("chain", [])

    if not tier1_chain and not tier2_chain:
        return ""

    sections: List[str] = []

    # DD-EM-002 v1.1: Detect causal chains between spec_drift entries and follow-ups
    causal_chains = _detect_spec_drift_causal_chains(tier1_chain) if tier1_chain else {}

    # Header
    target = context.get("targetResource", "unknown")
    sections.append(f"### REMEDIATION HISTORY for {target}")
    sections.append("")

    # Regression warning
    if context.get("regressionDetected", False):
        sections.append("**WARNING: CONFIGURATION REGRESSION DETECTED**")
        sections.append(
            "The current resource spec matches a pre-remediation state (preRemediation hash match). "
            "This indicates the resource has reverted to a previously remediated configuration. "
            "Consider a different remediation approach."
        )
        sections.append("")

    # Tier 1: Detailed entries (recent history)
    if tier1_chain:
        window = context.get("tier1", {}).get("window", "24h")
        sections.append(f"#### Recent Remediations (last {window})")
        sections.append("")

        for entry in tier1_chain:
            sections.append(_format_tier1_entry(entry, causal_chains))
            sections.append("")

    # Tier 2: Summary entries (wider history)
    if tier2_chain:
        window = context.get("tier2", {}).get("window", "2160h")
        sections.append(f"#### Historical Remediations (last {window})")
        sections.append("")

        for summary in tier2_chain:
            sections.append(_format_tier2_entry(summary))
            sections.append("")

    # Declining effectiveness trend detection (excludes spec_drift entries)
    if tier1_chain:
        declining = _detect_declining_effectiveness(tier1_chain)
        for workflow_type in declining:
            sections.append(
                f"**WARNING: DECLINING EFFECTIVENESS for '{workflow_type}' workflow** -- "
                "Each successive application is less effective, suggesting the workflow "
                "treats the symptom rather than the root cause. Consider a different approach."
            )
            sections.append("")

    # Issue #224: Completed-but-recurring detection across all tiers
    all_entries = tier1_chain + tier2_chain
    if escalation_threshold is None:
        from config.constants import REPEATED_REMEDIATION_ESCALATION_THRESHOLD
        escalation_threshold = REPEATED_REMEDIATION_ESCALATION_THRESHOLD

    recurring = _detect_completed_but_recurring(all_entries, threshold=escalation_threshold)
    for workflow_type, count, signal_type in recurring:
        sections.append(
            f"**WARNING: REPEATED INEFFECTIVE REMEDIATION for '{workflow_type}'** -- "
            f"Completed {count} times for signal '{signal_type}' but the issue continues "
            "to recur. This suggests the workflow treats the symptom, not the root cause. "
            "Recommend selecting `needs_human_review` or an alternative escalation workflow."
        )
        sections.append("")

    # Reasoning guidance
    sections.append("**Reasoning Guidance:**")
    sections.append(
        "Use the above remediation history to inform your workflow selection. "
        "Avoid repeating workflows that previously failed or had poor effectiveness. "
        "If a workflow completed successfully multiple times but the same signal keeps "
        "recurring, escalate to human review -- the workflow is not addressing the root cause. "
        "If regression is detected, consider alternative approaches."
    )

    # DD-EM-002 v1.1: Spec drift awareness guidance
    has_spec_drift = any(
        e.get("assessmentReason") == "spec_drift" for e in all_entries
    )
    if has_spec_drift:
        sections.append("")
        sections.append(
            "**Note on spec drift entries:** Some remediation assessments were inconclusive "
            "because the target resource spec was modified during the assessment window. Do not "
            "treat these as failed remediations. Investigate what modified the spec -- it may "
            "be the root cause or a contributing factor."
        )

    return "\n".join(sections)


def _format_tier1_entry(
    entry: Dict[str, Any],
    causal_chains: Optional[Dict[str, str]] = None,
) -> str:
    """Format a single Tier 1 detailed entry for the prompt.

    DD-EM-002 v1.1: When assessmentReason == "spec_drift", the entry is rendered
    as INCONCLUSIVE with suppressed health/metrics data. Two variants exist:
    - Default: "may still be viable under different conditions"
    - Causal chain: "led to follow-up remediation (UID)" when hash chain detected
    """
    uid = entry.get("remediationUID", "unknown")
    completed = entry.get("completedAt", "unknown")
    outcome = entry.get("outcome", "unknown")
    workflow = entry.get("workflowType", "unknown")
    signal = entry.get("signalType", "")
    assessment_reason = entry.get("assessmentReason")

    lines = [
        f"- **Remediation {uid}** ({completed})",
        f"  Workflow: {workflow} | Outcome: {outcome} | Signal: {signal}",
    ]

    # DD-EM-002 v1.1: Spec drift semantic rewrite
    if assessment_reason == "spec_drift":
        causal_chains = causal_chains or {}
        followup_uid = causal_chains.get(uid)

        if followup_uid:
            # Causal chain variant: this spec_drift led to a follow-up remediation
            lines.append(
                f"  **Assessment: INCONCLUSIVE (spec drift -- led to follow-up remediation)** -- "
                f"The target resource spec changed after this remediation, and a subsequent "
                f"remediation ({followup_uid}) was triggered from the resulting state. This suggests "
                f"the outcome was unstable, but the workflow may still work under different "
                f"conditions. Use with caution."
            )
        else:
            # Default variant: no causal chain detected
            lines.append(
                "  **Assessment: INCONCLUSIVE (spec drift)** -- The target resource spec was "
                "modified by an external actor during the assessment window, invalidating "
                "effectiveness data. This workflow may still be viable under different "
                "conditions. The spec change could indicate a competing controller, GitOps "
                "sync, or manual edit that is itself relevant to the root cause."
            )
        # Do NOT show health checks, metric deltas, signalResolved, or score
        # (they are unreliable when spec drift occurred)
        return "\n".join(lines)

    # Normal entry formatting (non-spec_drift)
    score = entry.get("effectivenessScore")
    hash_match = entry.get("hashMatch", "none")

    # Effectiveness score
    level = effectiveness_level(score)
    if score is not None:
        lines.append(f"  Effectiveness: {score:.2f} ({level})")

    # Hash match
    if hash_match and hash_match != "none":
        lines.append(f"  Hash match: {hash_match}")

    # Signal resolved
    signal_resolved = entry.get("signalResolved")
    if signal_resolved is not None:
        resolved_text = "YES" if signal_resolved else "NO"
        lines.append(f"  Signal resolved: {resolved_text}")

    # Health checks
    hc = entry.get("healthChecks")
    if hc:
        lines.append(f"  Health: {_format_health_checks(hc)}")

    # Metric deltas
    md = entry.get("metricDeltas")
    if md:
        lines.append(f"  Metrics: {_format_metric_deltas(md)}")

    return "\n".join(lines)


def _format_tier2_entry(entry: Dict[str, Any]) -> str:
    """Format a single Tier 2 summary entry for the prompt.

    DD-EM-002 v1.1: When assessmentReason == "spec_drift", show INCONCLUSIVE
    instead of the unreliable 0.0 score.
    """
    uid = entry.get("remediationUID", "unknown")
    completed = entry.get("completedAt", "unknown")
    outcome = entry.get("outcome", "unknown")
    workflow = entry.get("workflowType", "unknown")
    score = entry.get("effectivenessScore")
    hash_match = entry.get("hashMatch", "none")
    assessment_reason = entry.get("assessmentReason")

    if assessment_reason == "spec_drift":
        score_text = "INCONCLUSIVE (spec drift)"
    else:
        level = effectiveness_level(score)
        score_text = f"{score:.2f} ({level})" if score is not None else "N/A"

    return (
        f"- {uid} ({completed}): {workflow} -> {outcome}, "
        f"effectiveness={score_text}, hashMatch={hash_match}"
    )


def _format_health_checks(hc: Dict[str, Any]) -> str:
    """Format health checks from typed sub-object into readable text."""
    parts = []

    pod_running = hc.get("podRunning")
    if pod_running is not None:
        parts.append(f"pod_running={'yes' if pod_running else 'no'}")

    readiness = hc.get("readinessPass")
    if readiness is not None:
        parts.append(f"readiness={'pass' if readiness else 'fail'}")

    restart_delta = hc.get("restartDelta")
    if restart_delta is not None:
        parts.append(f"restart_delta={restart_delta}")

    crash_loops = hc.get("crashLoops")
    if crash_loops is not None:
        parts.append(f"crash_loops={'yes' if crash_loops else 'no'}")

    oom_killed = hc.get("oomKilled")
    if oom_killed is not None:
        parts.append(f"oom_killed={'yes' if oom_killed else 'no'}")

    pending = hc.get("pendingCount")
    if pending is not None and pending > 0:
        parts.append(f"pending_pods={pending} (scheduling/resource issue)")

    return ", ".join(parts) if parts else "N/A"


def _format_metric_deltas(md: Dict[str, Any]) -> str:
    """Format metric deltas from typed sub-object with before->after notation."""
    parts = []

    cpu_before = md.get("cpuBefore")
    cpu_after = md.get("cpuAfter")
    if cpu_before is not None and cpu_after is not None:
        parts.append(f"cpu: {cpu_before:.2f} -> {cpu_after:.2f}")

    mem_before = md.get("memoryBefore")
    mem_after = md.get("memoryAfter")
    if mem_before is not None and mem_after is not None:
        parts.append(f"memory: {mem_before:.1f} -> {mem_after:.1f}")

    lat_before = md.get("latencyP95BeforeMs")
    lat_after = md.get("latencyP95AfterMs")
    if lat_before is not None and lat_after is not None:
        parts.append(f"latency_p95: {lat_before:.1f}ms -> {lat_after:.1f}ms")

    err_before = md.get("errorRateBefore")
    err_after = md.get("errorRateAfter")
    if err_before is not None and err_after is not None:
        parts.append(f"error_rate: {err_before:.4f} -> {err_after:.4f}")

    return ", ".join(parts) if parts else "N/A"


def _detect_declining_effectiveness(chain: List[Dict[str, Any]]) -> List[str]:
    """Detect workflow types with declining effectiveness scores.

    Groups entries by workflowType and checks if scores are monotonically
    decreasing for groups with >= 3 entries. Returns the workflow types
    exhibiting a declining trend.

    Args:
        chain: List of tier 1 remediation history entries.

    Returns:
        List of workflow type names with declining effectiveness.
    """
    from collections import defaultdict

    # Group scores by workflow type, preserving order
    # DD-EM-002 v1.1: Exclude spec_drift entries -- their 0.0 scores would
    # create false declining trends since the score is unreliable.
    workflow_scores: Dict[str, List[float]] = defaultdict(list)
    for entry in chain:
        if entry.get("assessmentReason") == "spec_drift":
            continue  # Skip unreliable spec_drift entries
        wf_type = entry.get("workflowType")
        score = entry.get("effectivenessScore")
        if wf_type and score is not None:
            workflow_scores[wf_type].append(score)

    declining: List[str] = []
    for wf_type, scores in workflow_scores.items():
        if len(scores) >= 3:
            # Check if scores are strictly declining
            is_declining = all(
                scores[i] > scores[i + 1] for i in range(len(scores) - 1)
            )
            if is_declining:
                declining.append(wf_type)

    return declining


def _detect_completed_but_recurring(
    chain: List[Dict[str, Any]],
    threshold: int = 2,
) -> List[tuple]:
    """Detect workflows that completed successfully multiple times for the same signal.

    Issue #224: When a workflow completes successfully but the same signal recurs,
    the LLM should escalate to human review instead of selecting the same workflow.

    Args:
        chain: Combined tier1+tier2 remediation history entries.
        threshold: Minimum number of completed entries to trigger detection.

    Returns:
        List of (workflow_type, count, signal_type) tuples for recurring patterns.
    """
    from collections import defaultdict

    COMPLETED_OUTCOMES = {"completed", "success", "Completed", "Success"}

    counts: Dict[tuple, int] = defaultdict(int)
    for entry in chain:
        if entry.get("assessmentReason") == "spec_drift":
            continue
        outcome = entry.get("outcome", "")
        if outcome not in COMPLETED_OUTCOMES:
            continue
        wf_type = entry.get("workflowType", "")
        signal = entry.get("signalType", "")
        if wf_type and signal:
            counts[(wf_type, signal)] += 1

    result = []
    for (wf_type, signal), count in counts.items():
        if count >= threshold:
            result.append((wf_type, count, signal))

    return result


def _detect_spec_drift_causal_chains(chain: List[Dict[str, Any]]) -> Dict[str, str]:
    """Detect when a spec_drift entry's postRemediationSpecHash matches
    a subsequent entry's preRemediationSpecHash, proving the spec_drift
    entry led to a follow-up remediation.

    DD-EM-002 v1.1: Causal chain detection for prompt semantic rewriting.

    Args:
        chain: List of tier 1 remediation history entries.

    Returns:
        Dict mapping spec_drift entry's remediationUID -> follow-up remediationUID.
    """
    causal_map: Dict[str, str] = {}

    # Build index of preRemediationSpecHash -> remediationUID for all entries
    pre_hash_index: Dict[str, str] = {}
    for entry in chain:
        pre_hash = entry.get("preRemediationSpecHash", "")
        uid = entry.get("remediationUID", "")
        if pre_hash and uid:
            pre_hash_index[pre_hash] = uid

    # For each spec_drift entry, check if its postHash matches any entry's preHash
    for entry in chain:
        if entry.get("assessmentReason") != "spec_drift":
            continue
        drift_uid = entry.get("remediationUID", "")
        post_hash = entry.get("postRemediationSpecHash", "")
        if not drift_uid or not post_hash:
            continue

        followup_uid = pre_hash_index.get(post_hash, "")
        if followup_uid and followup_uid != drift_uid:
            causal_map[drift_uid] = followup_uid

    return causal_map


def effectiveness_level(score: Optional[float]) -> str:
    """Classify effectiveness score into human-readable level.

    Args:
        score: Effectiveness score (0.0-1.0), or None.

    Returns:
        "good" (>=0.7), "moderate" (>=0.4), "poor" (<0.4), or "unknown" (None).
    """
    if score is None:
        return "unknown"
    if score >= 0.7:
        return "good"
    if score >= 0.4:
        return "moderate"
    return "poor"
