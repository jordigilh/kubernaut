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
Tests for get_resource_context tool -- historical context only (ADR-055).

ADR-056 SoC Refactoring: DetectedLabels computation was removed from
get_resource_context. The tool now focuses exclusively on its single
concern: owner chain resolution, spec hash computation, and remediation
history. Label detection is handled by WorkflowDiscoveryToolset.

Business Requirements:
  - BR-HAPI-250: DetectedLabels integration with Data Storage (via workflow discovery)
  - BR-SP-101:   DetectedLabels Auto-Detection (via workflow discovery)

Test Matrix: 5 tests
  - UT-HAPI-056-034: Tool no longer accepts session_state/label_detector params
  - UT-HAPI-056-035: Tool returns root_owner + history (existing behavior preserved)
  - UT-HAPI-056-036: Tool resolves owner chain and returns root managing resource
  - UT-HAPI-056-037: History fetcher failure is handled gracefully
  - UT-HAPI-056-038: K8s client failure is handled gracefully
"""

import pytest
from unittest.mock import AsyncMock, MagicMock


OWNER_CHAIN_POD_TO_DEPLOY = [
    {"kind": "Pod", "name": "api-pod-abc", "namespace": "production"},
    {"kind": "ReplicaSet", "name": "api-rs-xyz", "namespace": "production"},
    {"kind": "Deployment", "name": "api", "namespace": "production"},
]


def _make_mock_k8s():
    """Create a mock K8s client with standard return values."""
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:abc123"
    return mock_k8s


class TestResourceContextSeparationOfConcerns:
    """UT-HAPI-056-034 through UT-HAPI-056-038: resource_context no longer handles label detection."""

    def test_ut_hapi_056_034_no_session_state_or_label_detector_params(self):
        """UT-HAPI-056-034: GetResourceContextTool no longer accepts session_state/label_detector."""
        from toolsets.resource_context import GetResourceContextTool
        import inspect

        sig = inspect.signature(GetResourceContextTool.__init__)
        param_names = set(sig.parameters.keys()) - {"self"}
        assert "session_state" not in param_names
        assert "label_detector" not in param_names
        assert "k8s_client" in param_names
        assert "history_fetcher" in param_names

    @pytest.mark.asyncio
    async def test_ut_hapi_056_035_returns_root_owner_and_history(self):
        """UT-HAPI-056-035: Tool still returns root_owner + remediation_history."""
        from toolsets.resource_context import GetResourceContextTool

        mock_k8s = _make_mock_k8s()
        mock_history = MagicMock(return_value=[{"workflow_id": "wf-1"}])

        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=mock_history,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert data["root_owner"] == {"kind": "Deployment", "name": "api", "namespace": "production"}
        assert len(data["remediation_history"]) == 1
        assert "detected_labels" not in data

    @pytest.mark.asyncio
    async def test_ut_hapi_056_036_resolves_owner_chain_to_root(self):
        """UT-HAPI-056-036: Tool resolves Pod -> RS -> Deployment chain and returns root."""
        from toolsets.resource_context import GetResourceContextTool

        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(k8s_client=mock_k8s)

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        root = result.data["root_owner"]
        assert root["kind"] == "Deployment"
        assert root["name"] == "api"
        mock_k8s.resolve_owner_chain.assert_called_once_with("Pod", "api-pod-abc", "production")
        mock_k8s.compute_spec_hash.assert_called_once_with("Deployment", "api", "production")

    @pytest.mark.asyncio
    async def test_ut_hapi_056_037_history_fetcher_failure_graceful(self):
        """UT-HAPI-056-037: History fetcher exception results in empty history, tool succeeds."""
        from toolsets.resource_context import GetResourceContextTool

        mock_k8s = _make_mock_k8s()
        mock_history = MagicMock(side_effect=RuntimeError("DS unavailable"))

        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=mock_history,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert result.data["remediation_history"] == []

    @pytest.mark.asyncio
    async def test_ut_hapi_056_038_k8s_client_failure_returns_error(self):
        """UT-HAPI-056-038: K8s client exception returns ERROR status."""
        from toolsets.resource_context import GetResourceContextTool

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.side_effect = RuntimeError("K8s API unavailable")

        tool = GetResourceContextTool(k8s_client=mock_k8s)

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "error"


class TestResourceContextToolsetSoC:
    """Verify ResourceContextToolset no longer accepts session_state."""

    def test_toolset_no_session_state_param(self):
        """ResourceContextToolset.__init__ no longer accepts session_state."""
        from toolsets.resource_context import ResourceContextToolset
        import inspect

        sig = inspect.signature(ResourceContextToolset.__init__)
        param_names = set(sig.parameters.keys()) - {"self"}
        assert "session_state" not in param_names
        assert "k8s_client" in param_names
        assert "history_fetcher" in param_names
