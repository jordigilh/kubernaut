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
Tests for K8s client owner resolution and spec hash computation.

Issue #97: HAPI needs to resolve the root owner from the owner chain and compute
the canonical spec hash for remediation history lookups.

Test Plan:
- UT-K8S-001: resolve_root_owner with full owner chain returns root controller
- UT-K8S-002: resolve_root_owner with empty chain falls back to signal target
- UT-K8S-003: resolve_root_owner with None chain falls back to signal target
- UT-K8S-004: resolve_root_owner with bare Pod (no controller)
- UT-K8S-005: resolve_root_owner with Node signal (cluster-scoped)
- UT-K8S-006: compute_spec_hash delegates to canonical_spec_hash
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch


class TestResolveRootOwner:
    """UT-K8S-001 through UT-K8S-005: Owner chain resolution tests."""

    def test_full_owner_chain_returns_root_controller(self):
        """UT-K8S-001: Chain [Pod, ReplicaSet, Deployment] -> Deployment."""
        from clients.k8s_client import resolve_root_owner

        owner_chain = [
            {"kind": "Pod", "name": "api-xyz-abc", "namespace": "production"},
            {"kind": "ReplicaSet", "name": "api-xyz", "namespace": "production"},
            {"kind": "Deployment", "name": "api", "namespace": "production"},
        ]
        signal_target = {
            "kind": "Pod",
            "name": "api-xyz-abc",
            "namespace": "production",
        }

        result = resolve_root_owner(owner_chain, signal_target)

        assert result["kind"] == "Deployment"
        assert result["name"] == "api"
        assert result["namespace"] == "production"

    def test_empty_chain_falls_back_to_signal_target(self):
        """UT-K8S-002: Empty owner chain falls back to signal target."""
        from clients.k8s_client import resolve_root_owner

        signal_target = {
            "kind": "Pod",
            "name": "standalone-pod",
            "namespace": "default",
        }

        result = resolve_root_owner([], signal_target)

        assert result["kind"] == "Pod"
        assert result["name"] == "standalone-pod"
        assert result["namespace"] == "default"

    def test_none_chain_falls_back_to_signal_target(self):
        """UT-K8S-003: None owner chain falls back to signal target."""
        from clients.k8s_client import resolve_root_owner

        signal_target = {
            "kind": "Pod",
            "name": "standalone-pod",
            "namespace": "default",
        }

        result = resolve_root_owner(None, signal_target)

        assert result["kind"] == "Pod"
        assert result["name"] == "standalone-pod"
        assert result["namespace"] == "default"

    def test_bare_pod_no_controller(self):
        """UT-K8S-004: Bare Pod without ownerReferences returns Pod as root."""
        from clients.k8s_client import resolve_root_owner

        signal_target = {
            "kind": "Pod",
            "name": "debug-pod",
            "namespace": "kube-system",
        }

        result = resolve_root_owner(None, signal_target)

        assert result["kind"] == "Pod"
        assert result["name"] == "debug-pod"
        assert result["namespace"] == "kube-system"

    def test_node_signal_cluster_scoped(self):
        """UT-K8S-005: Node signal (cluster-scoped, no namespace)."""
        from clients.k8s_client import resolve_root_owner

        signal_target = {
            "kind": "Node",
            "name": "worker-1",
            "namespace": "",
        }

        result = resolve_root_owner(None, signal_target)

        assert result["kind"] == "Node"
        assert result["name"] == "worker-1"
        assert result["namespace"] == ""

    def test_statefulset_chain(self):
        """UT-K8S-005b: StatefulSet chain [Pod, StatefulSet] -> StatefulSet."""
        from clients.k8s_client import resolve_root_owner

        owner_chain = [
            {"kind": "Pod", "name": "db-0", "namespace": "data"},
            {"kind": "StatefulSet", "name": "db", "namespace": "data"},
        ]
        signal_target = {
            "kind": "Pod",
            "name": "db-0",
            "namespace": "data",
        }

        result = resolve_root_owner(owner_chain, signal_target)

        assert result["kind"] == "StatefulSet"
        assert result["name"] == "db"
        assert result["namespace"] == "data"


class TestK8sResourceClientSpecHash:
    """UT-K8S-006: Spec hash computation via K8s API."""

    @pytest.mark.asyncio
    async def test_compute_spec_hash_returns_sha256(self):
        """UT-K8S-006: compute_spec_hash delegates to canonical_spec_hash."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        mock_spec = {"replicas": 3, "selector": {"matchLabels": {"app": "test"}}}

        with patch.object(
            k8s, "get_resource_spec", new_callable=AsyncMock, return_value=mock_spec
        ):
            result = await k8s.compute_spec_hash("Deployment", "test-app", "default")

        assert result.startswith("sha256:")
        assert len(result) == 71  # "sha256:" + 64 hex chars

    @pytest.mark.asyncio
    async def test_compute_spec_hash_returns_empty_on_not_found(self):
        """UT-K8S-006b: compute_spec_hash returns empty string when resource not found."""
        from clients.k8s_client import K8sResourceClient

        k8s = K8sResourceClient()

        with patch.object(
            k8s, "get_resource_spec", new_callable=AsyncMock, return_value=None
        ):
            result = await k8s.compute_spec_hash("Deployment", "missing", "default")

        assert result == ""
