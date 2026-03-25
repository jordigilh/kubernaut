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
Tests for get_namespaced_resource_context / get_cluster_resource_context (resource_context toolset) and resolve_owner_chain.

ADR-055: The LLM calls get_namespaced_resource_context / get_cluster_resource_context after RCA to fetch remediation
context. The tool internally resolves the owner chain and spec hash, then
returns only the root owner identity and remediation history to the LLM.

Test Plan:
- UT-RC-001: resolve_owner_chain traverses Pod -> RS -> Deployment via ownerReferences
- UT-RC-002: resolve_owner_chain handles bare Pod (no ownerReferences)
- UT-RC-003: resolve_owner_chain handles resource not found (empty chain)
- UT-RC-004: resolve_owner_chain handles cluster-scoped resources (Node)
- UT-RC-005: GetNamespacedResourceContextTool returns root owner and history (not chain or hash)
- UT-RC-006: GetNamespacedResourceContextTool handles missing resource gracefully
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
        mock_owner_ref_rs = MagicMock()
        mock_owner_ref_rs.kind = "ReplicaSet"
        mock_owner_ref_rs.name = "api-xyz"
        mock_owner_ref_rs.api_version = "apps/v1"
        mock_pod.metadata.owner_references = [mock_owner_ref_rs]
        mock_pod.metadata.namespace = "production"

        mock_rs = MagicMock()
        mock_owner_ref_deploy = MagicMock()
        mock_owner_ref_deploy.kind = "Deployment"
        mock_owner_ref_deploy.name = "api"
        mock_owner_ref_deploy.api_version = "apps/v1"
        mock_rs.metadata.owner_references = [mock_owner_ref_deploy]
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


class TestGetNamespacedResourceContextTool:
    """UT-RC-005 through UT-RC-006: Namespaced tool invocation tests."""

    @pytest.mark.asyncio
    async def test_returns_root_owner_and_history(self):
        """UT-RC-005: Tool returns root owner and history (not chain or hash)."""
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
        result = await tool._invoke_async(kind="Pod", name="api-xyz-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data

        # Root owner is the last entry in the chain (Deployment), not the Pod
        assert data["root_owner"] == {"kind": "Deployment", "name": "api", "namespace": "production"}
        assert len(data["remediation_history"]) == 1

        # Owner chain and spec hash are internal -- not exposed to the LLM
        assert "owner_chain" not in data
        assert "current_spec_hash" not in data

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
        result = await tool._invoke_async(kind="Pod", name="missing", namespace="default")

        assert result.status.value == "success"
        data = result.data
        # When resource not found, root_owner falls back to the requested resource
        assert data["root_owner"] == {"kind": "Pod", "name": "missing", "namespace": "default"}
        assert data["remediation_history"] == []

        # Internal fields still not exposed
        assert "owner_chain" not in data
        assert "current_spec_hash" not in data


# ========================================
# TDD Group 1: Cluster-Scoped Resource Context Tool (#524)
# ========================================


class TestGetClusterResourceContextTool:
    """UT-HAPI-524-001 through 007: Cluster-scoped resource context."""

    @pytest.mark.asyncio
    async def test_ut_hapi_524_001_node_as_root_owner_no_namespace(self):
        """UT-HAPI-524-001: Cluster-scoped tool returns Node as root_owner with no namespace."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None
        mock_k8s._get_resource_metadata.return_value = mock_node
        mock_k8s.compute_spec_hash.return_value = "sha256:node123"

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        cluster_tool = None
        for t in toolset.tools:
            if t.name == "get_cluster_resource_context":
                cluster_tool = t
                break
        assert cluster_tool is not None, "get_cluster_resource_context tool not found in toolset"

        result = await cluster_tool._invoke_async(kind="Node", name="worker-3")

        assert result.status.value == "success"
        data = result.data
        assert data["root_owner"]["kind"] == "Node"
        assert data["root_owner"]["name"] == "worker-3"
        assert "namespace" not in data["root_owner"]

        mock_k8s.resolve_owner_chain.assert_not_called()

    @pytest.mark.asyncio
    async def test_ut_hapi_524_002_stores_resource_scope_cluster(self):
        """UT-HAPI-524-002: Cluster-scoped tool stores resource_scope='cluster' in session_state."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None
        mock_k8s._get_resource_metadata.return_value = mock_node
        mock_k8s.compute_spec_hash.return_value = ""

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        cluster_tool = [t for t in toolset.tools if t.name == "get_cluster_resource_context"][0]
        await cluster_tool._invoke_async(kind="Node", name="worker-3")

        assert session_state["resource_scope"] == "cluster"
        assert session_state["root_owner"] == {"kind": "Node", "name": "worker-3"}

    @pytest.mark.asyncio
    async def test_ut_hapi_524_003_skips_owner_chain_walk(self):
        """UT-HAPI-524-003: Cluster-scoped tool does NOT call resolve_owner_chain."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None
        mock_k8s._get_resource_metadata.return_value = mock_node
        mock_k8s.compute_spec_hash.return_value = ""

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        cluster_tool = [t for t in toolset.tools if t.name == "get_cluster_resource_context"][0]
        await cluster_tool._invoke_async(kind="Node", name="worker-3")

        mock_k8s.resolve_owner_chain.assert_not_called()

    @pytest.mark.asyncio
    async def test_ut_hapi_524_004_resource_not_found_graceful(self):
        """UT-HAPI-524-004: Cluster-scoped tool handles resource-not-found gracefully."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_k8s._get_resource_metadata.return_value = None
        mock_k8s.compute_spec_hash.return_value = ""

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        cluster_tool = [t for t in toolset.tools if t.name == "get_cluster_resource_context"][0]
        result = await cluster_tool._invoke_async(kind="Node", name="missing-node")

        assert result.status.value == "success"
        assert result.data["root_owner"]["kind"] == "Node"
        assert result.data["root_owner"]["name"] == "missing-node"

    @pytest.mark.asyncio
    async def test_ut_hapi_524_005_computes_spec_hash(self):
        """UT-HAPI-524-005: Cluster-scoped tool computes spec hash for cluster resource."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None
        mock_k8s._get_resource_metadata.return_value = mock_node
        mock_k8s.compute_spec_hash.return_value = "sha256:nodefoo"

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        cluster_tool = [t for t in toolset.tools if t.name == "get_cluster_resource_context"][0]
        await cluster_tool._invoke_async(kind="Node", name="worker-3")

        mock_k8s.compute_spec_hash.assert_called_once_with("Node", "worker-3", "")

    @pytest.mark.asyncio
    async def test_ut_hapi_524_006_history_without_namespace(self):
        """UT-HAPI-524-006: Cluster-scoped tool fetches history with empty namespace."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_node = MagicMock()
        mock_node.metadata.owner_references = None
        mock_node.metadata.namespace = None
        mock_k8s._get_resource_metadata.return_value = mock_node
        mock_k8s.compute_spec_hash.return_value = "sha256:abc"

        mock_history = MagicMock(return_value=[{"workflow_id": "remove-taint-v1"}])

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            history_fetcher=mock_history,
            session_state=session_state,
        )

        cluster_tool = [t for t in toolset.tools if t.name == "get_cluster_resource_context"][0]
        result = await cluster_tool._invoke_async(kind="Node", name="worker-3")

        mock_history.assert_called_once_with(
            resource_kind="Node",
            resource_name="worker-3",
            resource_namespace="",
            current_spec_hash="sha256:abc",
        )
        assert len(result.data["remediation_history"]) == 1


# ========================================
# TDD Group 2: Tool Rename (#524)
# ========================================


class TestToolRename524:
    """UT-HAPI-524-010 through 013: Tool rename and toolset composition."""

    def test_ut_hapi_524_010_namespaced_tool_has_new_name(self):
        """UT-HAPI-524-010: Renamed tool name attribute is 'get_namespaced_resource_context'."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        toolset = ResourceContextToolset(k8s_client=mock_k8s)

        namespaced_tools = [t for t in toolset.tools if t.name == "get_namespaced_resource_context"]
        assert len(namespaced_tools) == 1, (
            f"Expected tool named 'get_namespaced_resource_context', "
            f"found tools: {[t.name for t in toolset.tools]}"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_524_011_stores_resource_scope_namespaced(self):
        """UT-HAPI-524-011: Namespaced tool stores resource_scope='namespaced' in session_state."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Deployment", "name": "api", "namespace": "prod"},
        ]
        mock_k8s.compute_spec_hash.return_value = ""

        session_state = {}
        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        namespaced_tool = [t for t in toolset.tools if t.name == "get_namespaced_resource_context"][0]
        await namespaced_tool._invoke_async(kind="Deployment", name="api", namespace="prod")

        assert session_state["resource_scope"] == "namespaced"

    @pytest.mark.asyncio
    async def test_ut_hapi_524_012_renamed_tool_behavior_identical(self):
        """UT-HAPI-524-012: Renamed tool resolves owner chain and returns root_owner like before."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "api-xyz-abc", "namespace": "production"},
            {"kind": "ReplicaSet", "name": "api-xyz", "namespace": "production"},
            {"kind": "Deployment", "name": "api", "namespace": "production"},
        ]
        mock_k8s.compute_spec_hash.return_value = "sha256:abc123"

        mock_history = MagicMock(return_value=[{"workflow_id": "wf-1"}])
        session_state = {}

        toolset = ResourceContextToolset(
            k8s_client=mock_k8s,
            history_fetcher=mock_history,
            session_state=session_state,
        )

        namespaced_tool = [t for t in toolset.tools if t.name == "get_namespaced_resource_context"][0]
        result = await namespaced_tool._invoke_async(kind="Pod", name="api-xyz-abc", namespace="production")

        assert result.status.value == "success"
        assert result.data["root_owner"] == {"kind": "Deployment", "name": "api", "namespace": "production"}
        assert len(result.data["remediation_history"]) == 1

    def test_ut_hapi_524_013_toolset_contains_both_tools(self):
        """UT-HAPI-524-013: ResourceContextToolset contains exactly 2 tools."""
        from toolsets.resource_context import ResourceContextToolset

        mock_k8s = AsyncMock()
        toolset = ResourceContextToolset(k8s_client=mock_k8s)

        tool_names = sorted([t.name for t in toolset.tools])
        assert tool_names == ["get_cluster_resource_context", "get_namespaced_resource_context"], (
            f"Expected 2 tools, got: {tool_names}"
        )
