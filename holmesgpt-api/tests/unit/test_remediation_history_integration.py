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
Tests for remediation history integration into prompt builders.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: Remediation history section in incident and recovery analysis prompts.

Tests verify:
  - create_incident_investigation_prompt accepts remediation_history_context parameter
  - When context is provided, history section is included in the prompt
  - When context is None, prompt is unchanged (backward compatible)
  - _create_recovery_investigation_prompt accepts remediation_history_context parameter
  - _create_investigation_prompt accepts remediation_history_context parameter
"""

import pytest
from unittest.mock import MagicMock, patch

from extensions.incident.prompt_builder import create_incident_investigation_prompt
from extensions.recovery.prompt_builder import (
    _create_recovery_investigation_prompt,
    _create_investigation_prompt,
)
from clients.remediation_history_client import (
    query_remediation_history,
    create_remediation_history_api,
)


# Minimal valid request data for creating a prompt
MINIMAL_REQUEST = {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "resource_namespace": "production",
    "resource_kind": "Deployment",
    "resource_name": "memory-eater",
    "environment": "production",
    "error_message": "Container memory-eater exceeded limit",
}


class TestRemediationHistoryPromptIntegration:
    """UT-RH-INTEGRATION-001 through UT-RH-INTEGRATION-003: Prompt builder integration."""

    def test_backward_compatible_no_context(self):
        """UT-RH-INTEGRATION-001: Prompt is unchanged when no context provided (backward compat)."""
        prompt = create_incident_investigation_prompt(MINIMAL_REQUEST)
        assert "REMEDIATION HISTORY" not in prompt
        # Should still contain standard sections
        assert "Incident Analysis Request" in prompt

    def test_context_included_in_prompt(self):
        """UT-RH-INTEGRATION-002: History section is included when context provided."""
        context = {
            "targetResource": "production/Deployment/memory-eater",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-001",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "signalType": "alert",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.85,
                        "hashMatch": "postRemediation",
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }

        prompt = create_incident_investigation_prompt(
            MINIMAL_REQUEST, remediation_history_context=context
        )

        assert "REMEDIATION HISTORY" in prompt
        assert "rr-001" in prompt
        assert "restart" in prompt

    def test_regression_warning_in_prompt(self):
        """UT-RH-INTEGRATION-003: Regression warning appears in prompt when detected."""
        context = {
            "targetResource": "production/Deployment/memory-eater",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": True,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-reg",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "hashMatch": "preRemediation",
                        "effectivenessScore": 0.5,
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }

        prompt = create_incident_investigation_prompt(
            MINIMAL_REQUEST, remediation_history_context=context
        )

        assert "REGRESSION" in prompt.upper()


# Minimal recovery request data with previous execution context
MINIMAL_RECOVERY_REQUEST = {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "resource_namespace": "production",
    "resource_kind": "Deployment",
    "resource_name": "memory-eater",
    "environment": "production",
    "error_message": "Container memory-eater exceeded limit",
    "is_recovery_attempt": True,
    "recovery_attempt_number": 2,
    "previous_execution": {
        "original_rca": {
            "summary": "Memory limit exceeded due to leak",
            "signal_type": "OOMKilled",
            "severity": "critical",
            "contributing_factors": ["memory_leak"],
        },
        "selected_workflow": {
            "workflow_id": "restart-deployment-v1",
            "version": "1.0.0",
            "container_image": "kubernaut/restart:1.0",
            "rationale": "Restart to clear memory",
        },
        "failure": {
            "reason": "BackoffLimitExceeded",
            "message": "Job has reached the specified backoff limit",
            "failed_step_index": 1,
            "failed_step_name": "restart-pod",
        },
    },
}

# Shared remediation history context fixture for recovery tests
RECOVERY_HISTORY_CONTEXT = {
    "targetResource": "production/Deployment/memory-eater",
    "currentSpecHash": "sha256:abc123",
    "regressionDetected": False,
    "tier1": {
        "window": "24h0m0s",
        "chain": [
            {
                "remediationUID": "rr-prev-001",
                "completedAt": "2026-02-12T08:00:00Z",
                "signalType": "OOMKilled",
                "workflowType": "restart",
                "outcome": "success",
                "effectivenessScore": 0.4,
                "hashMatch": "none",
            }
        ],
    },
    "tier2": {"window": "2160h0m0s", "chain": []},
}


class TestRecoveryPromptRemediationHistoryIntegration:
    """UT-RH-INTEGRATION-004 through UT-RH-INTEGRATION-007: Recovery prompt builder integration."""

    def test_recovery_prompt_backward_compatible(self):
        """UT-RH-INTEGRATION-004: Recovery prompt unchanged when no context provided."""
        prompt = _create_recovery_investigation_prompt(MINIMAL_RECOVERY_REQUEST)
        assert "REMEDIATION HISTORY" not in prompt
        assert "Recovery Analysis Request" in prompt

    def test_recovery_prompt_includes_history(self):
        """UT-RH-INTEGRATION-005: Recovery prompt includes history when context provided."""
        prompt = _create_recovery_investigation_prompt(
            MINIMAL_RECOVERY_REQUEST,
            remediation_history_context=RECOVERY_HISTORY_CONTEXT,
        )

        assert "REMEDIATION HISTORY" in prompt
        assert "rr-prev-001" in prompt
        assert "restart" in prompt

    def test_investigation_prompt_backward_compatible(self):
        """UT-RH-INTEGRATION-006: Investigation prompt unchanged when no context provided."""
        prompt = _create_investigation_prompt(MINIMAL_REQUEST)
        assert "REMEDIATION HISTORY" not in prompt

    def test_investigation_prompt_includes_history(self):
        """UT-RH-INTEGRATION-007: Investigation prompt includes history when context provided."""
        prompt = _create_investigation_prompt(
            MINIMAL_REQUEST,
            remediation_history_context=RECOVERY_HISTORY_CONTEXT,
        )

        assert "REMEDIATION HISTORY" in prompt
        assert "rr-prev-001" in prompt


class TestRemediationHistoryWiring:
    """UT-RH-WIRING-001 through UT-RH-WIRING-005: End-to-end DS query and prompt enrichment wiring."""

    def test_fetch_remediation_history_queries_ds(self):
        """UT-RH-WIRING-001: fetch_remediation_history_for_request queries DS with correct args."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()
        mock_context = MagicMock()
        mock_context.to_dict.return_value = RECOVERY_HISTORY_CONTEXT
        mock_api.get_remediation_history_context.return_value = mock_context

        result = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="sha256:abc123",
        )

        assert result is not None
        assert result["targetResource"] == "production/Deployment/memory-eater"
        mock_api.get_remediation_history_context.assert_called_once_with(
            target_kind="Deployment",
            target_name="memory-eater",
            target_namespace="production",
            current_spec_hash="sha256:abc123",
        )

    def test_fetch_returns_none_when_api_none(self):
        """UT-RH-WIRING-002: fetch returns None when API not configured (graceful degradation)."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        result = fetch_remediation_history_for_request(
            api=None,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_fetch_returns_none_when_no_spec_hash(self):
        """UT-RH-WIRING-003: fetch returns None when spec hash is empty (no enrichment data)."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()

        result = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="",
        )

        assert result is None
        mock_api.get_remediation_history_context.assert_not_called()

    def test_fetch_returns_none_on_ds_error(self):
        """UT-RH-WIRING-004: fetch returns None on DS error (graceful degradation)."""
        from datastorage.exceptions import ServiceException
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()
        mock_api.get_remediation_history_context.side_effect = ServiceException(
            status=500, reason="Internal Server Error"
        )

        result = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_incident_prompt_enriched_via_wiring(self):
        """UT-RH-WIRING-005: Full wiring test - fetch context then enrich incident prompt."""
        from clients.remediation_history_client import fetch_remediation_history_for_request

        mock_api = MagicMock()
        mock_context = MagicMock()
        mock_context.to_dict.return_value = RECOVERY_HISTORY_CONTEXT
        mock_api.get_remediation_history_context.return_value = mock_context

        # Step 1: Fetch context (as analyze_incident would)
        context = fetch_remediation_history_for_request(
            api=mock_api,
            request_data=MINIMAL_REQUEST,
            current_spec_hash="sha256:abc123",
        )

        # Step 2: Build prompt with context (as analyze_incident would)
        prompt = create_incident_investigation_prompt(
            MINIMAL_REQUEST, remediation_history_context=context
        )

        # Verify end-to-end
        assert context is not None
        assert "REMEDIATION HISTORY" in prompt
        assert "rr-prev-001" in prompt
