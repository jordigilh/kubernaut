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
Unit tests for EnrichmentService (Phase 2 of #529 three-phase RCA architecture).

TDD Group 3: EnrichmentService core logic.
Tests owner resolution, label detection, remediation history fetch,
retry with exponential backoff, and fail-hard behavior.

BR-HAPI-264: Post-RCA Infrastructure Label Detection via EnrichmentService
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from typing import Dict, Any, List, Optional


class TestEnrichmentService:
    """G3: EnrichmentService core (BR-HAPI-264 + Phase 2)."""

    @pytest.mark.asyncio
    async def test_ut_529_e_001_resolves_owner_chain(self):
        """UT-529-E-001: EnrichmentService resolves owner chain (Pod -> Deployment).

        BR-HAPI-264: Given an affectedResource (Pod), the EnrichmentService
        resolves the K8s owner chain to the root owner (Deployment).
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "api-server-abc123", "namespace": "production"},
            {"kind": "ReplicaSet", "name": "api-server-rs", "namespace": "production"},
            {"kind": "Deployment", "name": "api-server", "namespace": "production"},
        ]

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=AsyncMock(return_value=None))
        result = await service.enrich(
            remediation_target={"kind": "Pod", "name": "api-server-abc123", "namespace": "production"}
        )

        assert result.root_owner is not None
        assert result.root_owner["kind"] == "Deployment"
        assert result.root_owner["name"] == "api-server"

    @pytest.mark.asyncio
    async def test_ut_529_e_002_fetches_remediation_history(self):
        """UT-529-E-002: EnrichmentService fetches history for resolved root owner.

        BR-HAPI-264 / #540: After resolving the root owner, the EnrichmentService
        fetches remediation history via the history_fetcher callable, which wraps
        the remediation_history_client wrapper with spec hash computation.
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Deployment", "name": "api-server", "namespace": "production"},
        ]

        mock_history_fetcher = AsyncMock(return_value={
            "totalRemediations": 3,
            "recentRemediations": [{"workflowId": "wf-1", "outcome": "success"}],
        })

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=mock_history_fetcher)
        result = await service.enrich(
            remediation_target={"kind": "Deployment", "name": "api-server", "namespace": "production"}
        )

        assert result.remediation_history is not None
        assert result.remediation_history["totalRemediations"] == 3
        mock_history_fetcher.assert_called_once_with(
            target_kind="Deployment",
            target_name="api-server",
            target_namespace="production",
        )

    @pytest.mark.asyncio
    async def test_ut_529_e_003_retries_k8s_with_backoff(self):
        """UT-529-E-003: EnrichmentService retries K8s API with exponential backoff.

        BR-HAPI-264: Infrastructure calls retry 3 times with 1s/2s/4s delays.
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.side_effect = [
            Exception("K8s API timeout"),
            Exception("K8s API timeout"),
            [{"kind": "Deployment", "name": "api-server", "namespace": "production"}],
        ]

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=AsyncMock(return_value=None))
        result = await service.enrich(
            remediation_target={"kind": "Pod", "name": "pod-1", "namespace": "default"}
        )

        assert result.root_owner is not None
        assert mock_k8s.resolve_owner_chain.call_count == 3

    @pytest.mark.asyncio
    async def test_ut_529_e_004_retries_history_fetcher_with_backoff(self):
        """UT-529-E-004: EnrichmentService retries history_fetcher with exponential backoff.

        BR-HAPI-264 / #540: History fetcher calls use exponential backoff retries.
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Deployment", "name": "api-server", "namespace": "production"},
        ]

        mock_history_fetcher = AsyncMock(side_effect=[
            Exception("DS connection refused"),
            {"totalRemediations": 1, "recentRemediations": []},
        ])

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=mock_history_fetcher)
        result = await service.enrich(
            remediation_target={"kind": "Deployment", "name": "api-server", "namespace": "production"}
        )

        assert result.remediation_history is not None
        assert mock_history_fetcher.call_count == 2

    @pytest.mark.asyncio
    async def test_ut_529_e_005_fails_hard_after_retry_exhaustion(self):
        """UT-529-E-005: EnrichmentService fails hard after retry exhaustion.

        BR-HAPI-264: After 3 retries, infrastructure failures cause
        the enrichment to fail with rca_incomplete status.
        """
        from src.extensions.incident.enrichment_service import (
            EnrichmentService,
            EnrichmentFailure,
        )

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.side_effect = Exception("K8s API down")

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=AsyncMock(return_value=None))

        with pytest.raises(EnrichmentFailure) as exc_info:
            await service.enrich(
                remediation_target={"kind": "Pod", "name": "pod-1", "namespace": "default"}
            )

        assert "rca_incomplete" in str(exc_info.value).lower() or exc_info.value.reason == "rca_incomplete"

    @pytest.mark.asyncio
    async def test_ut_529_e_006_returns_complete_enrichment_result(self):
        """UT-529-E-006: EnrichmentService returns complete EnrichmentResult.

        BR-HAPI-264: The result contains root_owner, detected_labels,
        and remediation_history.
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Deployment", "name": "api-server", "namespace": "production"},
        ]

        mock_history_fetcher = AsyncMock(return_value={
            "totalRemediations": 0,
            "recentRemediations": [],
        })

        service = EnrichmentService(
            k8s_client=mock_k8s,
            history_fetcher=mock_history_fetcher,
            label_detector=AsyncMock(return_value={"gitOpsManaged": True}),
        )
        result = await service.enrich(
            remediation_target={"kind": "Deployment", "name": "api-server", "namespace": "production"}
        )

        assert result.root_owner is not None
        assert result.detected_labels is not None
        assert result.remediation_history is not None

    @pytest.mark.asyncio
    async def test_ut_264_001_detects_labels_for_root_owner(self):
        """UT-HAPI-264-001: EnrichmentService detects labels for resolved root owner.

        BR-HAPI-264: Label detection runs against the resolved root owner,
        not the original affectedResource.
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Pod", "name": "pod-1", "namespace": "default"},
            {"kind": "Deployment", "name": "app", "namespace": "default"},
        ]

        mock_label_detector = AsyncMock()
        mock_label_detector.return_value = {
            "helmManaged": True,
            "hpaEnabled": False,
            "stateful": False,
        }

        service = EnrichmentService(
            k8s_client=mock_k8s,
            history_fetcher=AsyncMock(return_value=None),
            label_detector=mock_label_detector,
        )
        result = await service.enrich(
            remediation_target={"kind": "Pod", "name": "pod-1", "namespace": "default"}
        )

        assert result.detected_labels is not None
        assert result.detected_labels.get("helmManaged") is True
        mock_label_detector.assert_called_once()
        call_args = mock_label_detector.call_args
        assert call_args[0][0] == {"kind": "Deployment", "name": "app", "namespace": "default"}
        assert len(call_args[0][1]) == 2  # owner_chain has Pod + Deployment

    @pytest.mark.asyncio
    async def test_ut_540_001_history_fetcher_none_skips_history(self):
        """UT-540-001: EnrichmentService gracefully skips history when no fetcher.

        #540: When history_fetcher is None, enrichment proceeds without
        remediation history (same as ds_client=None behavior before).
        """
        from src.extensions.incident.enrichment_service import EnrichmentService

        mock_k8s = AsyncMock()
        mock_k8s.resolve_owner_chain.return_value = [
            {"kind": "Deployment", "name": "api-server", "namespace": "production"},
        ]

        service = EnrichmentService(k8s_client=mock_k8s, history_fetcher=None)
        result = await service.enrich(
            remediation_target={"kind": "Deployment", "name": "api-server", "namespace": "production"}
        )

        assert result.root_owner is not None
        assert result.remediation_history is None
