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
Tests for K8s client spec hash computation.

ADR-055: resolve_root_owner tests removed (pre-computation superseded by
LLM-driven context enrichment). compute_spec_hash tests retained (reused
by get_resource_context tool).

Test Plan:
- UT-K8S-006: compute_spec_hash delegates to canonical_spec_hash
- UT-K8S-006b: compute_spec_hash returns empty on not found
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch


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
