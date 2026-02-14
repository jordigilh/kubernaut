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
Tests for remediation history prompt section builder.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: Prompt formatting of remediation history context.

Tests cover:
  - Empty/None context handling
  - Tier 1 detailed entry formatting (effectiveness, health, metrics, alert resolution)
  - Tier 2 summary entry formatting
  - Regression warning text
  - Effectiveness score classification
  - Health check formatting with pendingCount
  - Metric delta formatting with before/after notation
  - Alert resolution formatting
  - Declining effectiveness trend detection
  - Partially populated data graceful handling
"""

import pytest

from extensions.remediation_history_prompt import (
    build_remediation_history_section,
    effectiveness_level,
)


class TestBuildRemediationHistorySection:
    """UT-RH-PROMPT-001 through UT-RH-PROMPT-006: Prompt section builder."""

    def test_none_context_returns_empty(self):
        """UT-RH-PROMPT-001: None context returns empty string (no history available)."""
        result = build_remediation_history_section(None)
        assert result == ""

    def test_empty_tier1_chain_returns_empty(self):
        """UT-RH-PROMPT-002: Empty tier1 chain returns empty string."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {"window": "24h0m0s", "chain": []},
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)
        assert result == ""

    def test_tier1_entry_formatted(self):
        """UT-RH-PROMPT-003: Tier 1 entry with full data is formatted as structured text."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-001",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "signalType": "alert",
                        "signalFingerprint": "fp-001",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.85,
                        "hashMatch": "postRemediation",
                        "signalResolved": True,
                        "healthChecks": {
                            "podRunning": True,
                            "readinessPass": True,
                            "restartDelta": 0,
                            "crashLoops": False,
                            "oomKilled": False,
                            "pendingCount": 0,
                        },
                        "metricDeltas": {
                            "cpuBefore": 0.8,
                            "cpuAfter": 0.3,
                            "memoryBefore": 512.0,
                            "memoryAfter": 256.0,
                        },
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        # Should contain section header
        assert "REMEDIATION HISTORY" in result
        # Should contain entry details
        assert "rr-001" in result
        assert "restart" in result
        assert "success" in result
        # Should contain effectiveness level
        assert "good" in result.lower() or "0.85" in result
        # Should contain health check info
        assert "pod_running" in result.lower() or "podRunning" in result or "running" in result.lower()
        # Should contain metric deltas
        assert "cpu" in result.lower()
        # Should contain signal resolved
        assert "resolved" in result.lower()

    def test_regression_warning_included(self):
        """UT-RH-PROMPT-004: Regression detected includes warning text."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": True,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-reg",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "signalType": "alert",
                        "workflowType": "restart",
                        "outcome": "success",
                        "hashMatch": "preRemediation",
                        "effectivenessScore": 0.5,
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "REGRESSION" in result.upper()
        assert "preRemediation" in result or "pre-remediation" in result.lower()

    def test_tier2_summary_formatted(self):
        """UT-RH-PROMPT-005: Tier 2 summary entries are formatted."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": True,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-t1",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "signalType": "alert",
                        "workflowType": "restart",
                        "outcome": "success",
                        "hashMatch": "preRemediation",
                        "effectivenessScore": 0.5,
                    }
                ],
            },
            "tier2": {
                "window": "2160h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-t2-001",
                        "completedAt": "2026-01-15T08:00:00Z",
                        "signalType": "alert",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.9,
                        "hashMatch": "postRemediation",
                    },
                    {
                        "remediationUID": "rr-t2-002",
                        "completedAt": "2026-01-10T12:00:00Z",
                        "signalType": "alert",
                        "workflowType": "scale-up",
                        "outcome": "failure",
                        "effectivenessScore": 0.2,
                        "hashMatch": "none",
                    },
                ],
            },
        }
        result = build_remediation_history_section(context)

        # Tier 2 section should be present
        assert "historical" in result.lower() or "tier 2" in result.lower() or "wider" in result.lower()
        assert "rr-t2-001" in result
        assert "rr-t2-002" in result

    def test_partial_data_graceful(self):
        """UT-RH-PROMPT-006: Partial data (missing health/metrics) is handled gracefully."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-partial",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "outcome": "success",
                        # No healthChecks, no metricDeltas, no effectivenessScore
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "rr-partial" in result
        assert "success" in result
        # Should not crash or contain "None" as raw text
        assert "None" not in result


class TestEffectivenessLevel:
    """UT-RH-PROMPT-007 through UT-RH-PROMPT-010: Effectiveness level classification."""

    def test_good_level(self):
        """UT-RH-PROMPT-007: Score >= 0.7 is classified as 'good'."""
        assert effectiveness_level(0.7) == "good"
        assert effectiveness_level(0.85) == "good"
        assert effectiveness_level(1.0) == "good"

    def test_moderate_level(self):
        """UT-RH-PROMPT-008: Score >= 0.4 and < 0.7 is classified as 'moderate'."""
        assert effectiveness_level(0.4) == "moderate"
        assert effectiveness_level(0.55) == "moderate"
        assert effectiveness_level(0.69) == "moderate"

    def test_poor_level(self):
        """UT-RH-PROMPT-009: Score < 0.4 is classified as 'poor'."""
        assert effectiveness_level(0.0) == "poor"
        assert effectiveness_level(0.2) == "poor"
        assert effectiveness_level(0.39) == "poor"

    def test_none_score(self):
        """UT-RH-PROMPT-010: None score returns 'unknown'."""
        assert effectiveness_level(None) == "unknown"


class TestHealthCheckFormatting:
    """UT-RH-PROMPT-011: Health check formatting with pendingCount."""

    def test_pending_count_surfaced(self):
        """UT-RH-PROMPT-011: Non-zero pendingCount surfaced as scheduling indicator."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-pending",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "outcome": "success",
                        "effectivenessScore": 0.6,
                        "healthChecks": {
                            "podRunning": True,
                            "readinessPass": False,
                            "restartDelta": 2,
                            "pendingCount": 3,
                        },
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        # pendingCount should appear in the output
        assert "pending" in result.lower()
        assert "3" in result


class TestMetricDeltaFormatting:
    """UT-RH-PROMPT-012: Metric delta formatting with before/after notation."""

    def test_metric_deltas_formatted(self):
        """UT-RH-PROMPT-012: Metric deltas show before->after with direction."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-metrics",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "outcome": "success",
                        "effectivenessScore": 0.9,
                        "metricDeltas": {
                            "cpuBefore": 0.8,
                            "cpuAfter": 0.3,
                            "memoryBefore": 512.0,
                            "memoryAfter": 256.0,
                            "latencyP95BeforeMs": 200.0,
                            "latencyP95AfterMs": 50.0,
                        },
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        # Should contain CPU metric info
        assert "cpu" in result.lower()
        assert "0.8" in result or "0.80" in result
        assert "0.3" in result or "0.30" in result
        # Should contain memory metric info
        assert "memory" in result.lower()


class TestDecliningEffectiveness:
    """UT-RH-PROMPT-013 through UT-RH-PROMPT-015: Declining effectiveness trend detection."""

    def test_declining_same_workflow_detected(self):
        """UT-RH-PROMPT-013: Three entries with same workflowType and declining scores triggers warning.

        Scenario: Memory leak in payment-api causes repeated OOMKilled events.
        Same 'restart' workflow applied 3 times with decreasing effectiveness:
        - 1st restart (0.80): Pod recovers, memory drops to normal, alert resolves.
        - 2nd restart (0.60): Pod recovers but memory climbs back faster, restart_delta accumulates.
        - 3rd restart (0.30): Pod barely recovers, readiness fails, enters CrashLoopBackOff.
        The declining trend signals the workflow treats the symptom (crash) not the root cause (leak).
        """
        context = {
            "targetResource": "production/Deployment/payment-api",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-decline-1",
                        "completedAt": "2026-02-12T06:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.80,
                    },
                    {
                        "remediationUID": "rr-decline-2",
                        "completedAt": "2026-02-12T12:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.60,
                    },
                    {
                        "remediationUID": "rr-decline-3",
                        "completedAt": "2026-02-12T16:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.30,
                    },
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "DECLINING" in result.upper()
        assert "restart" in result.lower()

    def test_no_decline_when_different_workflows(self):
        """UT-RH-PROMPT-014: Different workflow types with declining scores -> no decline warning."""
        context = {
            "targetResource": "production/Deployment/payment-api",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-mix-1",
                        "completedAt": "2026-02-12T06:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.80,
                    },
                    {
                        "remediationUID": "rr-mix-2",
                        "completedAt": "2026-02-12T12:00:00Z",
                        "workflowType": "scale-up",
                        "outcome": "success",
                        "effectivenessScore": 0.60,
                    },
                    {
                        "remediationUID": "rr-mix-3",
                        "completedAt": "2026-02-12T16:00:00Z",
                        "workflowType": "increase-memory",
                        "outcome": "success",
                        "effectivenessScore": 0.30,
                    },
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "DECLINING" not in result.upper()

    def test_no_decline_with_only_two_entries(self):
        """UT-RH-PROMPT-015: Only 2 entries for same workflow -> no decline warning (need >= 3)."""
        context = {
            "targetResource": "production/Deployment/payment-api",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-two-1",
                        "completedAt": "2026-02-12T06:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.80,
                    },
                    {
                        "remediationUID": "rr-two-2",
                        "completedAt": "2026-02-12T12:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.30,
                    },
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "DECLINING" not in result.upper()


class TestSignalResolvedFormatting:
    """UT-RH-PROMPT-016: signalResolved=False displays as 'NO'."""

    def test_signal_resolved_false_shows_no(self):
        """UT-RH-PROMPT-016: signalResolved: False -> output contains 'Signal resolved: NO'."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-unresolved",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.3,
                        "signalResolved": False,
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "Signal resolved: NO" in result


class TestPartialHealthChecks:
    """UT-RH-PROMPT-017: Partial healthChecks with null fields renders cleanly."""

    def test_partial_health_no_none_literal(self):
        """UT-RH-PROMPT-017: Only podRunning set, others null -> no 'None' literal in output."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-partial-hc",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.7,
                        "healthChecks": {
                            "podRunning": True,
                            "readinessPass": None,
                            "restartDelta": None,
                            "crashLoops": None,
                            "oomKilled": None,
                            "pendingCount": None,
                        },
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        assert "rr-partial-hc" in result
        assert "pod_running=yes" in result
        assert "None" not in result


class TestMetricDeltaArrowNotation:
    """UT-RH-PROMPT-018: Metric deltas use -> arrow notation."""

    def test_arrow_notation_in_metric_output(self):
        """UT-RH-PROMPT-018: Metric deltas formatted with '->' between before and after values."""
        context = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-arrow",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.9,
                        "metricDeltas": {
                            "cpuBefore": 0.50,
                            "cpuAfter": 0.30,
                        },
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        result = build_remediation_history_section(context)

        # Assert arrow notation is used
        assert "->" in result
        assert "0.50" in result
        assert "0.30" in result
