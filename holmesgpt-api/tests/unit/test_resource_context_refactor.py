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
Unit tests for #529 Resource Context Tool Refactor.

TDD Group 4: Strip session_state writes from resource context tools.
Tests that get_namespaced_resource_context and get_cluster_resource_context
no longer write root_owner, resource_scope, or detected_labels to session_state.

The tools still return correct data to the LLM, including detected_infrastructure.
EnrichmentService (Phase 2) is the sole authoritative source for these values.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from typing import Dict, Any


class TestResourceContextRefactor:
    """G4: Resource context tool refactor (#529)."""

    @pytest.mark.asyncio
    async def test_ut_529_rc_001_namespaced_no_session_state_writes(self):
        """UT-529-RC-001: get_namespaced_resource_context no longer writes to session_state.

        #529: After the refactor, the tool must NOT write root_owner,
        resource_scope, or detected_labels to session_state.
        """
        from src.toolsets.resource_context import GetNamespacedResourceContextTool

        session_state: Dict[str, Any] = {}

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "test-pod", "namespace": "default"},
            {"kind": "Deployment", "name": "test-app", "namespace": "default"},
        ]
        mock_k8s.compute_spec_hash.return_value = "abc123"

        tool = GetNamespacedResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=AsyncMock(return_value=None),
            session_state=session_state,
        )

        await tool._invoke_async(kind="Pod", name="test-pod", namespace="default")

        assert "root_owner" not in session_state
        assert "resource_scope" not in session_state
        assert "detected_labels" not in session_state

    @pytest.mark.asyncio
    async def test_ut_529_rc_002_cluster_no_session_state_writes(self):
        """UT-529-RC-002: get_cluster_resource_context no longer writes to session_state.

        #529: Cluster-scoped tool must also not write root_owner or resource_scope.
        """
        from src.toolsets.resource_context import GetClusterResourceContextTool

        session_state: Dict[str, Any] = {}

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Node", "name": "worker-1", "namespace": ""},
        ]
        mock_k8s.compute_spec_hash.return_value = "def456"

        tool = GetClusterResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=AsyncMock(return_value=None),
            session_state=session_state,
        )

        await tool._invoke_async(kind="Node", name="worker-1")

        assert "root_owner" not in session_state
        assert "resource_scope" not in session_state

    @pytest.mark.asyncio
    async def test_ut_529_rc_003_tools_still_return_data_to_llm(self):
        """UT-529-RC-003: Resource context tools still return correct data to LLM.

        #529: The tool response must still contain root_owner and
        remediation_history for the LLM's informational use. Only
        session_state writes are stripped.
        """
        from src.toolsets.resource_context import GetNamespacedResourceContextTool

        session_state: Dict[str, Any] = {}

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "test-pod", "namespace": "default"},
            {"kind": "Deployment", "name": "test-app", "namespace": "default"},
        ]
        mock_k8s.compute_spec_hash.return_value = "abc123"

        tool = GetNamespacedResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=MagicMock(return_value={"totalRemediations": 0}),
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="test-pod", namespace="default")

        assert result is not None
        result_data = result.data if hasattr(result, "data") else str(result)
        if isinstance(result_data, dict):
            assert "root_owner" in result_data
            assert result_data["root_owner"]["kind"] == "Deployment"
        else:
            assert "Deployment" in str(result_data)
