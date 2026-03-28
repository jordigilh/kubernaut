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
Tests for get_namespaced_resource_context / get_cluster_resource_context -- label detection + one-shot reassessment.

ADR-056 v1.4 Phase 1: get_namespaced_resource_context / get_cluster_resource_context compute DetectedLabels for the
RCA target resource (post-RCA). #529: tools no longer persist label metadata, root_owner, or target keys in
session_state — EnrichmentService is authoritative. When active labels are detected, includes
detected_infrastructure in the response for LLM RCA reassessment. Label detection runs on each tool call.

Business Requirements:
  - BR-SP-101:       DetectedLabels Auto-Detection (8 characteristics)
  - BR-SP-103:       FailedDetections tracking
  - BR-HAPI-194:     Honor failedDetections in workflow filtering
  - BR-HAPI-017-008: One-shot reassessment via detected_infrastructure
  - BR-HAPI-250:     DetectedLabels in workflow search (#366: ResourceQuota)

Test Matrix: 17 tests
  - UT-HAPI-056-034: Does not write detected_labels to session_state (#529)
  - UT-HAPI-056-035: None detection does not write detected_labels to session_state (#529)
  - UT-HAPI-056-036: Preserves return behavior (root_owner + history always present)
  - UT-HAPI-056-037: Pod->Deployment chain produces correct labels (response, not session_state)
  - UT-HAPI-056-038: Deployment-only chain produces correct labels (response)
  - UT-HAPI-056-039: StatefulSet chain labels (stateful=true) (response)
  - UT-HAPI-056-040: Namespace metadata None graceful fallback
  - UT-HAPI-056-041: LabelDetector exception does not write detected_labels (#529)
  - UT-HAPI-056-042: No session_state provided, detection runs without crash
  - UT-HAPI-056-090: Active labels include detected_infrastructure in response
  - UT-HAPI-056-091: All-default labels omit detected_infrastructure
  - UT-HAPI-056-092: Second call still runs detection; detected_infrastructure when active (#529)
  - UT-HAPI-366-011: quota_details in result_data when quotas exist (#366)
  - UT-HAPI-366-012: quota_details absent when no quotas (#366)
  - UT-HAPI-366-013: Labels dict contains only flat values (#366)
  - IT-HAPI-366-001: Full pipeline with quotas present (#366)
  - IT-HAPI-366-002: Full pipeline with no quotas (#366)
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch


OWNER_CHAIN_POD_TO_DEPLOY = [
    {"kind": "Pod", "name": "api-pod-abc", "namespace": "production"},
    {"kind": "ReplicaSet", "name": "api-rs-xyz", "namespace": "production"},
    {"kind": "Deployment", "name": "api", "namespace": "production"},
]

OWNER_CHAIN_DEPLOY_ONLY = [
    {"kind": "Deployment", "name": "api", "namespace": "production"},
]

OWNER_CHAIN_STATEFULSET = [
    {"kind": "Pod", "name": "db-0", "namespace": "production"},
    {"kind": "StatefulSet", "name": "db", "namespace": "production"},
]

LABELS_GITOPS_ARGOCD = {
    "failedDetections": [],
    "gitOpsManaged": True,
    "gitOpsTool": "argocd",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
}

LABELS_HELM_MANAGED = {
    "failedDetections": [],
    "gitOpsManaged": False,
    "gitOpsTool": "",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": True,
    "networkIsolated": False,
    "serviceMesh": "",
}

LABELS_STATEFUL = {
    "failedDetections": [],
    "gitOpsManaged": False,
    "gitOpsTool": "",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": True,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
}

LABELS_ALL_DEFAULTS = {
    "failedDetections": [],
    "gitOpsManaged": False,
    "gitOpsTool": "",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
}

LABELS_QUOTA_CONSTRAINED = {
    "failedDetections": [],
    "gitOpsManaged": False,
    "gitOpsTool": "",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
    "resourceQuotaConstrained": True,
}

LABELS_NO_QUOTA = {
    "failedDetections": [],
    "gitOpsManaged": False,
    "gitOpsTool": "",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
    "resourceQuotaConstrained": False,
}

QUOTA_SUMMARY_FIXTURE = {
    "cpu_hard": "4",
    "cpu_used": "2500m",
    "memory_hard": "8Gi",
    "memory_used": "6Gi",
    "pods_hard": "20",
    "pods_used": "15",
}


def _make_mock_k8s(owner_chain=None):
    """Create a mock K8s client with standard return values."""
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = owner_chain or OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:abc123"

    pod_meta = MagicMock()
    pod_meta.metadata.labels = {"app": "api"}
    pod_meta.metadata.annotations = {}
    mock_k8s._get_resource_metadata.return_value = pod_meta

    mock_k8s.get_namespace_metadata.return_value = {
        "labels": {},
        "annotations": {},
    }
    return mock_k8s


class TestResourceContextLabelDetection:
    """UT-HAPI-056-034 through 042: get_namespaced_resource_context / get_cluster_resource_context compute labels for RCA target."""

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_034_does_not_write_detected_labels_to_session_state(self, mock_detector_cls):
        """UT-HAPI-056-034: Labels computed post-RCA are not stored in session_state (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        data = result.data
        assert data["detected_infrastructure"]["labels"]["gitOpsManaged"] is True
        assert data["detected_infrastructure"]["labels"]["gitOpsTool"] == "argocd"

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_035_no_session_write_when_detection_returns_none(self, mock_detector_cls):
        """UT-HAPI-056-035: LabelDetector returns None — tool succeeds, no detected_labels in session_state (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (None, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert "detected_infrastructure" not in result.data
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_036_preserves_return_behavior(self, mock_detector_cls):
        """UT-HAPI-056-036: root_owner + history always present in response."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        mock_history = MagicMock(return_value=[{"workflow_id": "wf-1"}])
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            history_fetcher=mock_history,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "root_owner" in data
        assert data["root_owner"]["kind"] == "Deployment"
        assert "remediation_history" in data
        assert len(data["remediation_history"]) == 1

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_037_pod_deployment_chain_labels(self, mock_detector_cls):
        """UT-HAPI-056-037: Pod->RS->Deployment chain produces correct labels in response (#529: not in session_state)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert result.data["detected_infrastructure"]["labels"]["gitOpsManaged"] is True
        assert result.data["detected_infrastructure"]["labels"]["gitOpsTool"] == "argocd"

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_038_deployment_only_chain_labels(self, mock_detector_cls):
        """UT-HAPI-056-038: Deployment-only chain produces correct labels."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_HELM_MANAGED, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_DEPLOY_ONLY)
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Deployment", name="api", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert result.data["detected_infrastructure"]["labels"]["helmManaged"] is True

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_039_statefulset_chain_labels(self, mock_detector_cls):
        """UT-HAPI-056-039: StatefulSet in owner chain produces stateful=true."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_STATEFUL, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_STATEFULSET)
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="db-0", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert result.data["detected_infrastructure"]["labels"]["stateful"] is True

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_040_namespace_metadata_none_fallback(self, mock_detector_cls):
        """UT-HAPI-056-040: Namespace metadata None does not crash detection."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        mock_k8s.get_namespace_metadata.return_value = None
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_041_label_detector_exception_no_session_write(self, mock_detector_cls):
        """UT-HAPI-056-041: LabelDetector exception — tool succeeds, no detected_labels in session_state (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.side_effect = RuntimeError("detection failed")
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert "root_owner" in result.data

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_042_no_session_state_no_crash(self, mock_detector_cls):
        """UT-HAPI-056-042: No session_state provided, detection runs without crash."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=None,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert "root_owner" in result.data


class TestResourceContextRootOwnerCapture:
    """UT-BR-496-001 through 003: root_owner in tool response; not persisted to session_state (#529)."""

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_br_496_001_root_owner_not_stored_in_session_state(self, mock_detector_cls):
        """UT-BR-496-001: root_owner returned in data, not written to session_state (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        ro = result.data["root_owner"]
        assert ro["kind"] == "Deployment"
        assert ro["name"] == "api"
        assert ro["namespace"] == "production"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "root_owner" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_br_496_002_successive_calls_return_current_root_owner(self, mock_detector_cls):
        """UT-BR-496-002: Each call returns root_owner for that invocation; session_state unchanged (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        r1 = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")
        assert r1.data["root_owner"]["kind"] == "Deployment"

        mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_STATEFULSET
        r2 = await tool._invoke_async(kind="Pod", name="db-0", namespace="production")
        assert r2.data["root_owner"]["kind"] == "StatefulSet"
        assert r2.data["root_owner"]["name"] == "db"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "root_owner" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_br_496_003_no_session_state_no_root_owner_crash(self, mock_detector_cls):
        """UT-BR-496-003: No session_state provided, root_owner not stored, no crash."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=None,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")
        assert result.status.value == "success"


class TestResourceContextTargetTracking:
    """UT-HAPI-516-001 through 004: tools do not track target or labels in session_state (#529)."""

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_516_001_does_not_store_last_resource_context_target(self, mock_detector_cls):
        """UT-HAPI-516-001: last_resource_context_target not written to session_state (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(k8s_client=mock_k8s, session_state=session_state)

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert result.data["root_owner"]["kind"] == "Deployment"
        assert result.data["root_owner"]["name"] == "api"
        assert result.data["root_owner"]["namespace"] == "production"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "last_resource_context_target" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_516_002_redetects_labels_when_target_changes(self, mock_detector_cls):
        """UT-HAPI-516-002: New target runs detection again; response reflects new labels (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        call_count = 0

        async def detect_labels_side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                return (LABELS_GITOPS_ARGOCD, None)
            return (LABELS_STATEFUL, None)

        mock_detector = AsyncMock()
        mock_detector.detect_labels.side_effect = detect_labels_side_effect
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(k8s_client=mock_k8s, session_state=session_state)

        r1 = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")
        assert r1.data["detected_infrastructure"]["labels"]["gitOpsManaged"] is True

        mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_STATEFULSET
        r2 = await tool._invoke_async(kind="Pod", name="db-0", namespace="production")

        assert r2.data["detected_infrastructure"]["labels"]["stateful"] is True
        assert r2.data["root_owner"]["kind"] == "StatefulSet"
        assert call_count == 2
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert "last_resource_context_target" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_516_003_same_target_invokes_detection_each_call(self, mock_detector_cls):
        """UT-HAPI-516-003: Same target twice — label detection runs each time (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(k8s_client=mock_k8s, session_state=session_state)

        await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")
        await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")
        assert mock_detector.detect_labels.call_count == 2
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_516_004_first_call_does_not_write_tracking_keys(self, mock_detector_cls):
        """UT-HAPI-516-004: First call does not write detected_labels or last_resource_context_target (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s(owner_chain=OWNER_CHAIN_POD_TO_DEPLOY)
        tool = GetResourceContextTool(k8s_client=mock_k8s, session_state=session_state)

        await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state
        assert "last_resource_context_target" not in session_state


class TestResourceContextReassessment:
    """UT-HAPI-056-090 through 092: detected_infrastructure in tool response; #529 no session_state persistence."""

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_090_active_labels_include_detected_infrastructure(self, mock_detector_cls):
        """UT-HAPI-056-090: Active labels trigger detected_infrastructure in response."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "detected_infrastructure" in data
        assert "labels" in data["detected_infrastructure"]
        assert data["detected_infrastructure"]["labels"]["gitOpsManaged"] is True
        assert data["detected_infrastructure"]["labels"]["gitOpsTool"] == "argocd"
        assert "note" in data["detected_infrastructure"]
        assert len(data["detected_infrastructure"]["note"]) > 0
        assert "root_owner" in data
        assert "remediation_history" in data
        assert "failedDetections" not in data["detected_infrastructure"]["labels"]

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_091_all_default_labels_omit_detected_infrastructure(self, mock_detector_cls):
        """UT-HAPI-056-091: All-default labels omit detected_infrastructure."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_ALL_DEFAULTS, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "detected_infrastructure" not in data
        assert "root_owner" in data
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert "detected_labels" not in session_state

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_056_092_second_call_runs_redetection(self, mock_detector_cls):
        """UT-HAPI-056-092: Session pre-population does not skip detection; active labels still returned (#529)."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_GITOPS_ARGOCD, None)
        mock_detector_cls.return_value = mock_detector

        original_labels = {"gitOpsManaged": True, "gitOpsTool": "argocd"}
        session_state = {"detected_labels": original_labels}
        mock_k8s = _make_mock_k8s(owner_chain=[
            {"kind": "Node", "name": "worker-3", "namespace": ""},
        ])
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Node", name="worker-3", namespace="")

        assert result.status.value == "success"
        data = result.data
        assert "detected_infrastructure" in data
        assert data["detected_infrastructure"]["labels"]["gitOpsManaged"] is True
        assert data["root_owner"]["kind"] == "Node"
        assert data["root_owner"]["name"] == "worker-3"
        # #529: session_state writes removed; EnrichmentService is authoritative
        assert session_state["detected_labels"] is original_labels
        mock_detector.detect_labels.assert_called_once()


class TestResourceContextQuotaDetails:
    """UT-HAPI-366-011 through UT-HAPI-366-013: quota_details in tool result (#366).

    Design: detect_labels() returns Tuple[Optional[Dict], Optional[Dict]].
    _detect_labels_if_needed returns (detected_infra, quota_details) tuple.
    """

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_366_011_quota_details_in_result_when_quotas_exist(self, mock_detector_cls):
        """UT-HAPI-366-011: quota_details top-level field in tool result_data when quotas exist."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_QUOTA_CONSTRAINED, QUOTA_SUMMARY_FIXTURE)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "quota_details" in data
        assert data["quota_details"]["cpu_hard"] == "4"
        assert data["quota_details"]["memory_hard"] == "8Gi"
        assert data["quota_details"]["pods_hard"] == "20"

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_366_012_quota_details_absent_when_no_quotas(self, mock_detector_cls):
        """UT-HAPI-366-012: quota_details absent from tool result_data when no quotas."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_NO_QUOTA, None)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "quota_details" not in data

    @pytest.mark.asyncio
    @patch("src.detection.labels.LabelDetector")
    async def test_ut_hapi_366_013_labels_dict_flat_values_only(self, mock_detector_cls):
        """UT-HAPI-366-013: detected_infrastructure.labels contains only flat bool/string values."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = (LABELS_QUOTA_CONSTRAINED, QUOTA_SUMMARY_FIXTURE)
        mock_detector_cls.return_value = mock_detector

        session_state = {}
        mock_k8s = _make_mock_k8s()
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        data = result.data
        assert "detected_infrastructure" in data
        labels = data["detected_infrastructure"]["labels"]
        for key, value in labels.items():
            assert isinstance(value, (bool, str)), (
                f"Label '{key}' has type {type(value).__name__}, expected bool or str"
            )


class TestResourceContextQuotaIntegration:
    """IT-HAPI-366-001 through IT-HAPI-366-002: Full pipeline integration (#366).

    Uses real LabelDetector (not patched) with mocked K8s client.
    """

    @pytest.mark.asyncio
    async def test_it_hapi_366_001_full_pipeline_quotas_present(self):
        """IT-HAPI-366-001: Full pipeline with quotas -> label=true + quota_details in result."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_k8s = _make_mock_k8s()
        quota = MagicMock()
        quota.status.hard = {"cpu": "4", "memory": "8Gi", "pods": "20"}
        quota.status.used = {"cpu": "2500m", "memory": "6Gi", "pods": "15"}
        quota.spec.hard = {"cpu": "4", "memory": "8Gi", "pods": "20"}
        mock_k8s.list_resource_quotas = AsyncMock(return_value=([quota], None))
        mock_k8s.list_pdbs = AsyncMock(return_value=([], None))
        mock_k8s.list_hpas = AsyncMock(return_value=([], None))
        mock_k8s.list_network_policies = AsyncMock(return_value=([], None))

        session_state = {}
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert "detected_labels" not in session_state
        data = result.data
        assert "quota_details" in data
        assert data["quota_details"]["cpu_hard"] == "4"
        assert data["quota_details"]["memory_used"] == "6Gi"
        assert "detected_infrastructure" in data
        labels = data["detected_infrastructure"]["labels"]
        assert labels["resourceQuotaConstrained"] is True
        for value in labels.values():
            assert not isinstance(value, dict), "Labels must be flat (no nested dicts)"

    @pytest.mark.asyncio
    async def test_it_hapi_366_002_full_pipeline_no_quotas(self):
        """IT-HAPI-366-002: Full pipeline with no quotas -> label=false + no quota_details."""
        from toolsets.resource_context import GetNamespacedResourceContextTool as GetResourceContextTool

        mock_k8s = _make_mock_k8s()
        mock_k8s.list_resource_quotas = AsyncMock(return_value=([], None))
        mock_k8s.list_pdbs = AsyncMock(return_value=([], None))
        mock_k8s.list_hpas = AsyncMock(return_value=([], None))
        mock_k8s.list_network_policies = AsyncMock(return_value=([], None))

        session_state = {}
        tool = GetResourceContextTool(
            k8s_client=mock_k8s,
            session_state=session_state,
        )

        result = await tool._invoke_async(kind="Pod", name="api-pod-abc", namespace="production")

        assert result.status.value == "success"
        assert "detected_labels" not in session_state
        data = result.data
        assert "quota_details" not in data
