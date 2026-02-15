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
Tests for remediation history DS client wrapper using generated OpenAPI client.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: HAPI queries DS for remediation history context.
DD-AUTH-014: ServiceAccount authentication for DataStorage access.

Tests cover:
  - Successful query returns context dict (via generated RemediationHistoryAPIApi)
  - DS unavailable (connection error) returns None (graceful degradation)
  - DS returns error (5xx ServiceException) returns None with logged warning
  - API not configured (None) returns None
  - Request timeout returns None
  - Response is converted to dict with camelCase keys (alias serialization)
"""

import pytest
from unittest.mock import MagicMock, patch

from datastorage.exceptions import ApiException, ServiceException

from src.clients.remediation_history_client import query_remediation_history


class TestQueryRemediationHistory:
    """UT-RH-CLIENT-001 through UT-RH-CLIENT-006: DS client wrapper with generated OpenAPI client."""

    def test_successful_query_returns_dict(self):
        """UT-RH-CLIENT-001: Successful query returns context dict via generated client."""
        # Arrange: mock RemediationHistoryAPIApi
        mock_api = MagicMock()
        mock_context = MagicMock()
        mock_context.to_dict.return_value = {
            "targetResource": "default/Deployment/nginx",
            "currentSpecHash": "sha256:abc123",
            "regressionDetected": False,
            "tier1": {"window": "24h0m0s", "chain": []},
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        mock_api.get_remediation_history_context.return_value = mock_context

        # Act
        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

        # Assert
        assert result is not None
        assert result["targetResource"] == "default/Deployment/nginx"
        assert result["regressionDetected"] is False
        mock_api.get_remediation_history_context.assert_called_once_with(
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

    def test_ds_unavailable_returns_none(self):
        """UT-RH-CLIENT-002: Connection error returns None (graceful degradation)."""
        mock_api = MagicMock()
        mock_api.get_remediation_history_context.side_effect = ConnectionError(
            "Connection refused"
        )

        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_ds_service_error_returns_none(self):
        """UT-RH-CLIENT-003: DS returns 500 ServiceException -> returns None."""
        mock_api = MagicMock()
        mock_api.get_remediation_history_context.side_effect = ServiceException(
            status=500, reason="Internal Server Error"
        )

        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_api_not_configured_returns_none(self):
        """UT-RH-CLIENT-004: None API instance returns None (DS not configured)."""
        result = query_remediation_history(
            api=None,
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_timeout_returns_none(self):
        """UT-RH-CLIENT-005: Request timeout returns None (graceful degradation)."""
        mock_api = MagicMock()
        # urllib3 raises TimeoutError which surfaces through the generated client
        mock_api.get_remediation_history_context.side_effect = ApiException(
            status=None, reason="Read timed out"
        )

        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="nginx",
            target_namespace="default",
            current_spec_hash="sha256:abc123",
        )

        assert result is None

    def test_response_uses_camel_case_aliases(self):
        """UT-RH-CLIENT-006: Response dict uses camelCase aliases for prompt builder compatibility."""
        mock_api = MagicMock()
        mock_context = MagicMock()
        # to_dict() returns camelCase aliases (OpenAPI generator convention)
        mock_context.to_dict.return_value = {
            "targetResource": "prod/Deployment/payment-api",
            "currentSpecHash": "sha256:def456",
            "regressionDetected": True,
            "tier1": {
                "window": "24h0m0s",
                "chain": [
                    {
                        "remediationUID": "rr-001",
                        "completedAt": "2026-02-12T10:00:00Z",
                        "workflowType": "restart",
                        "outcome": "success",
                        "effectivenessScore": 0.85,
                        "hashMatch": "preRemediation",
                    }
                ],
            },
            "tier2": {"window": "2160h0m0s", "chain": []},
        }
        mock_api.get_remediation_history_context.return_value = mock_context

        result = query_remediation_history(
            api=mock_api,
            target_kind="Deployment",
            target_name="payment-api",
            target_namespace="prod",
            current_spec_hash="sha256:def456",
        )

        # Verify camelCase keys that prompt builder expects
        assert result is not None
        assert "targetResource" in result
        assert "currentSpecHash" in result
        assert "regressionDetected" in result
        assert result["tier1"]["chain"][0]["remediationUID"] == "rr-001"
        assert result["tier1"]["chain"][0]["hashMatch"] == "preRemediation"
