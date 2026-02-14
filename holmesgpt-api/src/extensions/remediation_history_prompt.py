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


def build_remediation_history_section(context: Optional[Dict[str, Any]]) -> str:
    """Build the remediation history section for LLM prompt enrichment.

    Args:
        context: The remediation history context from DS endpoint, or None.
                 Expected structure matches the JSON response from
                 GET /api/v1/remediation-history/context.

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
            sections.append(_format_tier1_entry(entry))
            sections.append("")

    # Tier 2: Summary entries (wider history)
    if tier2_chain:
        window = context.get("tier2", {}).get("window", "2160h")
        sections.append(f"#### Historical Remediations (last {window})")
        sections.append("")

        for summary in tier2_chain:
            sections.append(_format_tier2_entry(summary))
            sections.append("")

    # Declining effectiveness trend detection
    if tier1_chain:
        declining = _detect_declining_effectiveness(tier1_chain)
        for workflow_type in declining:
            sections.append(
                f"**WARNING: DECLINING EFFECTIVENESS for '{workflow_type}' workflow** -- "
                "Each successive application is less effective, suggesting the workflow "
                "treats the symptom rather than the root cause. Consider a different approach."
            )
            sections.append("")

    # Reasoning guidance
    sections.append("**Reasoning Guidance:**")
    sections.append(
        "Use the above remediation history to inform your workflow selection. "
        "Avoid repeating workflows that previously failed or had poor effectiveness. "
        "If regression is detected, consider alternative approaches."
    )

    return "\n".join(sections)


def _format_tier1_entry(entry: Dict[str, Any]) -> str:
    """Format a single Tier 1 detailed entry for the prompt."""
    uid = entry.get("remediationUID", "unknown")
    completed = entry.get("completedAt", "unknown")
    outcome = entry.get("outcome", "unknown")
    workflow = entry.get("workflowType", "unknown")
    signal = entry.get("signalType", "")
    score = entry.get("effectivenessScore")
    hash_match = entry.get("hashMatch", "none")

    lines = [
        f"- **Remediation {uid}** ({completed})",
        f"  Workflow: {workflow} | Outcome: {outcome} | Signal: {signal}",
    ]

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
    """Format a single Tier 2 summary entry for the prompt."""
    uid = entry.get("remediationUID", "unknown")
    completed = entry.get("completedAt", "unknown")
    outcome = entry.get("outcome", "unknown")
    workflow = entry.get("workflowType", "unknown")
    score = entry.get("effectivenessScore")
    hash_match = entry.get("hashMatch", "none")

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
    workflow_scores: Dict[str, List[float]] = defaultdict(list)
    for entry in chain:
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
