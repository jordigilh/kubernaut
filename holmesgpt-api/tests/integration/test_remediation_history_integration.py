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
Integration tests for remediation history client, query, and prompt wiring.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: Two-step query with EM scoring, spec_drift support.
DD-EM-002 v1.1: spec_drift assessment reason.

Test Plan: docs/testing/DD-HAPI-016/TEST_PLAN.md (IT-HAPI-016-001 through IT-HAPI-016-008)

Pattern: Direct function calls with env patching and mock pool manager.
Same approach as test_llm_prompt_business_logic.py — no external infrastructure
needed; tests exercise real business logic with mocked external dependencies.
"""

import os
import pytest
from unittest.mock import MagicMock, patch, PropertyMock


# ============================================================================
# Shared fixtures
# ============================================================================

SPEC_DRIFT_CONTEXT = {
    "targetResource": "production/Deployment/payment-api",
    "currentSpecHash": "sha256:current_abc",
    "regressionDetected": False,
    "tier1": {
        "window": "24h0m0s",
        "chain": [
            {
                "remediationUID": "rr-drift-001",
                "completedAt": "2026-02-12T08:00:00Z",
                "signalType": "HighCPULoad",
                "workflowType": "ScaleUp",
                "outcome": "success",
                "effectivenessScore": 0.0,
                "assessmentReason": "spec_drift",
                "hashMatch": "none",
                "preRemediationSpecHash": "sha256:pre_drift",
                "postRemediationSpecHash": "sha256:post_drift",
            },
        ],
    },
    "tier2": {"window": "2160h0m0s", "chain": []},
}

FULL_CONTEXT = {
    "targetResource": "production/Deployment/payment-api",
    "currentSpecHash": "sha256:current_abc",
    "regressionDetected": False,
    "tier1": {
        "window": "24h0m0s",
        "chain": [
            {
                "remediationUID": "rr-full-001",
                "completedAt": "2026-02-12T10:00:00Z",
                "signalType": "HighCPULoad",
                "workflowType": "ScaleUp",
                "outcome": "success",
                "effectivenessScore": 0.85,
                "assessmentReason": "full",
                "hashMatch": "postRemediation",
                "healthChecks": {
                    "podRunning": True,
                    "readinessPass": True,
                },
                "metricDeltas": {
                    "cpuBefore": 0.85,
                    "cpuAfter": 0.45,
                },
            },
        ],
    },
    "tier2": {"window": "2160h0m0s", "chain": []},
}

MINIMAL_REQUEST = {
    "signal_type": "HighCPULoad",
    "severity": "critical",
    "resource_namespace": "production",
    "resource_kind": "Deployment",
    "resource_name": "payment-api",
    "environment": "production",
    "error_message": "CPU usage at 95%",
}

MINIMAL_RECOVERY_REQUEST = {
    "signal_type": "HighCPULoad",
    "severity": "critical",
    "resource_namespace": "production",
    "resource_kind": "Deployment",
    "resource_name": "payment-api",
    "environment": "production",
    "error_message": "CPU usage at 95%",
    "is_recovery_attempt": True,
    "recovery_attempt_number": 2,
    "previous_execution": {
        "original_rca": {
            "summary": "CPU spike due to load",
            "signal_type": "HighCPULoad",
            "severity": "critical",
            "contributing_factors": ["high_traffic"],
        },
        "selected_workflow": {
            "workflow_id": "scale-up-v1",
            "version": "1.0.0",
            "container_image": "kubernaut/scale-up:1.0",
            "rationale": "Scale up replicas to handle load",
        },
        "failure": {
            "reason": "BackoffLimitExceeded",
            "message": "Job has reached the specified backoff limit",
            "failed_step_index": 0,
            "failed_step_name": "scale-replicas",
        },
    },
}


# ============================================================================
# 3.1 Client Factory Tests
# ============================================================================


class TestClientFactory:
    """IT-HAPI-016-001 through IT-HAPI-016-003: create_remediation_history_api."""

    def test_creates_api_with_env_url_and_pool_manager(self, monkeypatch):
        """IT-HAPI-016-001: create_remediation_history_api with DATA_STORAGE_URL + mocked pool manager."""
        monkeypatch.setenv("DATA_STORAGE_URL", "http://127.0.0.1:18098")
        monkeypatch.setenv("DATA_STORAGE_TIMEOUT", "30")

        mock_pool = MagicMock()

        with patch(
            "clients.remediation_history_client.get_shared_datastorage_pool_manager",
            return_value=mock_pool,
            create=True,
        ):
            # Need to reimport to pick up env changes
            from importlib import reload
            import clients.remediation_history_client as client_mod
            reload(client_mod)

            result = client_mod.create_remediation_history_api()

        assert result is not None
        # Verify it's a RemediationHistoryAPIApi instance
        assert hasattr(result, "get_remediation_history_context")

    def test_returns_none_without_ds_url(self, monkeypatch):
        """IT-HAPI-016-002: create_remediation_history_api returns None when no DS URL."""
        monkeypatch.delenv("DATA_STORAGE_URL", raising=False)

        from clients.remediation_history_client import create_remediation_history_api

        result = create_remediation_history_api(app_config={})

        assert result is None

    def test_returns_none_on_pool_manager_error(self, monkeypatch):
        """IT-HAPI-016-003: create_remediation_history_api returns None when pool manager fails."""
        monkeypatch.setenv("DATA_STORAGE_URL", "http://127.0.0.1:18098")

        with patch(
            "clients.remediation_history_client.get_shared_datastorage_pool_manager",
            side_effect=ImportError("No module named 'datastorage_pool_manager'"),
            create=True,
        ):
            from importlib import reload
            import clients.remediation_history_client as client_mod
            reload(client_mod)

            result = client_mod.create_remediation_history_api()

        assert result is None


# ============================================================================
# 3.2 Client Query Tests
# ============================================================================


class TestClientQuery:
    """IT-HAPI-016-004 through IT-HAPI-016-005: query_remediation_history."""

    def test_spec_drift_preserved_through_client(self):
        """IT-HAPI-016-004: query_remediation_history preserves assessmentReason=spec_drift."""
        from clients.remediation_history_client import query_remediation_history

        mock_api = MagicMock()
        mock_context = MagicMock()
        mock_context.to_dict.return_value = SPEC_DRIFT_CONTEXT
        mock_api.get_remediation_history_context.return_value = mock_context

        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="payment-api",
            target_namespace="production",
            current_spec_hash="sha256:current_abc",
        )

        assert result is not None
        entry = result["tier1"]["chain"][0]
        assert entry["assessmentReason"] == "spec_drift"
        assert entry["effectivenessScore"] == 0.0
        assert entry["preRemediationSpecHash"] == "sha256:pre_drift"
        assert entry["postRemediationSpecHash"] == "sha256:post_drift"

    def test_connection_error_returns_none(self):
        """IT-HAPI-016-005: fetch_remediation_history_for_request with ConnectionError returns None."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()
        mock_api.get_remediation_history_context.side_effect = ConnectionError(
            "Connection refused"
        )

        result = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="sha256:current_abc",
        )

        assert result is None


# ============================================================================
# 3.3 Prompt Wiring Tests
# ============================================================================


class TestPromptWiring:
    """IT-HAPI-016-006 through IT-HAPI-016-008: Prompt enrichment with spec_drift."""

    def test_incident_prompt_with_spec_drift_context(self):
        """IT-HAPI-016-006: Incident prompt includes INCONCLUSIVE for spec_drift entries."""
        from extensions.incident.prompt_builder import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            MINIMAL_REQUEST,
            remediation_history_context=SPEC_DRIFT_CONTEXT,
        )

        # Verify spec_drift semantics appear in the prompt
        prompt_upper = prompt.upper()
        assert "REMEDIATION HISTORY" in prompt_upper
        assert "INCONCLUSIVE" in prompt_upper, (
            "spec_drift entries should be marked INCONCLUSIVE in the LLM prompt"
        )
        assert "SPEC DRIFT" in prompt_upper, (
            "spec_drift reason should be visible to the LLM"
        )
        # The unreliable 0.0 score should NOT appear as a real effectiveness value
        assert "0.00 (poor)" not in prompt.lower(), (
            "spec_drift score 0.0 should not be presented as 'poor' effectiveness"
        )

    def test_recovery_prompt_with_remediation_history(self):
        """IT-HAPI-016-007: Recovery prompt includes remediation history section."""
        from extensions.recovery.prompt_builder import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(
            MINIMAL_RECOVERY_REQUEST,
            remediation_history_context=FULL_CONTEXT,
        )

        prompt_upper = prompt.upper()
        assert "REMEDIATION HISTORY" in prompt_upper
        assert "rr-full-001" in prompt
        assert "ScaleUp" in prompt or "scaleup" in prompt.lower()

    def test_fetch_skips_query_without_spec_hash(self):
        """IT-HAPI-016-008: fetch_remediation_history_for_request returns None when spec_hash empty."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()

        result = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="",
        )

        assert result is None
        mock_api.get_remediation_history_context.assert_not_called()
