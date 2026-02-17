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
Tests for get_resource_context tool and resolve_owner_chain.

ADR-055: The LLM calls get_resource_context after RCA to fetch owner chain,
spec hash, and remediation history for the identified target resource.

Test Plan:
- UT-RC-001: resolve_owner_chain traverses Pod -> RS -> Deployment via ownerReferences
- UT-RC-002: resolve_owner_chain handles bare Pod (no ownerReferences)
- UT-RC-003: resolve_owner_chain handles resource not found (empty chain)
- UT-RC-004: resolve_owner_chain handles cluster-scoped resources (Node)
- UT-RC-005: GetResourceContextTool returns owner chain, spec hash, and history
- UT-RC-006: GetResourceContextTool handles missing resource gracefully
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from typing import Dict, Any, List, Optional


class TestResolveOwnerChain:
    """UT-RC-001 through UT-RC-004: K8s ownerReferences traversal."""

    @pytest.mark.asyncio
    async def test_pod_to_deployment_chain(self):
        """UT-RC-001: Pod -> ReplicaSet -> Deployment via ownerReferences."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        # Mock the K8s API responses for owner chain traversal
        mock_pod = MagicMock()
        mock_pod.metadata.owner_references = [
            MagicMock(kind="ReplicaSet", name="api-xyz", api_version="apps/v1")
        ]
        mock_pod.metadata.namespace = "production"

        mock_rs = MagicMock()
        mock_rs.metadata.owner_references = [
            MagicMock(kind="Deployment", name="api", api_version="apps/v1")
        ]
        mock_rs.metadata.namespace = "production"

        mock_deploy = MagicMock()
        mock_deploy.metadata.owner_references = None
        mock_deploy.metadata.namespace = "production"

        # Patch _get_resource_metadata_sync to return mocked resources
        call_count = 0
        responses = [mock_pod, mock_rs, mock_deploy]

        async def mock_get_metadata(kind, name, namespace):
            nonlocal call_count
            if call_count < len(responses):
                result = responses[call_count]
                call_count += 1
                return result
            return None

        with patch.object(k8s, "_get_resource_metadata", side_effect=mock_get_metadata):
            chain = await k8s.resolve_owner_chain("Pod", "api-xyz-abc", "production")

        assert len(chain) == 3
        assert chain[0] == {"kind": "Pod", "name": "api-xyz-abc", "namespace": "production"}
        assert chain[1] == {"kind": "ReplicaSet", "name": "api-xyz", "namespace": "production"}
        assert chain[2] == {"kind": "Deployment", "name": "api", "namespace": "production"}

    @pytest.mark.asyncio
    async def test_bare_pod_no_owner_references(self):
        """UT-RC-002: Bare Pod without ownerReferences returns single-entry chain."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        mock_pod = MagicMock()
        mock_pod.metadata.owner_references = None
        mock_pod.metadata.namespace = "default"

        async def mock_get_metadata(kind, name, namespace):
            return mock_pod

        with patch.object(k8s, "_get_resource_metadata", side_effect=mock_get_metadata):
            chain = await k8s.resolve_owner_chain("Pod", "debug-pod", "default")

        assert len(chain) == 1
        assert chain[0] == {"kind": "Pod", "name": "debug-pod", "namespace": "default"}

    @pytest.mark.asyncio
    async def test_resource_not_found_empty_chain(self):
        """UT-RC-003: Resource not found returns empty chain."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        async def mock_get_metadata(kind, name, namespace):
            return None

        with patch.object(k8s, "_get_resource_metadata", side_effect=mock_get_metadata):
            chain = await k8s.resolve_owner_chain("Pod", "missing", "default")

        assert chain == []

    @pytest.mark.asyncio
    async def test_cluster_scoped_resource(self):
        """UT-RC-004: Cluster-scoped resource (Node) returns single-entry chain."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None

        async def mock_get_metadata(kind, name, namespace):
            return mock_node

        with patch.object(k8s, "_get_resource_metadata", side_effect=mock_get_metadata):
            chain = await k8s.resolve_owner_chain("Node", "worker-1", "")

        assert len(chain) == 1
        assert chain[0] == {"kind": "Node", "name": "worker-1", "namespace": ""}


class TestGetResourceContextTool:
    """UT-RC-005 through UT-RC-006: Tool invocation tests."""

    @pytest.mark.asyncio
    async def test_returns_owner_chain_hash_and_history(self):
        """UT-RC-005: Tool returns complete context for valid resource."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "api-xyz-abc", "namespace": "production"},
            {"kind": "ReplicaSet", "name": "api-xyz", "namespace": "production"},
            {"kind": "Deployment", "name": "api", "namespace": "production"},
        ]
        mock_k8s.compute_spec_hash.return_value = "sha256:abc123"

        mock_history_fetcher = MagicMock()
        mock_history_fetcher.return_value = [
            {"workflow_id": "wf-1", "outcome": "success", "timestamp": "2026-01-01T00:00:00Z"}
        ]

        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            history_fetcher=mock_history_fetcher,
        )

        tool = toolset.tools[0]
        result = await tool.invoke(kind="Pod", name="api-xyz-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert len(data["owner_chain"]) == 3
        assert data["current_spec_hash"] == "sha256:abc123"
        assert len(data["remediation_history"]) == 1

    @pytest.mark.asyncio
    async def test_handles_missing_resource(self):
        """UT-RC-006: Tool handles missing resource gracefully."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = []
        mock_k8s.compute_spec_hash.return_value = ""

        mock_history_fetcher = MagicMock()
        mock_history_fetcher.return_value = []

        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            history_fetcher=mock_history_fetcher,
        )

        tool = toolset.tools[0]
        result = await tool.invoke(kind="Pod", name="missing", namespace="default")

        assert result.status.value == "success"
        data = result.data
        assert data["owner_chain"] == []
        assert data["current_spec_hash"] == ""
        assert data["remediation_history"] == []
